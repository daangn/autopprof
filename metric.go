package autopprof

import (
	"io"
	"time"
)

// CollectResult is the payload Metric.Collect hands to autopprof for
// forwarding to the Reporter.
//   - Reader is the bytes to upload. A nil Reader means "handled
//     internally, skip the Reporter call" — useful for side-effect-only
//     hooks that already sent their data elsewhere.
//   - Filename is optional. If empty, autopprof generates a default
//     filename from the Metric's name and the current timestamp.
//   - Comment is optional. If empty, autopprof generates a default
//     comment from the metric name, value, and threshold.
//
// Reporter implementations can override Filename/Comment by inspecting
// ReportInfo.Value/Threshold/MetricName instead.
type CollectResult struct {
	Reader   io.Reader
	Filename string
	Comment  string
}

// Metric is the unified abstraction for every threshold-triggered data
// collection autopprof performs. CPU, memory, and goroutine-count
// watchers are pre-defined Metric implementations built from Option's
// threshold fields at Start time. Users register additional Metrics
// via Option.Metrics or the package-level Register/Unregister
// functions.
//
// Thread-safety: for user-registered Metrics, autopprof only calls
// Query and Collect from that Metric's own watcher goroutine, so
// implementations do not need internal synchronization. (The cascade
// triggered by Option.ReportAll touches only the built-in metrics.)
//
// Stable meta: Name, Threshold, and Interval are read once at
// registration and cached; changing their return values afterwards
// has no effect. Interval() == 0 means "use the global watchInterval
// (default 5s)".
type Metric interface {
	Name() string
	Threshold() float64
	Interval() time.Duration
	Query() (float64, error)
	Collect(value float64) (CollectResult, error)
}

// NewMetric is a convenience constructor for ad-hoc metrics that don't
// warrant their own struct. Nil query/collect functions are defended
// against: the returned Metric surfaces ErrInvalidMetric at call time
// instead of panicking.
func NewMetric(
	name string,
	threshold float64,
	interval time.Duration,
	query func() (float64, error),
	collect func(value float64) (CollectResult, error),
) Metric {
	if query == nil {
		query = func() (float64, error) { return 0, ErrInvalidMetric }
	}
	if collect == nil {
		collect = func(float64) (CollectResult, error) {
			return CollectResult{}, ErrInvalidMetric
		}
	}
	return &basicMetric{
		name:      name,
		threshold: threshold,
		interval:  interval,
		query:     query,
		collect:   collect,
	}
}

type basicMetric struct {
	name      string
	threshold float64
	interval  time.Duration
	query     func() (float64, error)
	collect   func(value float64) (CollectResult, error)
}

func (b *basicMetric) Name() string                             { return b.name }
func (b *basicMetric) Threshold() float64                       { return b.threshold }
func (b *basicMetric) Interval() time.Duration                  { return b.interval }
func (b *basicMetric) Query() (float64, error)                  { return b.query() }
func (b *basicMetric) Collect(v float64) (CollectResult, error) { return b.collect(v) }

// validateMetric is shared by Option.validate and registerMetric.
// Query/Collect nil defense is handled inside NewMetric.
func validateMetric(m Metric) error {
	if m == nil {
		return ErrInvalidMetric
	}
	if m.Name() == "" || m.Threshold() < 0 || m.Interval() < 0 {
		return ErrInvalidMetric
	}
	return nil
}
