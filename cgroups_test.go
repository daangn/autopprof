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
