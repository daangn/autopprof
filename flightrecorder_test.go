package autopprof

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"runtime/trace"
	"sync"
	"testing"
	"time"
)

// traceHeader matches the 16-byte header every Go execution trace
// starts with, e.g. "go 1.26 trace\x00\x00\x00".
var traceHeader = regexp.MustCompile(`^go 1\.\d+ trace\x00+$`)

var smallFlightRecorder = FlightRecorderOption{MinAge: time.Second, MaxBytes: 1 << 20}

// startTestFlightRecorder starts the global recorder and stops it when
// the test ends, so later tests see it disabled.
func startTestFlightRecorder(tb testing.TB, opt FlightRecorderOption) {
	tb.Helper()
	if err := startFlightRecorder(&opt); err != nil {
		tb.Fatalf("startFlightRecorder: %v", err)
	}
	tb.Cleanup(stopFlightRecorder)
}

// spawnSome generates a few scheduler events so the window isn't
// header-only.
func spawnSome() {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go wg.Done()
	}
	wg.Wait()
}

func TestSnapshotTrace_enabled(t *testing.T) {
	startTestFlightRecorder(t, smallFlightRecorder)
	spawnSome()

	var buf bytes.Buffer
	n, err := SnapshotTrace(&buf)
	if err != nil {
		t.Fatalf("SnapshotTrace: %v", err)
	}
	if n != int64(buf.Len()) || n <= 16 {
		t.Fatalf("SnapshotTrace n=%d len=%d, want n == len > 16", n, buf.Len())
	}
	if h := buf.Bytes()[:16]; !traceHeader.Match(h) {
		t.Errorf("trace header = %q, want go 1.N trace", h)
	}
}

func TestSnapshotTrace_disabled(t *testing.T) {
	t.Run("never started", func(t *testing.T) {
		if _, err := SnapshotTrace(io.Discard); !errors.Is(err, ErrFlightRecorderDisabled) {
			t.Errorf("SnapshotTrace = %v, want ErrFlightRecorderDisabled", err)
		}
	})
	t.Run("nil option is a no-op", func(t *testing.T) {
		if err := startFlightRecorder(nil); err != nil {
			t.Fatalf("startFlightRecorder(nil) = %v", err)
		}
		if _, err := SnapshotTrace(io.Discard); !errors.Is(err, ErrFlightRecorderDisabled) {
			t.Errorf("SnapshotTrace = %v, want ErrFlightRecorderDisabled", err)
		}
	})
	t.Run("after stop", func(t *testing.T) {
		startTestFlightRecorder(t, smallFlightRecorder)
		stopFlightRecorder()
		if _, err := SnapshotTrace(io.Discard); !errors.Is(err, ErrFlightRecorderDisabled) {
			t.Errorf("SnapshotTrace = %v, want ErrFlightRecorderDisabled", err)
		}
	})
}

// TestSnapshotTrace_concurrentSerialized: the stdlib WriteTo fails fast
// with "already in progress" on overlap; SnapshotTrace must queue instead.
func TestSnapshotTrace_concurrentSerialized(t *testing.T) {
	startTestFlightRecorder(t, smallFlightRecorder)
	spawnSome()

	const callers = 8
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := SnapshotTrace(io.Discard)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent SnapshotTrace: %v", err)
		}
	}
}

// TestSnapshotTrace_racesStop: writers overlapping stopFlightRecorder see
// either a full snapshot or ErrFlightRecorderDisabled — never a stdlib
// error from a recorder stopped mid-write. Meaningful under -race.
func TestSnapshotTrace_racesStop(t *testing.T) {
	startTestFlightRecorder(t, smallFlightRecorder)
	spawnSome()

	const callers = 8
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := SnapshotTrace(io.Discard)
			errs <- err
		}()
	}
	stopFlightRecorder()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !errors.Is(err, ErrFlightRecorderDisabled) {
			t.Errorf("SnapshotTrace racing stop: %v", err)
		}
	}
}

// TestStartFlightRecorder_failFast: only one flight recorder may run
// per process, so a recorder someone else already started must surface
// as an error rather than a silently missing trace.
func TestStartFlightRecorder_failFast(t *testing.T) {
	other := trace.NewFlightRecorder(trace.FlightRecorderConfig{})
	if err := other.Start(); err != nil {
		t.Fatalf("start other recorder: %v", err)
	}
	t.Cleanup(other.Stop)

	if err := startFlightRecorder(&FlightRecorderOption{}); err == nil {
		stopFlightRecorder()
		t.Fatal("startFlightRecorder succeeded while another recorder was active")
	}
	if _, err := SnapshotTrace(io.Discard); !errors.Is(err, ErrFlightRecorderDisabled) {
		t.Errorf("SnapshotTrace after failed start = %v, want ErrFlightRecorderDisabled", err)
	}
}
