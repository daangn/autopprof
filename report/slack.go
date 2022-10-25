package report

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/slack-go/slack"
)

const (
	reportTimeLayout = "2006-01-02T15:04:05.MST"

	memCommentFmt = "mem_threshold: %.2f (%%), mem_usage: %.2f (%%)"
)

// SlackReporter is the reporter to send the profiling report to the
// specific Slack channel.
type SlackReporter struct {
	app     string
	channel string

	client *slack.Client
}

// SlackReporterOption is the option for the Slack reporter.
type SlackReporterOption struct {
	App     string
	Token   string
	Channel string
}

// NewSlackReporter returns the new SlackReporter.
func NewSlackReporter(opt *SlackReporterOption) *SlackReporter {
	return &SlackReporter{
		app:     opt.App,
		channel: opt.Channel,
		client:  slack.New(opt.Token),
	}
}

// ReportHeapProfile sends the heap profile report to the Slack.
func (s *SlackReporter) ReportHeapProfile(
	ctx context.Context, r io.Reader, mi MemInfo,
) error {
	hostname, _ := os.Hostname() // Don't care about this error.
	var (
		now      = time.Now().Format(reportTimeLayout)
		filename = fmt.Sprintf(HeapProfileFilenameFmt, s.app, hostname, now)
		comment  = fmt.Sprintf(memCommentFmt, mi.ThresholdPercentage, mi.UsagePercentage)
	)
	_, err := s.client.UploadFileContext(ctx, slack.FileUploadParameters{
		Reader:         r,
		Filename:       filename,
		Title:          filename,
		InitialComment: comment,
		Channels:       []string{s.channel},
	})
	return err
}
