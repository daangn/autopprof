package report

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/slack-go/slack"
)

type slackReporter struct {
	app     string
	channel string

	client *slack.Client
}

// SlackReporterOption is the option for the Slack reporter.
type SlackReporterOption struct {
	Token   string
	Channel string
}

func newSlackReporter(app string, opt *SlackReporterOption) *slackReporter {
	return &slackReporter{
		app:     app,
		channel: opt.Channel,
		client:  slack.New(opt.Token),
	}
}

// ReportMem sends the heap profile report to the Slack.
func (s *slackReporter) ReportMem(ctx context.Context, r io.Reader) error {
	hostname, _ := os.Hostname() // Ignore the error intentionally.
	var (
		now      = time.Now().Format(reportTimeLayout)
		filename = fmt.Sprintf(heapProfileFilenameFmt, s.app, hostname, now)
	)

	// There must be context keys for the memory threshold and the usage.
	var (
		memThreshold, _ = ctx.Value(MemThresholdCtxKey).(float64)
		memUsage, _     = ctx.Value(MemUsageCtxKey).(float64)
	)

	_, err := s.client.UploadFileContext(ctx, slack.FileUploadParameters{
		Reader:         r,
		Filename:       filename,
		Title:          filename,
		InitialComment: fmt.Sprintf(memCommentFmt, memThreshold, memUsage),
		Channels:       []string{s.channel},
	})
	return err
}
