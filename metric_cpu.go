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
	// cpuTriggerCommentFmt is used when the metric itself breaches its
	// threshold (value >= threshold). The `>` then describes reality.
	cpuTriggerCommentFmt = ":rotating_light:[CPU] usage (*%.2f%%*) > threshold (*%.2f%%*)"
	// cpuCascadeCommentFmt is used for cascade companions that are
	// captured alongside another metric's trigger but did not breach
	// their own threshold. The em-dash avoids the false `>` claim.
	cpuCascadeCommentFmt = ":mag:[CPU] usage (*%.2f%%*) — threshold (*%.2f%%*)"
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
	commentFmt := cpuCascadeCommentFmt
	if value >= m.threshold {
		commentFmt = cpuTriggerCommentFmt
	}
	return collectProfile(
		m.app, cpuProfileFilenameFmt,
		m.p.profileCPU,
		fmt.Sprintf(commentFmt, value*100, m.threshold*100),
	)
}
