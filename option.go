package autopprof

import (
	"time"

	"github.com/daangn/autopprof/v2/report"
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

	// ReportAll sets whether to trigger reports for all profiling types when any threshold is exceeded.
	// If some profiling is disabled, exclude it.
	ReportAll bool

	// Reporter is the reporter to send the profiling report implementing
	//  the report.Reporter interface.
	Reporter report.Reporter

	// App is embedded in built-in CPU/Mem/Goroutine filenames as the
	// "<app>" segment. If left empty, the app segment is omitted.
	App string

	// Metrics are user-defined Metrics registered at Start. Additional
	// metrics can be added later via autopprof.Register.
	Metrics []Metric
}

// NOTE(mingrammer): testing the validate() is done in autopprof_test.go.
func (o Option) validate() error {
	// Disable-all is only an error when no user metrics pick up the
	// slack; a user with one or more Metrics can still make the
	// library do meaningful work.
	if o.DisableCPUProf && o.DisableMemProf && o.DisableGoroutineProf && len(o.Metrics) == 0 {
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

	for _, m := range o.Metrics {
		if err := validateMetric(m); err != nil {
			return err
		}
	}
	return nil
}

