//go:build linux
// +build linux

package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/daangn/autopprof/v2"
	"github.com/daangn/autopprof/v2/report"
)

type mm struct {
	m map[int64]string
}

// queue is a toy struct that also implements autopprof.Metric —
// showing how a user's own type can plug directly into the watcher.
type queue struct {
	name      string
	threshold float64
}

func (q *queue) Name() string            { return q.name }
func (q *queue) Threshold() float64      { return q.threshold }
func (q *queue) Interval() time.Duration { return 3 * time.Second }

// Query returns the current queue depth. Normally this reads from a
// real backend; here we just return a constant so the threshold is
// always breached and the example always emits one report.
func (q *queue) Query() (float64, error) { return 42, nil }

// Collect produces the payload (a plaintext snapshot in this example)
// along with the filename and comment autopprof will forward via the
// Reporter. Returning (nil, nil) for Reader would skip the Reporter
// call, which is useful for side-effect-only hooks.
func (q *queue) Collect(value float64) (autopprof.CollectResult, error) {
	body := fmt.Sprintf("queue=%s depth=%v threshold=%v", q.name, value, q.threshold)
	return autopprof.CollectResult{
		Reader:   bytes.NewReader([]byte(body)),
		Filename: fmt.Sprintf("%s.snapshot.txt", q.name),
		Comment:  fmt.Sprintf(":warning:[%s] queue depth %.0f exceeded %.0f", q.name, value, q.threshold),
	}, nil
}

func main() {
	// (A) Start with the built-in CPU / Mem watchers. Option.App is
	// the single source of truth for the "<app>" segment in built-in
	// filenames.
	err := autopprof.Start(autopprof.Option{
		App:          "YOUR_APP_NAME",
		CPUThreshold: 0.8, // Default: 0.75.
		MemThreshold: 0.8, // Default: 0.75.
		Reporter: report.NewSlackReporter(
			&report.SlackReporterOption{
				Token:     "YOUR_TOKEN_HERE",
				ChannelID: "REPORT_CHANNEL_ID",
			},
		),
	})
	if errors.Is(err, autopprof.ErrUnsupportedPlatform) {
		// You can just skip the autopprof.
		log.Println(err)
	} else if err != nil {
		log.Fatalln(err)
	}
	defer autopprof.Stop()

	// (B) Register a user Metric implemented on a domain struct.
	// Perfect for metrics that live inside a lifecycle that starts
	// *after* autopprof.Start, e.g. a queue or connection pool whose
	// handle isn't yet available at Start time.
	q := &queue{name: "ingest", threshold: 10}
	if err := autopprof.Register(q); err != nil {
		log.Println("Register queue metric:", err)
	}

	// (C) Ad-hoc Metric via NewMetric — no custom struct needed.
	// Watches the process's goroutine count and dumps a full
	// goroutine stack trace when it exceeds the threshold.
	_ = autopprof.Register(autopprof.NewMetric(
		"goroutine_blocked",
		100,
		5*time.Second,
		func() (float64, error) {
			return float64(runtime.NumGoroutine()), nil
		},
		func(v float64) (autopprof.CollectResult, error) {
			var buf bytes.Buffer
			if err := pprof.Lookup("goroutine").WriteTo(&buf, 1); err != nil {
				return autopprof.CollectResult{}, err
			}
			return autopprof.CollectResult{
				Reader:   bytes.NewReader(buf.Bytes()),
				Filename: fmt.Sprintf("goroutine_blocked_%d.txt", time.Now().Unix()),
				Comment:  fmt.Sprintf(":rotating_light:[GB] count=%d", int(v)),
			}, nil
		},
	))

	eatMemory()

	go func() {
		for {
			iterative(1000)
		}
	}()
	go func() {
		for {
			recursive(15)
		}
	}()

	for {
		fmt.Println("main")
	}
}

func eatMemory() {
	m := make(map[int64]string, 20000000)
	for i := 0; i < 20000000; i++ {
		m[int64(i)] = "eating heap memory"
	}
	_ = mm{m: m}
}

// Iterative fibonacci func implementation.
func iterative(n int) int64 {
	var a, b int64 = 0, 1
	for i := 0; i < n; i++ {
		a, b = b, a+b
	}
	return a
}

// Recursive fibonacci func implementation.
func recursive(n int) int64 {
	if n <= 1 {
		return int64(n)
	}
	return recursive(n-1) + recursive(n-2)
}
