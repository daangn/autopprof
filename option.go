package autopprof

import (
	"time"

	"github.com/daangn/autopprof/report"
)

const (
	defaultMemThreshold = 0.75

	defaultScanInterval = 5 * time.Second

	defaultMinConsecutiveOverThreshold = 12 // min 1 minute. (12*5s)
)

// Option is the configuration for the autopprof.
type Option struct {
	// App is the name of the application to be used in the filename.
	App string

	// MemThreshold is the memory usage threshold (between 0 and 1)
	//  to trigger the heap profiling.
	// Autopprof will start the heap profiling when the memory usage
	//  is higher than this threshold.
	MemThreshold float64

	// Reporter is the reporter option to send the profiling report.
	Reporter report.ReporterOption
}

func (o Option) validate() error {
	if o.MemThreshold < 0 || o.MemThreshold > 1 {
		return ErrInvalidMemThreshold
	}
	return nil
}
