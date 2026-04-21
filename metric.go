package autopprof

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"
)

const reportTimeLayout = "2006-01-02T150405.MST"

// CollectResult is the payload Metric.Collect hands to autopprof.
// Reader == nil means "handled internally, skip the Reporter call"
// (useful for side-effect-only hooks). Empty Filename/Comment are
// filled in with autopprof defaults.
type CollectResult struct {
	Reader   io.Reader
	Filename string
	Comment  string
}

// Metric is the unified abstraction for every threshold-triggered
// data collection autopprof performs. Built-in CPU/Mem/Goroutine
// watchers are pre-defined implementations; users register additional
// Metrics via Option.Metrics or autopprof.Register.
//
// Thread-safety: autopprof only calls Query and Collect from the
// Metric's own watcher goroutine, so implementations do not need
// internal synchronization. (The ReportAll cascade touches only
// built-ins.)
//
// Name/Threshold/Interval are read once at registration. Interval == 0
// means "use the global watchInterval (default 5s)".
type Metric interface {
	Name() string
	Threshold() float64
	Interval() time.Duration
	Query() (float64, error)
	Collect(value float64) (CollectResult, error)
}

// NewMetric is a convenience constructor. Nil query/collect surface
// ErrInvalidMetric at call time instead of panicking.
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

func validateMetric(m Metric) error {
	if m == nil {
		return ErrInvalidMetric
	}
	if m.Name() == "" || m.Threshold() < 0 || m.Interval() < 0 {
		return ErrInvalidMetric
	}
	return nil
}

func hostnameSafe() string {
	h, _ := os.Hostname()
	return h
}

func collectProfile(
	app, filenameFmt string,
	profile func() ([]byte, error),
	comment string,
) (CollectResult, error) {
	b, err := profile()
	if err != nil {
		return CollectResult{}, err
	}
	now := time.Now().Format(reportTimeLayout)
	return CollectResult{
		Reader:   bytes.NewReader(b),
		Filename: fmt.Sprintf(filenameFmt, app, hostnameSafe(), now),
		Comment:  comment,
	}, nil
}

var _ io.Reader = (*bytes.Reader)(nil)

// defaultFilename is used when Collect returns an empty Filename. The
// ".bin" extension signals "opaque bytes" to Reporter implementations
// that don't recognize the metric name.
func defaultFilename(metricName string) string {
	return fmt.Sprintf(
		"%s.%s.%s.bin",
		metricName, hostnameSafe(), time.Now().Format(reportTimeLayout),
	)
}

func defaultComment(metricName string, value, threshold float64) string {
	return fmt.Sprintf(
		":rotating_light:[%s] value=%.2f threshold=%.2f",
		metricName, value, threshold,
	)
}
