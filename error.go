package autopprof

import "errors"

var (
	ErrUnsupportedPlatform = errors.New(
		"autopprof: unsupported platform (only Linux is supported)",
	)
	ErrInvalidCPUThreshold = errors.New(
		"autopprof: cpu threshold value must be between 0 and 1",
	)
	ErrInvalidMemThreshold = errors.New(
		"autopprof: memory threshold value must be between 0 and 1",
	)
	ErrInvalidGoroutineThreshold = errors.New(
		"autopprof: goroutine threshold value must be greater than to 0",
	)
	ErrInvalidReportTimeout = errors.New(
		"autopprof: report timeout must be a non-negative duration",
	)
	ErrInvalidWatchInterval = errors.New(
		"autopprof: watch interval must be a non-negative duration",
	)
	ErrInvalidReportCooldown = errors.New(
		"autopprof: report cooldown must be a non-negative duration",
	)
	ErrInvalidFlightRecorderMinAge = errors.New(
		"autopprof: flight recorder min age must be a non-negative duration",
	)
	ErrInvalidFlightRecorderMaxBytes = errors.New(
		"autopprof: flight recorder max bytes must be at most 256 MiB",
	)
	ErrNilReporter         = errors.New("autopprof: Reporter can't be nil")
	ErrDisableAllProfiling = errors.New("autopprof: all profiling is disabled")

	ErrInvalidMetric = errors.New(
		"autopprof: metric is invalid (nil, empty name, negative threshold/interval, or nil query/collect)",
	)
	ErrNotStarted = errors.New(
		"autopprof: Start() must be called before Register",
	)
	ErrInvalidCPUProfileDuration = errors.New(
		"autopprof: cpu profile duration must be positive",
	)
	ErrFlightRecorderDisabled = errors.New(
		"autopprof: flight recorder is disabled (Option.FlightRecorder is nil or autopprof is not running)",
	)
)
