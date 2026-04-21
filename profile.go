package autopprof

import (
	"bufio"
	"bytes"
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

type defaultProfiler struct {
	// cpuProfilingDuration is the duration to wait until collect
	// the enough cpu profiling data.
	// Default: 10s.
	cpuProfilingDuration time.Duration

	// cpuMu serializes profileCPU calls. pprof.StartCPUProfile is a
	// process-wide singleton — concurrent invocations would make the
	// second one fail immediately. With ReportAll a cascade path can
	// land on CPU at the same tick as its own watcher, so we gate it.
	cpuMu sync.Mutex
}

func newDefaultProfiler(duration time.Duration) *defaultProfiler {
	return &defaultProfiler{
		cpuProfilingDuration: duration,
	}
}

func (p *defaultProfiler) profileCPU() ([]byte, error) {
	p.cpuMu.Lock()
	defer p.cpuMu.Unlock()

	var (
		buf bytes.Buffer
		w   = bufio.NewWriter(&buf)
	)
	if err := pprof.StartCPUProfile(w); err != nil {
		return nil, err
	}
	<-time.After(p.cpuProfilingDuration)
	pprof.StopCPUProfile()

	if err := w.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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
