package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

type DiscordReporter struct {
	webhookURL string
}

type DiscordReporterOption struct {
	WebhookURL string
}

func NewDiscordReporter(opt *DiscordReporterOption) *DiscordReporter {
	return &DiscordReporter{
		webhookURL: opt.WebhookURL,
	}
}

// Report sends the profiling report to Discord. The filename and
// commment are provided by autopprof (either supplied by the Metric's
// Collect or filled with defaults).
func (d *DiscordReporter) Report(ctx context.Context, r io.Reader, info ReportInfo) error {
	if err := d.reportProfile(ctx, r, info.Filename, info.Comment); err != nil {
		return fmt.Errorf("autopprof: failed to upload a file to Discord webhook: %w", err)
	}

	return nil
}

func (d *DiscordReporter) reportProfile(ctx context.Context, r io.Reader, filname, comment string) error {
	reader := r
	if seeker, ok := r.(io.Seeker); ok {
		_, err := seeker.Seek(0, io.SeekEnd)
		if err != nil {
			return fmt.Errorf("failed to determine reader size by seeking: %w", err)
		}

		// Reset the stream's cursor to the beginning.
		// If we don't do this, the Discord webhook will start reading from the end of the stream
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
		reader = bytes.NewReader(data)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Discord webhook payload
	payload := struct {
		Content string `json:"content,omitempty"`
	}{
		Content: comment,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Discord payload: %w", err)
	}

	if err := writer.WriteField("payload_json", string(payloadJSON)); err != nil {
		return fmt.Errorf("failed to write Discord payload: %w", err)
	}

	// File attachment
	part, err := writer.CreateFormFile("files[0]", filname)
	if err != nil {
		return fmt.Errorf("failed to create Discord file part: %w", err)
	}

	if _, err := io.Copy(part, reader); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		d.webhookURL,
		&body,
	)
	if err != nil {
		return fmt.Errorf("failed to create Discord webhook request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Discord webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf(
			"Discord webhook returned status %d: %s",
			resp.StatusCode,
			string(respBody),
		)
	}

	return nil
}