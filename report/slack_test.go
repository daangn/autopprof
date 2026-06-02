package report

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slack-go/slack"
)

// fakeClock is a manually-advanced clock for deterministic TTL tests.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type postCall struct {
	channel string
	text    string
}

// fakeSlackClient records calls and returns scripted timestamps/errors
// in place of the real *slack.Client.
type fakeSlackClient struct {
	mu sync.Mutex

	postCalls   []postCall
	uploadCalls []slack.UploadFileV2Parameters

	// postTS supplies the ts returned for each successful PostMessage in
	// order; once exhausted a "ts-N" fallback is used.
	postTS  []string
	postErr error

	uploadErr error
}

func (f *fakeSlackClient) PostMessageContext(
	ctx context.Context, channelID string, options ...slack.MsgOption,
) (string, string, error) {
	text := ""
	if _, values, err := slack.UnsafeApplyMsgOptions(
		"token", channelID, "https://slack.com/api/", options...,
	); err == nil {
		text = values.Get("text")
	}

	f.mu.Lock()
	f.postCalls = append(f.postCalls, postCall{channel: channelID, text: text})
	n := len(f.postCalls)
	postErr := f.postErr
	ts := ""
	if n-1 < len(f.postTS) {
		ts = f.postTS[n-1]
	}
	f.mu.Unlock()

	if postErr != nil {
		return "", "", postErr
	}
	if ts == "" {
		ts = "ts-" + strconv.Itoa(n)
	}
	return channelID, ts, nil
}

func (f *fakeSlackClient) UploadFileV2Context(
	ctx context.Context, params slack.UploadFileV2Parameters,
) (*slack.FileSummary, error) {
	f.mu.Lock()
	f.uploadCalls = append(f.uploadCalls, params)
	uploadErr := f.uploadErr
	f.mu.Unlock()

	if uploadErr != nil {
		return nil, uploadErr
	}
	return &slack.FileSummary{ID: "F1"}, nil
}

func (f *fakeSlackClient) snapshot() (posts []postCall, uploads []slack.UploadFileV2Parameters) {
	f.mu.Lock()
	defer f.mu.Unlock()
	posts = append(posts, f.postCalls...)
	uploads = append(uploads, f.uploadCalls...)
	return posts, uploads
}

// newTestReporter wires a SlackReporter to the fake client and clock.
func newTestReporter(ttl time.Duration, clk *fakeClock, f *fakeSlackClient) *SlackReporter {
	return &SlackReporter{
		channelID: "C123",
		client:    f,
		threadTTL: ttl,
		hostname:  "pod-abc",
		clock:     clk,
	}
}

func doReport(t *testing.T, r *SlackReporter) error {
	t.Helper()
	return r.Report(context.Background(), bytes.NewReader([]byte("profile-bytes")), ReportInfo{
		MetricName: "cpu",
		Filename:   "cpu.bin",
		Comment:    ":rotating_light:[cpu]",
	})
}

func TestSlackReporter_FirstReportOpensThread(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	f := &fakeSlackClient{postTS: []string{"100.1"}}
	r := newTestReporter(time.Minute, clk, f)

	if err := doReport(t, r); err != nil {
		t.Fatalf("Report: %v", err)
	}

	posts, uploads := f.snapshot()
	if len(posts) != 1 {
		t.Fatalf("want 1 header post, got %d", len(posts))
	}
	if !strings.Contains(posts[0].text, "pod-abc") ||
		!strings.Contains(posts[0].text, ":rotating_light:") {
		t.Errorf("header missing host/emoji: %q", posts[0].text)
	}
	if len(uploads) != 1 {
		t.Fatalf("want 1 upload, got %d", len(uploads))
	}
	if uploads[0].ThreadTimestamp != "100.1" {
		t.Errorf("upload thread_ts = %q, want 100.1", uploads[0].ThreadTimestamp)
	}
	if uploads[0].Channel != "C123" {
		t.Errorf("upload channel = %q, want C123", uploads[0].Channel)
	}
}

func TestSlackReporter_ReuseWithinTTL(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	f := &fakeSlackClient{postTS: []string{"100.1"}}
	r := newTestReporter(time.Minute, clk, f)

	if err := doReport(t, r); err != nil {
		t.Fatalf("first Report: %v", err)
	}
	clk.advance(30 * time.Second) // still inside the 1m window
	if err := doReport(t, r); err != nil {
		t.Fatalf("second Report: %v", err)
	}

	posts, uploads := f.snapshot()
	if len(posts) != 1 {
		t.Fatalf("want 1 header post (reused thread), got %d", len(posts))
	}
	if len(uploads) != 2 {
		t.Fatalf("want 2 uploads, got %d", len(uploads))
	}
	for i, u := range uploads {
		if u.ThreadTimestamp != "100.1" {
			t.Errorf("upload[%d] thread_ts = %q, want 100.1", i, u.ThreadTimestamp)
		}
	}
}

func TestSlackReporter_RotateAfterTTL(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	f := &fakeSlackClient{postTS: []string{"A", "B"}}
	r := newTestReporter(time.Minute, clk, f)

	if err := doReport(t, r); err != nil {
		t.Fatalf("first Report: %v", err)
	}
	clk.advance(time.Minute) // exactly TTL: now-start >= TTL -> rotate
	if err := doReport(t, r); err != nil {
		t.Fatalf("second Report: %v", err)
	}

	posts, uploads := f.snapshot()
	if len(posts) != 2 {
		t.Fatalf("want 2 header posts (rotated), got %d", len(posts))
	}
	if len(uploads) != 2 {
		t.Fatalf("want 2 uploads, got %d", len(uploads))
	}
	if uploads[0].ThreadTimestamp != "A" {
		t.Errorf("upload[0] thread_ts = %q, want A", uploads[0].ThreadTimestamp)
	}
	if uploads[1].ThreadTimestamp != "B" {
		t.Errorf("upload[1] thread_ts = %q, want B", uploads[1].ThreadTimestamp)
	}
}

func TestSlackReporter_ThreadingDisabled(t *testing.T) {
	for _, ttl := range []time.Duration{0, -time.Second} {
		clk := &fakeClock{t: time.Unix(1000, 0)}
		f := &fakeSlackClient{}
		r := newTestReporter(ttl, clk, f)

		if err := doReport(t, r); err != nil {
			t.Fatalf("ttl=%v first Report: %v", ttl, err)
		}
		if err := doReport(t, r); err != nil {
			t.Fatalf("ttl=%v second Report: %v", ttl, err)
		}

		posts, uploads := f.snapshot()
		if len(posts) != 0 {
			t.Errorf("ttl=%v: want 0 header posts, got %d", ttl, len(posts))
		}
		if len(uploads) != 2 {
			t.Fatalf("ttl=%v: want 2 uploads, got %d", ttl, len(uploads))
		}
		for i, u := range uploads {
			if u.ThreadTimestamp != "" {
				t.Errorf("ttl=%v upload[%d] thread_ts = %q, want empty", ttl, i, u.ThreadTimestamp)
			}
		}
	}
}

func TestSlackReporter_ConcurrentReportsOpenOneThread(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	f := &fakeSlackClient{postTS: []string{"only"}}
	r := newTestReporter(time.Hour, clk, f)

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if err := doReport(t, r); err != nil {
				t.Errorf("Report: %v", err)
			}
		}()
	}
	wg.Wait()

	posts, uploads := f.snapshot()
	if len(posts) != 1 {
		t.Fatalf("want exactly 1 header post, got %d", len(posts))
	}
	if len(uploads) != n {
		t.Fatalf("want %d uploads, got %d", n, len(uploads))
	}
	for i, u := range uploads {
		if u.ThreadTimestamp != "only" {
			t.Errorf("upload[%d] thread_ts = %q, want only", i, u.ThreadTimestamp)
		}
	}
}

func TestSlackReporter_PostMessageFailureNotPoisoned(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	f := &fakeSlackClient{postErr: errors.New("boom"), postTS: []string{"", "X"}}
	r := newTestReporter(time.Minute, clk, f)

	if err := doReport(t, r); err == nil {
		t.Fatal("want error when PostMessage fails, got nil")
	}
	if _, uploads := f.snapshot(); len(uploads) != 0 {
		t.Fatalf("want 0 uploads after header failure, got %d", len(uploads))
	}
	if r.threadTS != "" {
		t.Fatalf("thread state poisoned after failure: threadTS=%q", r.threadTS)
	}

	// Recovery: a later breach must retry the header and succeed.
	f.mu.Lock()
	f.postErr = nil
	f.mu.Unlock()

	if err := doReport(t, r); err != nil {
		t.Fatalf("recovery Report: %v", err)
	}
	posts, uploads := f.snapshot()
	if len(posts) != 2 { // one failed attempt + one success
		t.Errorf("want 2 header attempts, got %d", len(posts))
	}
	if len(uploads) != 1 {
		t.Fatalf("want 1 upload after recovery, got %d", len(uploads))
	}
	if uploads[0].ThreadTimestamp != "X" {
		t.Errorf("recovered upload thread_ts = %q, want X", uploads[0].ThreadTimestamp)
	}
}

func TestSlackReporter_UploadFailureKeepsThread(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	f := &fakeSlackClient{postTS: []string{"X"}, uploadErr: errors.New("boom")}
	r := newTestReporter(time.Minute, clk, f)

	if err := doReport(t, r); err == nil {
		t.Fatal("want error when upload fails, got nil")
	}
	if r.threadTS != "X" {
		t.Fatalf("threadTS rolled back to %q, want X", r.threadTS)
	}

	// Upload recovers; still inside the window -> reuse thread, no new header.
	f.mu.Lock()
	f.uploadErr = nil
	f.mu.Unlock()
	clk.advance(10 * time.Second)

	if err := doReport(t, r); err != nil {
		t.Fatalf("second Report: %v", err)
	}
	posts, uploads := f.snapshot()
	if len(posts) != 1 {
		t.Errorf("want 1 header post (no new header), got %d", len(posts))
	}
	if len(uploads) != 2 {
		t.Fatalf("want 2 upload attempts, got %d", len(uploads))
	}
	if uploads[1].ThreadTimestamp != "X" {
		t.Errorf("second upload thread_ts = %q, want X", uploads[1].ThreadTimestamp)
	}
}
