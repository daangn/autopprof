package autopprof

import (
	"time"

	"github.com/daangn/autopprof/v3/report"
)

const (
	defaultApp                  = "autopprof"
	defaultCPUThreshold         = 0.75
	defaultMemThreshold         = 0.75
	defaultGoroutineThreshold   = 50000
	defaultWatchInterval        = 5 * time.Second
	defaultCPUProfilingDuration = 10 * time.Second
	defaultReportCooldown       = 1 * time.Minute
	defaultReportTimeout        = 5 * time.Second
)

// Option is the configuration for autopprof.
type Option struct {
	// DisableCPUProf disables the CPU profiling. Disabled built-ins
	// are also skipped by the cascade that fires when any other
	// built-in breaches its threshold.
	DisableCPUProf bool
	// DisableMemProf disables the memory profiling. Disabled built-ins
	// are also skipped by the cascade that fires when any other
	// built-in breaches its threshold.
	DisableMemProf bool
	// DisableGoroutineProf disables the goroutine profiling. Disabled
	// built-ins are also skipped by the cascade that fires when any
	// other built-in breaches its threshold.
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

	// Reporter is the reporter to send the profiling report. Must
	// implement the report.Reporter interface.
	Reporter report.Reporter

	// ReportTimeout is the per-call timeout passed to Reporter.Report
	// as the ctx deadline. Defaults to 5s when left zero.
	ReportTimeout time.Duration

	// WatchInterval is how often each metric is sampled against its
	// threshold. It applies to the built-in CPU/Mem/Goroutine watchers
	// and to any custom Metric that returns 0 from Interval(). Defaults
	// to 5s when left zero.
	WatchInterval time.Duration

	// ReportCooldown is the minimum time before the same metric is
	// reported again while it stays over threshold. A report still fires
	// immediately on the first breach; subsequent reports for that metric
	// are suppressed until this much wall-clock time has elapsed. The
	// elapsed check runs once per sample, so a metric is reported at most
	// once per watch interval, and dropping below the threshold re-arms an
	// immediate report on the next breach. Defaults to 1m when left zero.
	//
	// EventMetric events are debounced by the same duration but on their
	// own clock: ticks below the threshold do not re-arm the event path,
	// and an event report does not delay the next ticker report.
	ReportCooldown time.Duration

	// App is embedded in built-in CPU/Mem/Goroutine filenames as the
	// "<app>" segment. Defaults to "autopprof" when left empty.
	App string

	// Metrics are user-defined Metrics registered at Start. Additional
	// metrics can be added later via autopprof.Register.
	Metrics []Metric

	// FlightRecorder keeps a process-wide execution trace ring buffer
	// running so reports can include what happened *before* they fired.
	// Read it with WriteTrace, typically from Metric.Collect. nil
	// disables it. Only one flight recorder may run per process, so
	// Start fails if another one is already active.
	FlightRecorder *FlightRecorderOption
}

// FlightRecorderOption configures the runtime/trace FlightRecorder
// that autopprof owns. Zero values fall back to the runtime defaults
// (10s / 10 MiB as of Go 1.26).
//
// The window ends at whichever limit is hit first: roughly the last
// MinAge of execution, cut short once it would exceed MaxBytes. Size
// MaxBytes from the process's trace rate (e.g. bytes from a 10s
// /debug/pprof/trace) times the lookback you want.
type FlightRecorderOption struct {
	// MinAge is how far back the window aims to reach. MaxBytes takes
	// precedence, so under heavy tracing load the window is shorter.
	MinAge time.Duration
	// MaxBytes bounds the size of the trace window. It is a hint, not
	// a hard cap on memory or snapshot size.
	MaxBytes uint64
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
	if o.ReportTimeout < 0 {
		return ErrInvalidReportTimeout
	}
	if o.WatchInterval < 0 {
		return ErrInvalidWatchInterval
	}
	if o.ReportCooldown < 0 {
		return ErrInvalidReportCooldown
	}
	if o.FlightRecorder != nil && o.FlightRecorder.MinAge < 0 {
		return ErrInvalidFlightRecorderMinAge
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
