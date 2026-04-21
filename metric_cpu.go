//go:build linux
// +build linux

package autopprof

import (
	"fmt"
	"time"

	"github.com/daangn/autopprof/v2/queryer"
)

const (
	MetricNameCPU = "cpu"

	cpuProfileFilenameFmt = "pprof.%s.%s.samples.cpu.%s.pprof"
	cpuCommentFmt         = ":rotating_light:[CPU] usage (*%.2f%%*) > threshold (*%.2f%%*)"
)

type cpuMetric struct {
	app       string
	threshold float64
	cg        queryer.CgroupsQueryer
	p         profiler
}

func (m *cpuMetric) Name() string            { return MetricNameCPU }
func (m *cpuMetric) Threshold() float64      { return m.threshold }
func (m *cpuMetric) Interval() time.Duration { return 0 }
func (m *cpuMetric) Query() (float64, error) { return m.cg.CPUUsage() }

func (m *cpuMetric) Collect(value float64) (CollectResult, error) {
	return collectProfile(
		m.app, cpuProfileFilenameFmt,
		m.p.profileCPU,
		fmt.Sprintf(cpuCommentFmt, value*100, m.threshold*100),
	)
}
