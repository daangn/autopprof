package queryer

import "fmt"

// Errors.
var (
	ErrCgroupsUnavailable  = fmt.Errorf("autopprof: cgroups is unavailable")
	ErrV2CPUQuotaUndefined = fmt.Errorf("autopprof: v2 cpu quota is undefined")
	ErrV2CPUMaxEmpty       = fmt.Errorf("autopprof: v2 cpu.max is empty")
	ErrV1CPUSubsystemEmpty = fmt.Errorf("autopprof: v1 cpu subsystem is empty")
)
