package autopprof

import (
	"fmt"
	"io"
	"runtime/trace"
	"sync"
)

// flightRecorder is the process-wide recorder started by Start. It
// lives outside autoPprof so WriteTrace never reads globalAp, which
// Stop clears without a lock.
//
// traceMu guards flightRecorder and is held across WriteTo: the stdlib
// rejects concurrent WriteTo calls instead of queueing them, and its
// Stop resets the ring without waiting for an in-flight WriteTo.
var (
	traceMu        sync.Mutex
	flightRecorder *trace.FlightRecorder
)

func startFlightRecorder(opt *FlightRecorderOption) error {
	if opt == nil {
		return nil
	}
	fr := trace.NewFlightRecorder(trace.FlightRecorderConfig{
		MinAge:   opt.MinAge,
		MaxBytes: opt.MaxBytes,
	})
	if err := fr.Start(); err != nil {
		return fmt.Errorf("autopprof: start flight recorder: %w", err)
	}
	traceMu.Lock()
	flightRecorder = fr
	traceMu.Unlock()
	return nil
}

func stopFlightRecorder() {
	traceMu.Lock()
	defer traceMu.Unlock()
	if flightRecorder == nil {
		return
	}
	flightRecorder.Stop()
	flightRecorder = nil
}

// WriteTrace writes the current flight recorder window to w as a Go
// execution trace (readable with `go tool trace`). Concurrent calls are
// serialized. Returns ErrFlightRecorderDisabled when
// Option.FlightRecorder was nil or autopprof is not running.
func WriteTrace(w io.Writer) (int64, error) {
	traceMu.Lock()
	defer traceMu.Unlock()
	if flightRecorder == nil {
		return 0, ErrFlightRecorderDisabled
	}
	return flightRecorder.WriteTo(w)
}
