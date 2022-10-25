//go:build linux
// +build linux

package autopprof

import (
	"github.com/containerd/cgroups"
	cgroupsv2 "github.com/containerd/cgroups/v2"
)

type queryer interface {
	memUsage() (float64, error)
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

type cgroupsV1 struct {
	staticPath string
}

func newCgroupsV1() *cgroupsV1 {
	return &cgroupsV1{
		staticPath: "/",
	}
}

func (c cgroupsV1) memUsage() (float64, error) {
	cg, err := cgroups.Load(cgroups.V1, cgroups.StaticPath("/"))
	if err != nil {
		return 0, err
	}
	stats, err := cg.Stat()
	if err != nil {
		return 0, err
	}
	var (
		memStat    = stats.Memory
		workingSet = memStat.Usage.Usage - memStat.InactiveFile
		limit      = memStat.HierarchicalMemoryLimit
	)
	return float64(workingSet) / float64(limit), nil
}

type cgroupsV2 struct {
	nestedGroupPath string
	mountPoint      string
}

func newCgroupsV2() *cgroupsV2 {
	return &cgroupsV2{
		nestedGroupPath: "",
		mountPoint:      "/sys/fs/cgroup",
	}
}

func (c cgroupsV2) memUsage() (float64, error) {
	path, err := cgroupsv2.NestedGroupPath(c.nestedGroupPath)
	if err != nil {
		return 0, err
	}
	manager, err := cgroupsv2.LoadManager(c.mountPoint, path)
	if err != nil {
		return 0, err
	}
	stats, err := manager.Stat()
	if err != nil {
		return 0, err
	}
	var (
		memStat    = stats.Memory
		workingSet = memStat.Usage - memStat.InactiveFile
		limit      = memStat.UsageLimit
	)
	return float64(workingSet) / float64(limit), nil
}
