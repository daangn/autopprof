package autopprof

import "fmt"

// Errors.
var (
	ErrUnsupportedPlatform = fmt.Errorf(
		"autopprof: unsupported platform (only Linux is supported)",
	)
	ErrInvalidCPUThreshold = fmt.Errorf(
		"autopprof: cpu threshold value must be between 0 and 1",
	)
	ErrInvalidMemThreshold = fmt.Errorf(
		"autopprof: memory threshold value must be between 0 and 1",
	)
	ErrInvalidGoroutineThreshold = fmt.Errorf(
		"autopprof: goroutine threshold value must be greater than to 0",
	)
	ErrNilReporter         = fmt.Errorf("autopprof: Reporter can't be nil")
	ErrDisableAllProfiling = fmt.Errorf("autopprof: all profiling is disabled")
)
