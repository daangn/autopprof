package autopprof

import (
	"bufio"
	"bytes"
	"io"
	"runtime/pprof"
	"sync"
	"time"
)

//go:generate mockgen -source=profile.go -destination=profile_mock.go -package=autopprof

type profiler interface {
	// profileCPU profiles the CPU usage for a specific duration.
	profileCPU() ([]byte, error)
	// profileHeap profiles the heap usage.
	profileHeap() ([]byte, error)
	// profileGoroutine profiles the goroutine usage.
	profileGoroutine() ([]byte, error)
}

// cpuProfileMu serializes every CPU profile autopprof takes, built-in
// or through ProfileCPU. pprof.StartCPUProfile is a process-wide
// singleton — a concurrent call fails immediately instead of waiting,
// and the cascade can land on CPU at the same tick as its own watcher.
// It is package-level for the same reason the profiler is.
var cpuProfileMu sync.Mutex

type defaultProfiler struct {
	// cpuProfilingDuration is the duration to wait until collect
	// the enough cpu profiling data.
	// Default: 10s.
	cpuProfilingDuration time.Duration
}

func newDefaultProfiler(duration time.Duration) *defaultProfiler {
	return &defaultProfiler{
		cpuProfilingDuration: duration,
	}
}

func (p *defaultProfiler) profileCPU() ([]byte, error) {
	var buf bytes.Buffer
	if err := ProfileCPU(&buf, p.cpuProfilingDuration); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ProfileCPU records a CPU profile for duration d and writes it to w
// in pprof format. Calls wait for each other and for autopprof's
// built-in CPU profiling, so it is safe to call from Metric.Collect
// while the CPU watcher is profiling: it blocks until its turn instead
// of failing. It still fails if code outside autopprof (e.g.
// net/http/pprof) is holding the CPU profiler.
func ProfileCPU(w io.Writer, d time.Duration) error {
	if d <= 0 {
		return ErrInvalidCPUProfileDuration
	}
	cpuProfileMu.Lock()
	defer cpuProfileMu.Unlock()

	bw := bufio.NewWriter(w)
	if err := pprof.StartCPUProfile(bw); err != nil {
		return err
	}
	<-time.After(d)
	pprof.StopCPUProfile()
	return bw.Flush()
}

func (p *defaultProfiler) profileHeap() ([]byte, error) {
	var (
		buf bytes.Buffer
		w   = bufio.NewWriter(&buf)
	)
	if err := pprof.Lookup("heap").WriteTo(w, 0); err != nil {
		return nil, err
	}
	if err := w.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (p *defaultProfiler) profileGoroutine() ([]byte, error) {
	var (
		buf bytes.Buffer
		w   = bufio.NewWriter(&buf)
	)
	if err := pprof.Lookup("goroutine").WriteTo(w, 0); err != nil {
		return nil, err
	}
	if err := w.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
