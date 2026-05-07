package report

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"

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

// Report sends a single profiling report to Slack. The filename and
// comment are provided by autopprof (either supplied by the Metric's
// Collect or filled with defaults).
func (s *SlackReporter) Report(
	ctx context.Context, r io.Reader, info ReportInfo,
) error {
	if _, err := s.uploadFile(ctx, r, info.Filename, info.Comment, ""); err != nil {
		return fmt.Errorf("autopprof: failed to upload a file to Slack channel: %w", err)
	}
	return nil
}

// ReportBatch ships a cascade group as a single Slack thread:
// items[0] (the trigger) becomes the parent message, and items[1:]
// post as thread replies. If the parent's thread timestamp can't be
// resolved we fall back to N un-threaded uploads — each Comment is
// already truthful so the reader still sees the right messages,
// just without thread visual grouping.
func (s *SlackReporter) ReportBatch(
	ctx context.Context, items []ReportItem,
) error {
	if len(items) == 0 {
		return nil
	}
	parent := items[0]
	parentFile, err := s.uploadFile(ctx, parent.Reader, parent.Info.Filename, parent.Info.Comment, "")
	if err != nil {
		return fmt.Errorf("autopprof: failed to upload trigger to Slack channel: %w", err)
	}
	if len(items) == 1 {
		return nil
	}

	threadTS, lookupErr := s.lookupShareTS(ctx, parentFile.ID)
	if lookupErr != nil {
		log.Printf("autopprof: thread ts lookup failed for file %s: %v; cascade items will post unthreaded", parentFile.ID, lookupErr)
	}

	var firstErr error
	for _, it := range items[1:] {
		if _, err := s.uploadFile(ctx, it.Reader, it.Info.Filename, it.Info.Comment, threadTS); err != nil {
			log.Printf("autopprof: cascade upload %q failed: %v", it.Info.MetricName, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// uploadFile pushes one file to the configured Slack channel,
// optionally threading it under threadTS.
func (s *SlackReporter) uploadFile(
	ctx context.Context, r io.Reader, filename, comment, threadTS string,
) (*slack.FileSummary, error) {
	fileSize := 0
	reader := r
	if seeker, ok := r.(io.Seeker); ok {
		size, err := seeker.Seek(0, io.SeekEnd)
		if err != nil {
			return nil, fmt.Errorf("failed to determine reader size by seeking: %w", err)
		}
		fileSize = int(size)

		// Reset the stream's cursor to the beginning.
		// If we don't do this, the Slack client will start reading from the end of the stream
		// and upload an empty data.
		_, err = seeker.Seek(0, io.SeekStart)
		if err != nil {
			return nil, fmt.Errorf("failed to seek back to start: %w", err)
		}
	} else {
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("failed to read data: %w", err)
		}
		fileSize = len(data)
		reader = bytes.NewReader(data)
	}

	return s.client.UploadFileV2Context(ctx, slack.UploadFileV2Parameters{
		Reader:          reader,
		Filename:        filename,
		FileSize:        fileSize,
		Title:           filename,
		InitialComment:  comment,
		Channel:         s.channelID,
		ThreadTimestamp: threadTS,
	})
}

// lookupShareTS resolves the channel-post timestamp for an uploaded
// file so cascade companions can thread under it. files.info returns
// the share record under shares.public[channel] (or .private for
// private channels).
func (s *SlackReporter) lookupShareTS(ctx context.Context, fileID string) (string, error) {
	file, _, _, err := s.client.GetFileInfoContext(ctx, fileID, 0, 0)
	if err != nil {
		return "", err
	}
	if shares, ok := file.Shares.Public[s.channelID]; ok && len(shares) > 0 && shares[0].Ts != "" {
		return shares[0].Ts, nil
	}
	if shares, ok := file.Shares.Private[s.channelID]; ok && len(shares) > 0 && shares[0].Ts != "" {
		return shares[0].Ts, nil
	}
	return "", fmt.Errorf("no shares record for channel %s", s.channelID)
}
