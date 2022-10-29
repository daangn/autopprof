package autopprof

import (
	"bufio"
	"bytes"
	"runtime/pprof"
	"time"
)

//go:generate mockgen -source=profile.go -destination=profile_mock.go -package=autopprof

type profiler interface {
	// profileCPU profiles the CPU usage for a specific duration.
	profileCPU() ([]byte, error)
	// profileHeap profiles the heap usage.
	profileHeap() ([]byte, error)
}

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
	if err := pprof.WriteHeapProfile(w); err != nil {
		return nil, err
	}
	if err := w.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
