//go:build !linux
// +build !linux

package autopprof

import (
	"testing"
)

func TestMemUsage(t *testing.T) {
	_, err := memUsage()
	if err == nil {
		t.Errorf("memUsage() = %v, want %v", err, ErrUnsupportedPlatform)
	}
}
