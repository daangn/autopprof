//go:build linux
// +build linux

package autopprof

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"
)

const reportTimeLayout = "2006-01-02T150405.MST"

// Built-in Metric names. Exported so Reporter implementations can
// switch on ReportInfo.MetricName without string literals.
const (
	MetricNameCPU       = "cpu"
	MetricNameMem       = "mem"
	MetricNameGoroutine = "goroutine"
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

// Compile-time sanity check: collectBuiltIn returns a CollectResult
// whose Reader is an io.Reader (bytes.Reader also implements io.Seeker,
// which SlackReporter prefers for sized uploads).
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
