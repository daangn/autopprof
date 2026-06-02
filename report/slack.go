package report

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/slack-go/slack"
)

// slackClient is the subset of *slack.Client that SlackReporter needs.
// Declaring it as an interface lets tests inject a fake to exercise the
// thread-grouping logic without hitting the Slack API. *slack.Client
// satisfies this interface.
type slackClient interface {
	PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error)
	UploadFileV2Context(ctx context.Context, params slack.UploadFileV2Parameters) (*slack.FileSummary, error)
}

// clock abstracts time.Now so tests can drive the TTL window
// deterministically.
type clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// SlackReporter is the reporter to send the profiling report to the
// specific Slack channel.
type SlackReporter struct {
	channelID string

	client slackClient

	// threadTTL groups this process's reports into a single Slack thread
	// for the given window. Zero or negative disables threading: every
	// report is uploaded as its own top-level message (legacy behavior).
	threadTTL time.Duration

	// hostname identifies this pod in the thread header. Resolved once at
	// construction.
	hostname string

	// clock is realClock in production; tests inject a fake to drive the
	// TTL window deterministically.
	clock clock

	// mu guards the thread state below. ensureThread holds it across the
	// (fast) PostMessage call so concurrent first-reports can't each open
	// a separate thread.
	mu              sync.Mutex
	threadTS        string
	threadStartedAt time.Time
}

// SlackReporterOption is the option for the Slack reporter.
type SlackReporterOption struct {
	Token     string
	ChannelID string

	// ThreadTTL groups all of this process's reports into a single Slack
	// thread for the duration of the window, measured from when the
	// thread was opened. Once the window elapses, the next report opens a
	// fresh thread. Zero or negative (the default) disables threading:
	// each report is posted as a separate top-level message, preserving
	// the previous behavior.
	//
	// Enabling threading posts one extra header message per window to
	// obtain a parent timestamp, because Slack's file upload API returns
	// no timestamp to thread replies on.
	ThreadTTL time.Duration
}

// NewSlackReporter returns the new SlackReporter.
func NewSlackReporter(opt *SlackReporterOption) *SlackReporter {
	return &SlackReporter{
		channelID: opt.ChannelID,
		client:    slack.New(opt.Token),
		threadTTL: opt.ThreadTTL,
		hostname:  hostname(),
		clock:     realClock{},
	}
}

// Report sends the profiling report to Slack. The filename and
// comment are provided by autopprof (either supplied by the Metric's
// Collect or filled with defaults). When ThreadTTL is set, reports
// within the window are uploaded as replies under a shared thread
// header instead of as separate top-level messages.
func (s *SlackReporter) Report(
	ctx context.Context, r io.Reader, info ReportInfo,
) error {
	var threadTS string
	if s.threadTTL > 0 {
		ts, err := s.ensureThread(ctx)
		if err != nil {
			return fmt.Errorf("autopprof: failed to open a Slack thread: %w", err)
		}
		threadTS = ts
	}
	if err := s.reportProfile(ctx, r, info.Filename, info.Comment, threadTS); err != nil {
		return fmt.Errorf("autopprof: failed to upload a file to Slack channel: %w", err)
	}
	return nil
}

// ensureThread returns the timestamp that reports should reply to,
// opening (or rotating) the thread when none is active or the TTL
// window has elapsed. It holds the lock across PostMessageContext so
// concurrent callers open at most one thread per window.
func (s *SlackReporter) ensureThread(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clock.Now()
	if s.threadTS != "" && now.Sub(s.threadStartedAt) < s.threadTTL {
		return s.threadTS, nil
	}

	// Either no thread has been opened yet, or the window has elapsed
	// (now - start >= TTL): open a new one. On error we leave the
	// existing state untouched so the next breach simply retries — we do
	// not fall back to a top-level upload (that would silently reintroduce
	// the noise threading is meant to remove).
	_, ts, err := s.client.PostMessageContext(
		ctx, s.channelID, slack.MsgOptionText(s.header(now), false),
	)
	if err != nil {
		return "", err
	}
	s.threadTS = ts
	s.threadStartedAt = now
	return ts, nil
}

// header is the text of the thread's parent message. It carries the
// host so threads from different pods stay distinguishable, plus the
// open time so successive windows from the same pod don't look
// identical.
func (s *SlackReporter) header(now time.Time) string {
	return fmt.Sprintf(
		":rotating_light:[autopprof] profiling reports — host %s (since %s)",
		s.hostname, now.Format(time.RFC3339),
	)
}

// reportProfile uploads a single profile. threadTS is empty when
// threading is disabled, which yields the original top-level upload.
// A non-empty threadTS posts the file as a reply in that thread.
func (s *SlackReporter) reportProfile(ctx context.Context, r io.Reader, filename, comment, threadTS string) error {
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
		Reader:          reader,
		Filename:        filename,
		FileSize:        fileSize,
		Title:           filename,
		InitialComment:  comment,
		Channel:         s.channelID,
		ThreadTimestamp: threadTS,
	})
	return err
}

// hostname returns this host's name for thread headers, falling back to
// "unknown" when it can't be determined.
func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown"
	}
	return h
}
