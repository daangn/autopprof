//go:build linux
// +build linux

package autopprof

import (
	"github.com/containerd/cgroups"
)

//go:generate mockgen -source=cgroups.go -destination=cgroups_mock.go -package=autopprof

const (
	cpuUsageSnapshotQueueSize = 24 // 24 * 5s = 2 minutes.
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
