package report

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/slack-go/slack"
)

// SlackReporter is the reporter to send the profiling report to the
// specific Slack channel.
type SlackReporter struct {
	channelID string

	client *slack.Client
}

// SlackReporterOption is the option for the Slack reporter.
type SlackReporterOption struct {
	Token     string
	ChannelID string
}

// NewSlackReporter returns the new SlackReporter.
func NewSlackReporter(opt *SlackReporterOption) *SlackReporter {
	return &SlackReporter{
		channelID: opt.ChannelID,
		client:    slack.New(opt.Token),
	}
}

// Report sends the profiling report to Slack. The filename and
// comment are provided by autopprof (either supplied by the Metric's
// Collect or filled with defaults).
func (s *SlackReporter) Report(
	ctx context.Context, r io.Reader, info ReportInfo,
) error {
	if err := s.reportProfile(ctx, r, info.Filename, info.Comment); err != nil {
		return fmt.Errorf("autopprof: failed to upload a file to Slack channel: %w", err)
	}
	return nil
}

func (s *SlackReporter) reportProfile(ctx context.Context, r io.Reader, filename, comment string) error {
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
