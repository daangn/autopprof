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
	//
	// Names "cpu", "mem", and "goroutine" are reserved for the built-in
	// metrics and cannot be used here.
	Metrics []Metric
}

