//go:build linux
// +build linux

package autopprof

import (
	"testing"

	"github.com/containerd/cgroups"
)

func TestNewQueryer(t *testing.T) {
	mode := cgroups.Mode()
	_, err := newQueryer()
	if mode == cgroups.Unavailable && err == nil {
		t.Errorf("newQueryer() = nil, want error")
	} else if err != nil {
		t.Errorf("newQueryer() = %v, want nil", err)
	}
}

func TestCgroupsV1_memUsage(t *testing.T) {
	mode := cgroups.Mode()
	if mode != cgroups.Legacy {
		t.Skip("cgroup v1 is not available")
	}
	usage, err := newCgroupsV1().memUsage()
	if err != nil {
		t.Errorf("memUsageV1() = %v, want nil", err)
	}
	if usage < 0 || usage > 1 {
		t.Errorf("memUsageV1() = %f, want between 0 and 1", usage)
	}
}

func TestCgroupsV2_memUsage(t *testing.T) {
	mode := cgroups.Mode()
	if mode != cgroups.Hybrid && mode != cgroups.Unified {
		t.Skip("cgroup v2 is not available")
	}
	usage, err := newCgroupsV2().memUsage()
	if err != nil {
		t.Errorf("memUsageV2() = %v, want nil", err)
	}
	if usage < 0 || usage > 1 {
		t.Errorf("memUsageV2() = %f, want between 0 and 1", usage)
	}
}
