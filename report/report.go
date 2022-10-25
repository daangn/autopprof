package report

import (
	"context"
	"fmt"
	"io"
)

//go:generate mockgen -source=report.go -destination=report_mock.go -package=report

// ReporterType is the type of the reporter.
type ReporterType string

const (
	SLACK ReporterType = "SLACK"
)

const (
	reportTimeLayout = "2006-01-02T15:04:05.MST"

	// pprof.<app>.<hostname>.alloc_objects.alloc_space.inuse_objects.inuse_space.<formattedTime>.pprof
	heapProfileFilenameFmt = "pprof.%s.%s.alloc_objects.alloc_space.inuse_objects.inuse_space.%s.pprof"

	memCommentFmt = "mem_threshold: %.2f (%%), mem_usage: %.2f (%%)"
)

var (
	errUnknownReporterType   = fmt.Errorf("autopprof: unknown reporter type")
	errNoAppName             = fmt.Errorf("autopprof: empty app name to report")
	errNoSlackReporterOption = fmt.Errorf("autopprof: no slack reporter option")
)

// Reporter is the interface for the reporter.
// Reporter is responsible for reporting the profiling data report to the destination.
type Reporter interface {
	ReportMem(ctx context.Context, r io.Reader) error
}

// NewReporter creates a new reporter.
func NewReporter(app string, opt ReporterOption) (Reporter, error) {
	if app == "" {
		return nil, errNoAppName
	}
	switch opt.Type {
	case SLACK:
		if opt.SlackReporterOption == nil {
			return nil, errNoSlackReporterOption
		}
		return newSlackReporter(app, opt.SlackReporterOption), nil
	default:
		return nil, errUnknownReporterType
	}
}
