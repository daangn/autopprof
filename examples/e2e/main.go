//go:build linux
// +build linux

// e2e is a manual end-to-end test that exercises every built-in
// metric (CPU / Mem / Goroutine) plus a runtime-registered custom
// metric and uploads every resulting report to a real Slack channel.
//
// Configure via env vars:
//
//	SLACK_TOKEN       — Slack bot token that can upload files.
//	SLACK_CHANNEL_ID  — destination channel ID (not name).
//	SLACK_APP         — optional; defaults to "autopprof-e2e" and shows
//	                    up as the <app> segment in built-in filenames.
//	E2E_DURATION      — optional; total run time. Defaults to 180s so
//	                    the CPU snapshot queue (24 samples × 5s) has
//	                    time to warm up and fire at least once.
//
// Build + run inside a cgroup-enabled Linux container — see the
// adjacent Dockerfile / run.sh for a one-liner.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/daangn/autopprof/v2"
	"github.com/daangn/autopprof/v2/report"
)

func main() {
	token := os.Getenv("SLACK_TOKEN")
	channelID := os.Getenv("SLACK_CHANNEL_ID")
	if token == "" || channelID == "" {
		log.Fatalln("SLACK_TOKEN and SLACK_CHANNEL_ID env vars are required")
	}
	app := getenvOrDefault("SLACK_APP", "autopprof-e2e")
	duration := getenvDuration("E2E_DURATION", 180*time.Second)

	err := autopprof.Start(autopprof.Option{
		App: app,
		// Aggressive thresholds so every built-in metric fires during
		// the test window, even on small hosts.
		CPUThreshold:       0.30,
		MemThreshold:       0.30,
		GoroutineThreshold: 200,
		ReportAll:          false,
		Reporter: report.NewSlackReporter(&report.SlackReporterOption{
			Token:     token,
			ChannelID: channelID,
		}),
	})
	if errors.Is(err, autopprof.ErrUnsupportedPlatform) {
		log.Fatalln("autopprof only runs on linux containers; bail out")
	}
	if err != nil {
		log.Fatalln(err)
	}
	defer autopprof.Stop()

	// A runtime-registered custom metric: a counter that we spike
	// on purpose so the custom path also hits Slack.
	counter := &atomicCounter{}
	if err := autopprof.Register(autopprof.NewMetric(
		"e2e_counter",
		100, // threshold
		3*time.Second,
		func() (float64, error) {
			return float64(counter.Load()), nil
		},
		func(v float64) (autopprof.CollectResult, error) {
			body := fmt.Sprintf("e2e counter snapshot = %.0f\n", v)
			return autopprof.CollectResult{
				Reader:   bytes.NewReader([]byte(body)),
				Filename: fmt.Sprintf("e2e_counter_%d.txt", time.Now().Unix()),
				Comment:  fmt.Sprintf(":rotating_light:[e2e] counter=%.0f threshold=100", v),
			}, nil
		},
	)); err != nil {
		log.Fatalln("Register(e2e_counter):", err)
	}

	log.Printf("autopprof started (app=%q, duration=%s)", app, duration)
	log.Println("generating load: CPU burn, memory inflation, goroutine spawn, custom counter spike")

	stop := make(chan struct{})
	defer close(stop)

	// CPU pressure.
	for i := 0; i < runtime.NumCPU()*2; i++ {
		go burnCPU(stop)
	}
	// Memory pressure.
	go inflateMemory(stop)
	// Goroutine count pressure.
	go spawnGoroutines(300, stop)
	// Custom metric spike.
	counter.Store(250)

	log.Printf("sleeping %s so the CPU snapshot queue (24 × 5s) warms up and every metric can fire", duration)
	time.Sleep(duration)
	log.Println("done; calling autopprof.Stop()")
}

func burnCPU(stop <-chan struct{}) {
	var x uint64
	for {
		select {
		case <-stop:
			return
		default:
			x = fib(20) ^ x
		}
	}
}

func fib(n int) uint64 {
	if n < 2 {
		return uint64(n)
	}
	return fib(n-1) + fib(n-2)
}

func inflateMemory(stop <-chan struct{}) {
	// Allocate a steadily-growing chunk so MemThreshold triggers even
	// when the container memory limit is modest.
	buckets := make([][]byte, 0, 4096)
	for {
		select {
		case <-stop:
			runtime.KeepAlive(buckets)
			return
		default:
			buckets = append(buckets, make([]byte, 1<<20)) // 1 MiB
			if len(buckets) > 4096 {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func spawnGoroutines(n int, stop <-chan struct{}) {
	for i := 0; i < n; i++ {
		go func() { <-stop }()
	}
}

type atomicCounter struct{ v atomic.Int64 }

func (c *atomicCounter) Load() int64    { return c.v.Load() }
func (c *atomicCounter) Store(x int64)  { c.v.Store(x) }

func getenvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvDuration(key string, def time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		log.Printf("invalid %s=%q; falling back to %s", key, raw, def)
		return def
	}
	return d
}
