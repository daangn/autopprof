//go:build linux
// +build linux

package queryer

import (
	"github.com/containerd/cgroups"
)

//go:generate mockgen -source=queryer.go -destination=queryer_mock.go -package=queryer

const (
	cpuUsageSnapshotQueueSize = 24 // 24 * 5s = 2 minutes.
)

type CgroupsQueryer interface {
	CPUUsage() (float64, error)
	MemUsage() (float64, error)

	SetCPUQuota() error
}

type RuntimeQueryer interface {
	GoroutineCount() int
}

func NewCgroupQueryer() (CgroupsQueryer, error) {
	switch cgroups.Mode() {
	case cgroups.Legacy:
		return newCgroupsV1(), nil
	case cgroups.Hybrid, cgroups.Unified:
		return newCgroupsV2(), nil
	}
	return nil, ErrCgroupsUnavailable
}

func NewRuntimeQueryer() (RuntimeQueryer, error) {
	return newRuntimeQuery(), nil
}
