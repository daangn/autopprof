//go:build linux
// +build linux

package autopprof

import (
	"fmt"
	"time"

	"github.com/daangn/autopprof/v2/queryer"
)

const (
	MetricNameMem = "mem"

	heapProfileFilenameFmt = "pprof.%s.%s.alloc_objects.alloc_space.inuse_objects.inuse_space.%s.pprof"
	memCommentFmt          = ":rotating_light:[MEM] usage (*%.2f%%*) > threshold (*%.2f%%*)"
)

type memMetric struct {
	app       string
	threshold float64
	cg        queryer.CgroupsQueryer
	p         profiler
}

func (m *memMetric) Name() string            { return MetricNameMem }
func (m *memMetric) Threshold() float64      { return m.threshold }
func (m *memMetric) Interval() time.Duration { return 0 }
func (m *memMetric) Query() (float64, error) { return m.cg.MemUsage() }

func (m *memMetric) Collect(value float64) (CollectResult, error) {
	return collectProfile(
		m.app, heapProfileFilenameFmt,
		m.p.profileHeap,
		fmt.Sprintf(memCommentFmt, value*100, m.threshold*100),
	)
}
