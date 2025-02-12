package autopprof

import (
	"time"

	"github.com/ethan-k/autopprof/report"
)

const (
	defaultCPUThreshold                = 0.75
	defaultMemThreshold                = 0.75
	defaultGoroutineThreshold          = 50000
	defaultWatchInterval               = 5 * time.Second
	defaultCPUProfilingDuration        = 10 * time.Second
	defaultMinConsecutiveOverThreshold = 12 // min 1 minute. (12*5s)
)

// Option is the configuration for the autopprof.
type Option struct {
	// DisableCPUProf disables the CPU profiling.
	DisableCPUProf bool
	// DisableMemProf disables the memory profiling.
	DisableMemProf bool
	// DisableGoroutineProf disables the goroutine profiling.
	DisableGoroutineProf bool

	// CPUThreshold is the cpu usage threshold (between 0 and 1)
	//  to trigger the cpu profiling.
	// Autopprof will start the cpu profiling when the cpu usage
	//  is higher than this threshold.
	CPUThreshold float64

	// MemThreshold is the memory usage threshold (between 0 and 1)
	//  to trigger the heap profiling.
	// Autopprof will start the heap profiling when the memory usage
	//  is higher than this threshold.
	MemThreshold float64

	// GoroutineThreshold is the goroutine count threshold to trigger the goroutine profiling.
	//  to trigger the goroutine profiling.
	// Autopprof will start the goroutine profiling when the goroutine count
	//  is higher than this threshold.
	GoroutineThreshold int

	// deprecated: use reportAll instead.
	// ReportBoth sets whether to trigger reports for both CPU and memory when either threshold is exceeded.
	// If some profiling is disabled, exclude it.
	ReportBoth bool

	// ReportAll sets whether to trigger reports for all profiling types when any threshold is exceeded.
	// If some profiling is disabled, exclude it.
	ReportAll bool

	// Reporter is the reporter to send the profiling report implementing
	//  the report.Reporter interface.
	Reporter report.Reporter
}

// NOTE(mingrammer): testing the validate() is done in autopprof_test.go.
func (o Option) validate() error {
	if o.DisableCPUProf && o.DisableMemProf && o.DisableGoroutineProf {
		return ErrDisableAllProfiling
	}
	if o.CPUThreshold < 0 || o.CPUThreshold > 1 {
		return ErrInvalidCPUThreshold
	}
	if o.MemThreshold < 0 || o.MemThreshold > 1 {
		return ErrInvalidMemThreshold
	}
	if o.GoroutineThreshold < 0 {
		return ErrInvalidGoroutineThreshold
	}
	if o.Reporter == nil {
		return ErrNilReporter
	}
	return nil
}
