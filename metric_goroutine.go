//go:build linux
// +build linux

package autopprof

import (
	"fmt"
	"time"

	"github.com/daangn/autopprof/v2/queryer"
)

const (
	MetricNameGoroutine = "goroutine"

	goroutineProfileFilenameFmt = "pprof.%s.%s.goroutine.%s.pprof"
	goroutineCommentFmt         = ":rotating_light:[GOROUTINE] count (*%d*) > threshold (*%d*)"
)

// goroutineMetric keeps its threshold as int to mirror
// Option.GoroutineThreshold; the int(value) cast in Collect preserves
// the integer-formatted legacy comment.
type goroutineMetric struct {
	app       string
	threshold int
	rt        queryer.RuntimeQueryer
	p         profiler
}

func (m *goroutineMetric) Name() string            { return MetricNameGoroutine }
func (m *goroutineMetric) Threshold() float64      { return float64(m.threshold) }
func (m *goroutineMetric) Interval() time.Duration { return 0 }
func (m *goroutineMetric) Query() (float64, error) {
	return float64(m.rt.GoroutineCount()), nil
}

func (m *goroutineMetric) Collect(value float64) (CollectResult, error) {
	return collectProfile(
		m.app, goroutineProfileFilenameFmt,
		m.p.profileGoroutine,
		fmt.Sprintf(goroutineCommentFmt, int(value), m.threshold),
	)
}
