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
	reportTimeLayout = "2006-01-02T150405.MST"

	cpuCommentFmt       = ":rotating_light:[CPU] usage (*%.2f%%*) > threshold (*%.2f%%*)"
	memCommentFmt       = ":rotating_light:[MEM] usage (*%.2f%%*) > threshold (*%.2f%%*)"
	goroutineCommentFmt = ":rotating_light:[GOROUTINE] count (*%d*) > threshold (*%d*)"
)

// SlackReporter is the reporter to send the profiling report to the
// specific Slack channel.
type SlackReporter struct {
	app       string
	channelID string

	client *slack.Client
}

// SlackReporterOption is the option for the Slack reporter.
type SlackReporterOption struct {
	App       string
	Token     string
	ChannelID string
}

// NewSlackReporter returns the new SlackReporter.
func NewSlackReporter(opt *SlackReporterOption) *SlackReporter {
	return &SlackReporter{
		app:       opt.App,
		channelID: opt.ChannelID,
		client:    slack.New(opt.Token),
	}
}

// ReportCPUProfile sends the CPU profiling data to the Slack.
func (s *SlackReporter) ReportCPUProfile(
	ctx context.Context, r io.Reader, size int, ci CPUInfo,
) error {
	hostname, _ := os.Hostname() // Don't care about this error.
	var (
		now      = time.Now().Format(reportTimeLayout)
		filename = fmt.Sprintf(CPUProfileFilenameFmt, s.app, hostname, now)
		comment  = fmt.Sprintf(cpuCommentFmt, ci.UsagePercentage, ci.ThresholdPercentage)
	)
	if _, err := s.client.UploadFileV2Context(ctx, slack.UploadFileV2Parameters{
		Reader:         r,
		Filename:       filename,
		FileSize:       size,
		Title:          filename,
		InitialComment: comment,
		Channel:        s.channelID,
	}); err != nil {
		return fmt.Errorf("autopprof: failed to upload a file to Slack channel: %w", err)
	}
	return nil
}

// ReportHeapProfile sends the heap profiling data to the Slack.
func (s *SlackReporter) ReportHeapProfile(
	ctx context.Context, r io.Reader, size int, mi MemInfo,
) error {
	hostname, _ := os.Hostname() // Don't care about this error.
	var (
		now      = time.Now().Format(reportTimeLayout)
		filename = fmt.Sprintf(HeapProfileFilenameFmt, s.app, hostname, now)
		comment  = fmt.Sprintf(memCommentFmt, mi.UsagePercentage, mi.ThresholdPercentage)
	)
	if _, err := s.client.UploadFileV2Context(ctx, slack.UploadFileV2Parameters{
		Reader:         r,
		Filename:       filename,
		FileSize:       size,
		Title:          filename,
		InitialComment: comment,
		Channel:        s.channelID,
	}); err != nil {
		return fmt.Errorf("autopprof: failed to upload a file to Slack channel: %w", err)
	}
	return nil
}

// ReportGoroutineProfile sends the goroutine profiling data to the Slack.
func (s *SlackReporter) ReportGoroutineProfile(
	ctx context.Context, r io.Reader, size int, gi GoroutineInfo,
) error {
	hostname, _ := os.Hostname() // Don't care about this error.
	var (
		now      = time.Now().Format(reportTimeLayout)
		filename = fmt.Sprintf(GoroutineProfileFilenameFmt, s.app, hostname, now)
		comment  = fmt.Sprintf(goroutineCommentFmt, gi.Count, gi.ThresholdCount)
	)
	if _, err := s.client.UploadFileV2Context(ctx, slack.UploadFileV2Parameters{
		Reader:         r,
		Filename:       filename,
		FileSize:       size,
		Title:          filename,
		InitialComment: comment,
		Channel:        s.channelID,
	}); err != nil {
		return fmt.Errorf("autopprof: failed to upload a file to Slack channel: %w", err)
	}
	return nil
}
