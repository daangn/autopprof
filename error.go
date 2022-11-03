package autopprof

import "fmt"

// Errors.
var (
	ErrUnsupportedPlatform = fmt.Errorf(
		"autopprof: unsupported platform (only Linux is supported)",
	)
	ErrCgroupsUnavailable  = fmt.Errorf("autopprof: cgroups is unavailable")
	ErrInvalidCPUThreshold = fmt.Errorf(
		"autopprof: cpu threshold value must be between 0 and 1",
	)
	ErrInvalidMemThreshold = fmt.Errorf(
		"autopprof: memory threshold value must be between 0 and 1",
	)
	ErrNilReporter         = fmt.Errorf("autopprof: Reporter can't be nil")
	ErrDisableAllProfiling = fmt.Errorf("autopprof: all profiling is disabled")
	ErrV2CPUQuotaUndefined = fmt.Errorf("autopprof: v2 cpu quota is undefined")
	ErrV2CPUMaxEmpty       = fmt.Errorf("autopprof: v2 cpu.max is empty")
	ErrV1CPUSubsystemEmpty = fmt.Errorf("autopprof: v1 cpu subsystem is empty")
)
