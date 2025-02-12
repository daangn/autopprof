package report

import (
	"bytes"
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

// ReportCPUProfile sends the CPU profiling data to the Slack.
func (s *SlackReporter) ReportCPUProfile(
	ctx context.Context, r io.Reader, ci CPUInfo,
) error {
	hostname, _ := os.Hostname() // Don't care about this error.
	var (
		now      = time.Now().Format(reportTimeLayout)
		filename = fmt.Sprintf(CPUProfileFilenameFmt, s.app, hostname, now)
		comment  = fmt.Sprintf(cpuCommentFmt, ci.UsagePercentage, ci.ThresholdPercentage)
	)
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("autopprof: 프로파일 파일 읽기 실패: %w", err)
	}
	if len(data) == 0 {
		return fmt.Errorf("autopprof: 프로파일 파일 크기가 0입니다")
	}
	if _, err := s.client.UploadFileV2Context(ctx, slack.UploadFileV2Parameters{
		Reader:         bytes.NewReader(data),
		Filename:       filename,
		Title:          filename,
		InitialComment: comment,
		Channel:        s.channel,
	}); err != nil {
		return fmt.Errorf("autopprof: failed to upload a file to Slack channel: %w", err)
	}
	return nil
}

// ReportHeapProfile sends the heap profiling data to the Slack.
func (s *SlackReporter) ReportHeapProfile(
	ctx context.Context, r io.Reader, mi MemInfo,
) error {
	hostname, _ := os.Hostname() // Don't care about this error.
	var (
		now      = time.Now().Format(reportTimeLayout)
		filename = fmt.Sprintf(HeapProfileFilenameFmt, s.app, hostname, now)
		comment  = fmt.Sprintf(memCommentFmt, mi.UsagePercentage, mi.ThresholdPercentage)
	)
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("autopprof: 프로파일 파일 읽기 실패: %w", err)
	}
	if len(data) == 0 {
		return fmt.Errorf("autopprof: 프로파일 파일 크기가 0입니다")
	}
	if _, err := s.client.UploadFileV2Context(ctx, slack.UploadFileV2Parameters{
		Reader:         bytes.NewReader(data),
		Filename:       filename,
		Title:          filename,
		InitialComment: comment,
		Channel:        s.channel,
	}); err != nil {
		return fmt.Errorf("autopprof: failed to upload a file to Slack channel: %w", err)
	}
	return nil
}

// ReportGoroutineProfile sends the goroutine profiling data to the Slack.
func (s *SlackReporter) ReportGoroutineProfile(
	ctx context.Context, r io.Reader, gi GoroutineInfo,
) error {
	hostname, _ := os.Hostname() // Don't care about this error.
	var (
		now      = time.Now().Format(reportTimeLayout)
		filename = fmt.Sprintf(GoroutineProfileFilenameFmt, s.app, hostname, now)
		comment  = fmt.Sprintf(goroutineCommentFmt, gi.Count, gi.ThresholdCount)
	)
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("autopprof: 프로파일 파일 읽기 실패: %w", err)
	}
	if len(data) == 0 {
		return fmt.Errorf("autopprof: 프로파일 파일 크기가 0입니다")
	}
	if _, err := s.client.UploadFileV2Context(ctx, slack.UploadFileV2Parameters{
		Reader:         bytes.NewReader(data),
		Filename:       filename,
		Title:          filename,
		InitialComment: comment,
		Channel:        s.channel,
	}); err != nil {
		return fmt.Errorf("autopprof: failed to upload a file to Slack channel: %w", err)
	}
	return nil
}
