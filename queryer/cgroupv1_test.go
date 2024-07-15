//go:build linux
// +build linux

package queryer

import (
	"testing"
	"time"

	"github.com/containerd/cgroups"
)

func TestCgroupV1_CPUUsage(t *testing.T) {
	mode := cgroups.Mode()
	if mode != cgroups.Legacy {
		t.Skip("cgroup v1 is not available")
	}
	cgv1 := newCgroupsV1()
	cgv1.cpuQuota = 2
	cgv1.q = newCPUUsageSnapshotQueue(3)

	usage, err := cgv1.CPUUsage()
	if err != nil {
		t.Errorf("CPUUsage() = %v, want nil", err)
	}
	if usage != 0 { // The cpu usage is 0 until the queue is full.
		t.Errorf("CPUUsage() = %f, want 0", usage)
	}

	time.Sleep(1050 * time.Millisecond)

	usage, err = cgv1.CPUUsage()
	if err != nil {
		t.Errorf("CPUUsage() = %v, want nil", err)
	}
	if usage != 0 { // The cpu usage is 0 until the queue is full.
		t.Errorf("CPUUsage() = %f, want 0", usage)
	}

	time.Sleep(1050 * time.Millisecond)

	usage, err = cgv1.CPUUsage()
	if err != nil {
		t.Errorf("CPUUsage() = %v, want nil", err)
	}
	if usage < 0 || usage > 1 {
		t.Errorf("CPUUsage() = %f, want between 0 and 1", usage)
	}
}

func TestCgroupV1_MemUsage(t *testing.T) {
	mode := cgroups.Mode()
	if mode != cgroups.Legacy {
		t.Skip("cgroup v1 is not available")
	}
	usage, err := newCgroupsV1().MemUsage()
	if err != nil {
		t.Errorf("MemUsage() = %v, want nil", err)
	}
	if usage < 0 || usage > 1 {
		t.Errorf("MemUsage() = %f, want between 0 and 1", usage)
	}
}

func TestCgroupV1_SetCPUQuota(t *testing.T) {
	mode := cgroups.Mode()
	if mode != cgroups.Legacy {
		t.Skip("cgroup v1 is not available")
	}
	cgv1 := newCgroupsV1()
	if err := cgv1.SetCPUQuota(); err != nil {
		t.Errorf("SetCPUQuota() = %v, want nil", err)
	}
	// The cpu quota of test docker container is 1.5.
	if cgv1.cpuQuota != 1.5 {
		t.Errorf("cpuQuota = %f, want 1.5", cgv1.cpuQuota)
	}
}
