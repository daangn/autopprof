//go:build linux
// +build linux

package autopprof

import (
	"testing"

	"github.com/containerd/cgroups"
)

func TestMemUsage(t *testing.T) {
	mode := cgroups.Mode()
	_, err := memUsage()
	if mode == cgroups.Unavailable && err == nil {
		t.Errorf("memUsage() = nil, want error")
	} else if err != nil {
		t.Errorf("memUsage() = %v, want nil", err)
	}
}

func TestMemUsageV1(t *testing.T) {
	mode := cgroups.Mode()
	if mode != cgroups.Legacy {
		t.Skip("cgroup v1 is not available")
	}
	usage, err := memUsageV1()
	if err != nil {
		t.Errorf("memUsageV1() = %v, want nil", err)
	}
	if usage < 0 || usage > 1 {
		t.Errorf("memUsageV1() = %f, want between 0 and 1", usage)
	}
}

func TestMemUsageV2(t *testing.T) {
	mode := cgroups.Mode()
	if mode != cgroups.Hybrid && mode != cgroups.Unified {
		t.Skip("cgroup v2 is not available")
	}
	usage, err := memUsageV2()
	if err != nil {
		t.Errorf("memUsageV2() = %v, want nil", err)
	}
	if usage < 0 || usage > 1 {
		t.Errorf("memUsageV2() = %f, want between 0 and 1", usage)
	}
}
