//go:build linux
// +build linux

package autopprof

import (
	"bytes"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/daangn/autopprof/v2/queryer"
)

const reportTimeLayout = "2006-01-02T150405.MST"

// Built-in Metric names. These are reserved — users cannot register
// a Metric with one of these names.
const (
	MetricNameCPU       = "cpu"
	MetricNameMem       = "mem"
	MetricNameGoroutine = "goroutine"
)

// Built-in filename formats. Kept unexported because v2 Reporter
// implementations should route on ReportInfo.MetricName rather than
// parsing filenames.
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

// cachedHostname avoids repeating the os.Hostname syscall on every
// Collect call (hostname doesn't change during process lifetime).
var (
	cachedHostname     string
	cachedHostnameOnce sync.Once
)

// hostnameSafe returns os.Hostname() or "" (matches the original
// slack.go behavior, which also discards the error). The result is
// cached after the first call — hostname doesn't change during the
// process lifetime and the syscall was being repeated on every
// threshold breach.
func hostnameSafe() string {
	cachedHostnameOnce.Do(func() {
		cachedHostname, _ = os.Hostname()
	})
	return cachedHostname
}

// collectBuiltIn is the shared shape of Collect for built-in metrics.
// It runs the profiler function, wraps the bytes in a reader, and
// assembles the filename (via the legacy report.XxxProfileFilenameFmt
// template) plus the comment. Having this helper keeps cpuMetric /
// memMetric / goroutineMetric's Collect thin so future built-ins (io,
// disk, …) slot in without duplicating the boilerplate.
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

// defaultFilename is used when a Metric's Collect returns an empty
// Filename. Keeps the ".bin" extension to signal "opaque bytes" to
// Reporter implementations that don't recognize the metric name.
func defaultFilename(metricName string) string {
	return fmt.Sprintf(
		"%s.%s.%s.bin",
		metricName, hostnameSafe(), time.Now().Format(reportTimeLayout),
	)
}

// defaultComment is used when a Metric's Collect returns an empty
// Comment. Produces a generic alert text with the current value and
// threshold so the message is still informative.
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
func (m *cpuMetric) Interval() time.Duration { return 0 } // use global
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

// goroutineMetric stores its threshold as int to mirror
// Option.GoroutineThreshold's type, but exposes it as float64 through
// the Metric interface. The int(value) cast in Collect preserves the
// legacy integer-formatted comment produced by the original reporter.
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
