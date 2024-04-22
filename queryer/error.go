package queryer

import "fmt"

// Errors.
var (
	ErrCgroupsUnavailable = fmt.Errorf("autopprof: cgroups is unavailable")
)
