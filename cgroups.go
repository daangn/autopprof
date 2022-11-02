//go:build linux
// +build linux

package autopprof

import (
	"github.com/containerd/cgroups"
)

type queryer interface {
	cpuUsage() (float64, error)
	memUsage() (float64, error)

	setCPUQuota() error
}

func newQueryer() (queryer, error) {
	switch cgroups.Mode() {
	case cgroups.Legacy:
		return newCgroupsV1(), nil
	case cgroups.Hybrid, cgroups.Unified:
		return newCgroupsV2(), nil
	}
	return nil, ErrCgroupsUnavailable
}
