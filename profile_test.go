package autopprof

import (
	"bytes"
	"errors"
	"io"
	"runtime/pprof"
	"sync"
	"testing"
	"time"
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

// pprofMagic is the gzip header every pprof-encoded profile starts with.
var pprofMagic = []byte{0x1f, 0x8b}

func TestProfileCPU(t *testing.T) {
	var buf bytes.Buffer
	if err := ProfileCPU(&buf, 50*time.Millisecond); err != nil {
		t.Fatalf("ProfileCPU: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), pprofMagic) {
		t.Errorf("profile does not start with the gzip header: % x", buf.Bytes()[:min(4, buf.Len())])
	}
}

func TestProfileCPU_invalidDuration(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		if err := ProfileCPU(io.Discard, d); !errors.Is(err, ErrInvalidCPUProfileDuration) {
			t.Errorf("ProfileCPU(d=%v) = %v, want ErrInvalidCPUProfileDuration", d, err)
		}
	}
}

// TestProfileCPU_waitsForBuiltin: pprof.StartCPUProfile fails fast on
// overlap ("cpu profiling already in use"); ProfileCPU and the built-in
// profiler must queue behind each other instead.
func TestProfileCPU_waitsForBuiltin(t *testing.T) {
	const d = 50 * time.Millisecond
	builtin := newDefaultProfiler(d)

	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 2; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, err := builtin.profileCPU()
			errs <- err
		}()
		go func() {
			defer wg.Done()
			errs <- ProfileCPU(io.Discard, d)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("overlapping CPU profile: %v", err)
		}
	}
}

// TestProfileCPU_heldOutsideAutopprof: the mutex can't cover callers
// that go straight to runtime/pprof; those must surface as an error,
// not a hang.
func TestProfileCPU_heldOutsideAutopprof(t *testing.T) {
	if err := pprof.StartCPUProfile(io.Discard); err != nil {
		t.Fatalf("StartCPUProfile: %v", err)
	}
	t.Cleanup(pprof.StopCPUProfile)

	if err := ProfileCPU(io.Discard, 50*time.Millisecond); err == nil {
		t.Error("ProfileCPU succeeded while another CPU profile was running")
	}
}
