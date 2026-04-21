package report

import (
	"context"
	"io"
)

//go:generate mockgen -source=report.go -destination=report_mock.go -package=report

// ReportInfo carries structured metadata about a report so Reporter
// implementations can route or re-format without parsing filenames
// or comments.
type ReportInfo struct {
	// MetricName is "cpu", "mem", "goroutine", or the user-supplied
	// name of a Metric registered via autopprof.Register /
	// Option.Metrics.
	MetricName string

	// Filename is what autopprof chose for the upload. If the Metric
	// returned a non-empty Filename via CollectResult it is used as-is;
	// otherwise autopprof fills in a default.
	Filename string

	// Comment is the human-readable message associated with the report.
	// Same filling rule as Filename.
	Comment string

	// Value is the latest Query() value that triggered this report.
	Value float64

	// Threshold is the Metric's configured threshold.
	Threshold float64
}

// Reporter sends a single profile/payload to its destination. Every
// Metric (built-in CPU/Mem/Goroutine or user-defined) routes through
// this one method. The caller (autopprof) provides a preformatted
// filename/comment via Metric.Collect, plus structured metadata in
// ReportInfo so the Reporter can decide how to present the message.
type Reporter interface {
	Report(ctx context.Context, r io.Reader, info ReportInfo) error
}
