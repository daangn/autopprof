package autopprof

import (
	"testing"
	"time"
)

func TestAutoPprof_ProfileCPU(t *testing.T) {
	ap := &autoPprof{
		cpuProfilingDuration: 1 * time.Second,
	}
	b, err := ap.profileCPU()
	if err != nil {
		t.Errorf("profileCPU() = %v, want %v", err, nil)
		t.FailNow()
	}
	if len(b) == 0 {
		t.Error("len of cpu profile bytes= 0, want > 0")
	}
}

func TestAutoPprof_ProfileHeap(t *testing.T) {
	ap := &autoPprof{}
	b, err := ap.profileHeap()
	if err != nil {
		t.Errorf("profileHeap() = %v, want %v", err, nil)
		t.FailNow()
	}
	if len(b) == 0 {
		t.Error("len of heap profile bytes= 0, want > 0")
	}
}
