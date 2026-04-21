package autopprof

import (
	"time"

	"github.com/daangn/autopprof/v2/report"
)

const (
	defaultApp                         = "autopprof"
	defaultCPUThreshold                = 0.75
	defaultMemThreshold                = 0.75
	defaultGoroutineThreshold          = 50000
	defaultWatchInterval               = 5 * time.Second
	defaultCPUProfilingDuration        = 10 * time.Second
	defaultMinConsecutiveOverThreshold = 12 // 12 * 5s == 1 minute
)

// Option is the configuration for autopprof.
type Option struct {
	// DisableCPUProf disables the CPU profiling.
	DisableCPUProf bool
	// DisableMemProf disables the memory profiling.
	DisableMemProf bool
	// DisableGoroutineProf disables the goroutine profiling.
	DisableGoroutineProf bool

	// CPUThreshold is the cpu usage threshold (between 0 and 1) to
	// trigger the cpu profiling. Autopprof starts cpu profiling when
	// the cpu usage is higher than this threshold.
	CPUThreshold float64

	// MemThreshold is the memory usage threshold (between 0 and 1) to
	// trigger the heap profiling. Autopprof starts heap profiling
	// when the memory usage is higher than this threshold.
	MemThreshold float64

	// GoroutineThreshold is the goroutine count threshold to trigger
	// the goroutine profiling. Autopprof starts goroutine profiling
	// when the goroutine count is higher than this threshold.
	GoroutineThreshold int

	// ReportAll triggers reports for every enabled built-in profile
	// when any of them exceeds its threshold. Disabled built-ins are
	// skipped.
	ReportAll bool

	// Reporter is the reporter to send the profiling report. Must
	// implement the report.Reporter interface.
	Reporter report.Reporter

	// App is embedded in built-in CPU/Mem/Goroutine filenames as the
	// "<app>" segment. Defaults to "autopprof" when left empty.
	App string

	// Metrics are user-defined Metrics registered at Start. Additional
	// metrics can be added later via autopprof.Register.
	Metrics []Metric
}

func (o Option) validate() error {
	// Allow disabling every built-in as long as at least one custom
	// Metric is registered.
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
