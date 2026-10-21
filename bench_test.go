//go:build linux
// +build linux

package autopprof

import (
	"bytes"
	"fmt"
	"io"
	"runtime/trace"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// -------------------------------------------------------------------
// Benchmarks — cost of the always-on flight recorder and the event
// trigger path. Run with ./benchmark.sh; results are in the README.
// -------------------------------------------------------------------

// fanOut mimics a request handler that fans out to n goroutines (an
// unbounded errgroup) and waits for all of them.
func fanOut(n int) {
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fib(18)
		}()
	}
	wg.Wait()
}

// benchJobs spans "no scheduler events" (Heavy) to "nothing but
// scheduler events" (HeavyAsync). Tracing cost scales with events, not
// CPU, so FanOut40 is the shape closest to a fan-out RPC server.
var benchJobs = []struct {
	name string
	run  func()
}{
	{"Heavy", func() { fib(24) }},
	{"FanOut40", func() { fanOut(40) }},
	{"LightAsync", func() { fibAsync(10) }},
	{"HeavyAsync", func() { fibAsync(24) }},
}

// BenchmarkFlightRecorderOverhead compares each job with the flight
// recorder off and on. Read with `benchstat -col /fr`.
func BenchmarkFlightRecorderOverhead(b *testing.B) {
	for _, job := range benchJobs {
		b.Run("job="+job.name, func(b *testing.B) {
			b.Run("fr=off", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					job.run()
				}
			})
			b.Run("fr=on", func(b *testing.B) {
				startTestFlightRecorder(b, FlightRecorderOption{
					MinAge: time.Minute, MaxBytes: 64 << 20,
				})
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					job.run()
				}
			})
		})
	}
}

type countingWriter struct{ n atomic.Int64 }

func (w *countingWriter) Write(p []byte) (int, error) {
	w.n.Add(int64(len(p)))
	return len(p), nil
}

// BenchmarkTraceRate reports how many trace bytes each job produces —
// the input for sizing FlightRecorderOption.MaxBytes: the window lasts
// about MaxBytes / trace-MiB/s seconds.
func BenchmarkTraceRate(b *testing.B) {
	for _, job := range benchJobs {
		b.Run("job="+job.name, func(b *testing.B) {
			var w countingWriter
			if err := trace.Start(&w); err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				job.run()
			}
			b.StopTimer()
			trace.Stop() // flushes; w.n is final after this
			traced := float64(w.n.Load())
			b.ReportMetric(traced/float64(b.N), "trace-B/op")
			b.ReportMetric(traced/b.Elapsed().Seconds()/(1<<20), "trace-MiB/s")
		})
	}
}

// backgroundLoad keeps the scheduler busy the way a loaded server
// would, so snapshots are taken while the ring is being written to.
func backgroundLoad() (stop func()) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				fanOut(40)
			}
		}
	}()
	return func() {
		close(done)
		wg.Wait()
	}
}

// BenchmarkWriteTrace measures a snapshot of a full ring under load —
// the first thing a Collect does, ahead of the stop-the-world goroutine
// dump. sink=discard isolates the freeze (flush + ring copy; the window
// can't be overwritten after it), sink=buffer adds copying the bytes
// out, as a Collect building an in-memory bundle would.
func BenchmarkWriteTrace(b *testing.B) {
	for _, mib := range []uint64{4, 16, 64} {
		for _, sink := range []string{"discard", "buffer"} {
			b.Run(fmt.Sprintf("maxbytes=%dMiB/sink=%s", mib, sink), func(b *testing.B) {
				maxBytes := mib << 20
				startTestFlightRecorder(b, FlightRecorderOption{
					MinAge: time.Hour, MaxBytes: maxBytes, // let MaxBytes bind
				})
				stop := backgroundLoad()
				defer stop()
				fillFlightRecorder(b, maxBytes)

				var w io.Writer = io.Discard
				var buf bytes.Buffer
				if sink == "buffer" {
					buf.Grow(int(maxBytes) * 2)
					w = &buf
				}
				var total int64
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					buf.Reset()
					n, err := WriteTrace(w)
					if err != nil {
						b.Fatal(err)
					}
					total += n
				}
				b.ReportMetric(float64(total)/float64(b.N)/(1<<20), "snapshot-MiB")
			})
		}
	}
}

// fillFlightRecorder waits until the ring holds ~maxBytes, otherwise
// early iterations snapshot a nearly empty window.
func fillFlightRecorder(b *testing.B, maxBytes uint64) {
	b.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		n, err := WriteTrace(io.Discard)
		if err != nil {
			b.Fatal(err)
		}
		if uint64(n) >= maxBytes*9/10 {
			return
		}
		if time.Now().After(deadline) {
			b.Fatalf("ring reached only %d of %d bytes in 30s", n, maxBytes)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// BenchmarkEventTrigger measures Events() send → Collect running on
// the watcher goroutine. The ticker path's equivalent is on average
// half the watch interval (2.5s at the default 5s).
func BenchmarkEventTrigger(b *testing.B) {
	collected := make(chan struct{})
	em := newFakeEventMetric("bench", func(float64) (CollectResult, error) {
		collected <- struct{}{}
		return CollectResult{}, nil // nil Reader: skip the Reporter
	})
	ap := newTestAp(b, nil)
	ap.reportCooldown = 0 // fire on every event
	if err := ap.registerMetric(em); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(ap.stop)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		em.events <- 1
		<-collected
	}
}

// BenchmarkEventNotify measures the sender side recommended for
// EventMetric: a non-blocking send on a 1-slot channel that is already
// full, i.e. many requests overflowing at once while a report is
// pending. This runs on the request path, so it must stay cheap under
// contention.
func BenchmarkEventNotify(b *testing.B) {
	events := make(chan float64, 1)
	events <- 0
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			select {
			case events <- 1:
			default:
			}
		}
	})
}
