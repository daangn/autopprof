package autopprof

import (
	"testing"
)

func TestDefaultProfiler_ProfileCPU(t *testing.T) {
	p := newDefaultProfiler(defaultCPUProfilingDuration)
	b, err := p.profileCPU()
	if err != nil {
		t.Errorf("profileCPU() = %v, want %v", err, nil)
		t.FailNow()
	}
	if len(b) == 0 {
		t.Error("len of cpu profile bytes= 0, want > 0")
	}
}

func TestDefaultProfiler_ProfileHeap(t *testing.T) {
	p := newDefaultProfiler(defaultCPUProfilingDuration)
	b, err := p.profileHeap()
	if err != nil {
		t.Errorf("profileHeap() = %v, want %v", err, nil)
		t.FailNow()
	}
	if len(b) == 0 {
		t.Error("len of heap profile bytes= 0, want > 0")
	}
}
