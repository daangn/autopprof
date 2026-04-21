//go:build linux
// +build linux

package autopprof

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/daangn/autopprof/v2/queryer"
)

const reportTimeLayout = "2006-01-02T150405.MST"

// Built-in Metric names. Exported so Reporter implementations can
// switch on ReportInfo.MetricName without string literals.
const (
	MetricNameCPU       = "cpu"
	MetricNameMem       = "mem"
	MetricNameGoroutine = "goroutine"
)

const (
	cpuProfileFilenameFmt       = "pprof.%s.%s.samples.cpu.%s.pprof"
	heapProfileFilenameFmt      = "pprof.%s.%s.alloc_objects.alloc_space.inuse_objects.inuse_space.%s.pprof"
	goroutineProfileFilenameFmt = "pprof.%s.%s.goroutine.%s.pprof"
)

const (
	cpuCommentFmt       = ":rotating_light:[CPU] usage (*%.2f%%*) > threshold (*%.2f%%*)"
	memCommentFmt       = ":rotating_light:[MEM] usage (*%.2f%%*) > threshold (*%.2f%%*)"
	goroutineCommentFmt = ":rotating_light:[GOROUTINE] count (*%d*) > threshold (*%d*)"
)

func hostnameSafe() string {
	h, _ := os.Hostname()
	return h
}

func collectBuiltIn(
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

// ---------- cpuMetric ----------

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
	return collectBuiltIn(
		m.app, cpuProfileFilenameFmt,
		m.p.profileCPU,
		fmt.Sprintf(cpuCommentFmt, value*100, m.threshold*100),
	)
}

// ---------- memMetric ----------

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
	return collectBuiltIn(
		m.app, heapProfileFilenameFmt,
		m.p.profileHeap,
		fmt.Sprintf(memCommentFmt, value*100, m.threshold*100),
	)
}

// ---------- goroutineMetric ----------

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
	return collectBuiltIn(
		m.app, goroutineProfileFilenameFmt,
		m.p.profileGoroutine,
		fmt.Sprintf(goroutineCommentFmt, int(value), m.threshold),
	)
}
