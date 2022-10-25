//go:build linux
// +build linux

package autopprof

import (
	"github.com/containerd/cgroups"
	cgroupsv2 "github.com/containerd/cgroups/v2"
)

const (
	cgroupMountPoint = "/sys/fs/cgroup"
)

func memUsage() (float64, error) {
	switch cgroups.Mode() {
	case cgroups.Legacy:
		return memUsageV1()
	case cgroups.Hybrid, cgroups.Unified:
		return memUsageV2()
	}
	return 0, ErrCgroupsUnavailable
}

func memUsageV1() (float64, error) {
	cg, err := cgroups.Load(cgroups.V1, cgroups.StaticPath("/"))
	if err != nil {
		return 0, err
	}
	stats, err := cg.Stat()
	if err != nil {
		return 0, err
	}
	workingSet := stats.Memory.Usage.Usage - stats.Memory.InactiveFile
	limit := stats.Memory.HierarchicalMemoryLimit
	return float64(workingSet) / float64(limit), nil
}

func memUsageV2() (float64, error) {
	path, err := cgroupsv2.NestedGroupPath("")
	if err != nil {
		return 0, err
	}
	manager, err := cgroupsv2.LoadManager(cgroupMountPoint, path)
	if err != nil {
		return 0, err
	}
	stats, err := manager.Stat()
	if err != nil {
		return 0, err
	}
	workingSet := stats.Memory.Usage - stats.Memory.InactiveFile
	limit := stats.Memory.UsageLimit
	return float64(workingSet) / float64(limit), nil
}
