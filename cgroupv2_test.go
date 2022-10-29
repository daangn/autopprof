//go:build linux
// +build linux

package autopprof

import (
	"testing"

	"github.com/containerd/cgroups"
)

func TestCgroupV2_cpuUsage(t *testing.T) {
	mode := cgroups.Mode()
	if mode != cgroups.Hybrid && mode != cgroups.Unified {
		t.Skip("cgroup v2 is not available")
	}
	cgv2 := newCgroupsV2()
	cgv2.cpuQuota = 2
	usage, err := cgv2.cpuUsage()
	if err != nil {
		t.Errorf("cpuUsage() = %v, want nil", err)
	}
	if usage < 0 || usage > 1 {
		t.Errorf("cpuUsage() = %f, want between 0 and 1", usage)
	}
}

func TestCgroupV2_memUsage(t *testing.T) {
	mode := cgroups.Mode()
	if mode != cgroups.Hybrid && mode != cgroups.Unified {
		t.Skip("cgroup v2 is not available")
	}
	cgv2 := newCgroupsV2()
	usage, err := cgv2.memUsage()
	if err != nil {
		t.Errorf("memUsage() = %v, want nil", err)
	}
	if usage < 0 || usage > 1 {
		t.Errorf("memUsage() = %f, want between 0 and 1", usage)
	}
}

func TestCgroupV2_setCPUQuota(t *testing.T) {
	mode := cgroups.Mode()
	if mode != cgroups.Hybrid && mode != cgroups.Unified {
		t.Skip("cgroup v2 is not available")
	}
	cgv2 := newCgroupsV2()
	if err := cgv2.setCPUQuota(); err != nil {
		t.Errorf("setCPUQuota() = %v, want nil", err)
	}
	// The cpu quota of test docker container is 1.5.
	if cgv2.cpuQuota != 1.5 {
		t.Errorf("cpuQuota = %f, want 1.5", cgv2.cpuQuota)
	}
}
