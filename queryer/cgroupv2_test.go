//go:build linux
// +build linux

package queryer

import (
	"testing"
	"time"

	"github.com/containerd/cgroups"
)

func TestCgroupV2_CpuUsage(t *testing.T) {
	mode := cgroups.Mode()
	if mode != cgroups.Hybrid && mode != cgroups.Unified {
		t.Skip("cgroup v2 is not available")
	}
	cgv2 := newCgroupsV2()
	cgv2.cpuQuota = 2
	cgv2.q = newCPUUsageSnapshotQueue(3)

	usage, err := cgv2.CpuUsage()
	if err != nil {
		t.Errorf("CpuUsage() = %v, want nil", err)
	}
	if usage != 0 { // The cpu usage is 0 until the queue is full.
		t.Errorf("CpuUsage() = %f, want 0", usage)
	}

	time.Sleep(1050 * time.Millisecond)

	usage, err = cgv2.CpuUsage()
	if err != nil {
		t.Errorf("CpuUsage() = %v, want nil", err)
	}
	if usage != 0 { // The cpu usage is 0 until the queue is full.
		t.Errorf("CpuUsage() = %f, want 0", usage)
	}

	time.Sleep(1050 * time.Millisecond)

	usage, err = cgv2.CpuUsage()
	if err != nil {
		t.Errorf("CpuUsage() = %v, want nil", err)
	}
	if usage < 0 || usage > 1 {
		t.Errorf("CpuUsage() = %f, want between 0 and 1", usage)
	}
}

func TestCgroupV2_MemUsage(t *testing.T) {
	mode := cgroups.Mode()
	if mode != cgroups.Hybrid && mode != cgroups.Unified {
		t.Skip("cgroup v2 is not available")
	}
	cgv2 := newCgroupsV2()
	usage, err := cgv2.MemUsage()
	if err != nil {
		t.Errorf("MemUsage() = %v, want nil", err)
	}
	if usage < 0 || usage > 1 {
		t.Errorf("MemUsage() = %f, want between 0 and 1", usage)
	}
}

func TestCgroupV2_SetCPUQuota(t *testing.T) {
	mode := cgroups.Mode()
	if mode != cgroups.Hybrid && mode != cgroups.Unified {
		t.Skip("cgroup v2 is not available")
	}
	cgv2 := newCgroupsV2()
	if err := cgv2.SetCPUQuota(); err != nil {
		t.Errorf("SetCPUQuota() = %v, want nil", err)
	}
	// The cpu quota of test docker container is 1.5.
	if cgv2.cpuQuota != 1.5 {
		t.Errorf("cpuQuota = %f, want 1.5", cgv2.cpuQuota)
	}
}
