//go:build linux
// +build linux

package queryer

import (
	"testing"

	"github.com/containerd/cgroups"
)

func TestNewCgroupQueryer(t *testing.T) {
	mode := cgroups.Mode()
	_, err := NewCgroupQueryer()
	if mode == cgroups.Unavailable && err == nil {
		t.Errorf("newQueryer() = nil, want error")
	} else if err != nil {
		t.Errorf("newQueryer() = %v, want nil", err)
	}
}
