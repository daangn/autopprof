package autopprof

import (
	"bufio"
	"bytes"
	"runtime/pprof"
)

func profileHeap() ([]byte, error) {
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
