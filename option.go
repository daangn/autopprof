package autopprof

import (
	"time"

	"github.com/daangn/autopprof/report"
)

const (
	defaultCPUThreshold                = 0.75
	defaultMemThreshold                = 0.75
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

	// ReportBoth sets whether to trigger reports for both CPU and memory when either threshold is exceeded.
	// If some profiling is disabled, exclude it.
	ReportBoth bool

	// Reporter is the reporter to send the profiling report implementing
	//  the report.Reporter interface.
	Reporter report.Reporter
}

// NOTE(mingrammer): testing the validate() is done in autopprof_test.go.
func (o Option) validate() error {
	if o.DisableCPUProf && o.DisableMemProf {
		return ErrDisableAllProfiling
	}
	if o.CPUThreshold < 0 || o.CPUThreshold > 1 {
		return ErrInvalidCPUThreshold
	}
	if o.MemThreshold < 0 || o.MemThreshold > 1 {
		return ErrInvalidMemThreshold
	}
	if o.Reporter == nil {
		return ErrNilReporter
	}
	return nil
}
