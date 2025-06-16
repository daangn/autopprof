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

	channelID string

	client *slack.Client
}

// SlackReporterOption is the option for the Slack reporter.
type SlackReporterOption struct {
	App   string
	Token string
	// Deprecated: Use ChannelID instead. Reporting with a channel name is no longer supported because the latest Slack API for file uploads requires a channel ID instead of a channel name.
	// For more details about the Slack API, refer to: https://api.slack.com/methods/files.completeUploadExternal
	//
	// For details about the file upload process: https://api.slack.com/messaging/files#uploading_files
	Channel   string
	ChannelID string
}

// NewSlackReporter returns the new SlackReporter.
func NewSlackReporter(opt *SlackReporterOption) *SlackReporter {
	return &SlackReporter{
		app:       opt.App,
		channel:   opt.Channel,
		channelID: opt.ChannelID,
		client:    slack.New(opt.Token),
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
	if err := s.reportProfile(ctx, r, filename, comment); err != nil {
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
	if err := s.reportProfile(ctx, r, filename, comment); err != nil {
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
	if err := s.reportProfile(ctx, r, filename, comment); err != nil {
		return fmt.Errorf("autopprof: failed to upload a file to Slack channel: %w", err)
	}
	return nil
}

func (s *SlackReporter) reportProfile(ctx context.Context, r io.Reader, filename, comment string) error {
	if s.channelID != "" {
		fileSize := 0
		reader := r
		if seeker, ok := r.(io.Seeker); ok {
			size, err := seeker.Seek(0, io.SeekEnd)
			if err != nil {
				return fmt.Errorf("failed to determine reader size by seeking: %w", err)
			}
			fileSize = int(size)

			// Reset the stream's cursor to the beginning.
			// If we don't do this, the Slack client will start reading from the end of the stream
			// and upload an empty data.
			_, err = seeker.Seek(0, io.SeekStart)
			if err != nil {
				return fmt.Errorf("failed to seek back to start: %w", err)
			}
		} else {
			data, err := io.ReadAll(r)
			if err != nil {
				return fmt.Errorf("failed to read data: %w", err)
			}
			fileSize = len(data)
			reader = bytes.NewReader(data)
		}

		_, err := s.client.UploadFileV2Context(ctx, slack.UploadFileV2Parameters{
			Reader:         reader,
			Filename:       filename,
			FileSize:       fileSize,
			Title:          filename,
			InitialComment: comment,
			Channel:        s.channelID,
		})
		return err
	}
	_, err := s.client.UploadFileContext(ctx, slack.FileUploadParameters{
		Reader:         r,
		Filename:       filename,
		Title:          filename,
		InitialComment: comment,
		Channels:       []string{s.channel},
	})
	return err
}
