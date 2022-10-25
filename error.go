package autopprof

import "fmt"

// Errors.
var (
	ErrUnsupportedPlatform = fmt.Errorf(
		"autopprof: unsupported platform (only Linux is supported)",
	)
	ErrCgroupsUnavailable  = fmt.Errorf("autopprof: cgroups is unavailable")
	ErrInvalidMemThreshold = fmt.Errorf(
		"autopprof: memory threshold value must be between 0 and 1",
	)
	ErrNilReporter = fmt.Errorf("autopprof: Reporter can't be nil")
)
