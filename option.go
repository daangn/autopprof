package autopprof

import (
	"time"

	"github.com/daangn/autopprof/report"
)

const (
	defaultMemThreshold                = 0.75
	defaultScanInterval                = 5 * time.Second
	defaultMinConsecutiveOverThreshold = 12 // min 1 minute. (12*5s)
)

// Option is the configuration for the autopprof.
type Option struct {
	// MemThreshold is the memory usage threshold (between 0 and 1)
	//  to trigger the heap profiling.
	// Autopprof will start the heap profiling when the memory usage
	//  is higher than this threshold.
	MemThreshold float64

	// Reporter is the reporter to send the profiling report implementing
	//  the report.Reporter interface.
	Reporter report.Reporter
}

func (o Option) validate() error {
	if o.MemThreshold < 0 || o.MemThreshold > 1 {
		return ErrInvalidMemThreshold
	}
	if o.Reporter == nil {
		return ErrNilReporter
	}
	return nil
}
