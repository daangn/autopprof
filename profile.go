package autopprof

import (
	"bufio"
	"bytes"
	"runtime/pprof"
	"time"
)

func (ap *autoPprof) profileCPU() ([]byte, error) {
	var (
		buf bytes.Buffer
		w   = bufio.NewWriter(&buf)
	)
	if err := pprof.StartCPUProfile(w); err != nil {
		return nil, err
	}
	time.Sleep(ap.cpuProfilingDuration) // Collect.
	pprof.StopCPUProfile()

	if err := w.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (ap *autoPprof) profileHeap() ([]byte, error) {
	var (
		buf bytes.Buffer
		w   = bufio.NewWriter(&buf)
	)
	if err := pprof.WriteHeapProfile(w); err != nil {
		return nil, err
	}
	if err := w.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
