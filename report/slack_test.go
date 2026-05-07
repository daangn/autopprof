package report

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/slack-go/slack"
)

// newMockSlackReporter wires a SlackReporter to a fake Slack HTTP
// server. The handler routes Slack method paths (files.info, etc.)
// to user-provided handlers.
func newMockSlackReporter(t *testing.T, channelID string, handlers map[string]http.HandlerFunc) (*SlackReporter, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()
	for path, fn := range handlers {
		mux.HandleFunc("/"+path, fn)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &SlackReporter{
		channelID: channelID,
		client:    slack.New("test-token", slack.OptionAPIURL(srv.URL+"/")),
	}, srv
}

func TestSlackReporter_lookupShareTS_public(t *testing.T) {
	body := `{
		"ok": true,
		"file": {
			"id": "F1",
			"shares": {
				"public": {
					"C42": [{"ts": "1700000000.000100", "channel_name": "alerts"}]
				}
			}
		}
	}`
	s, _ := newMockSlackReporter(t, "C42", map[string]http.HandlerFunc{
		"files.info": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		},
	})
	ts, err := s.lookupShareTS(context.Background(), "F1")
	if err != nil {
		t.Fatalf("lookupShareTS: %v", err)
	}
	if ts != "1700000000.000100" {
		t.Errorf("ts = %q, want %q", ts, "1700000000.000100")
	}
}

func TestSlackReporter_lookupShareTS_private(t *testing.T) {
	body := `{
		"ok": true,
		"file": {
			"id": "F1",
			"shares": {
				"private": {
					"C42": [{"ts": "1700000000.000200"}]
				}
			}
		}
	}`
	s, _ := newMockSlackReporter(t, "C42", map[string]http.HandlerFunc{
		"files.info": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		},
	})
	ts, err := s.lookupShareTS(context.Background(), "F1")
	if err != nil {
		t.Fatalf("lookupShareTS: %v", err)
	}
	if ts != "1700000000.000200" {
		t.Errorf("ts = %q, want %q", ts, "1700000000.000200")
	}
}

func TestSlackReporter_lookupShareTS_missing(t *testing.T) {
	body := `{
		"ok": true,
		"file": {
			"id": "F1",
			"shares": {}
		}
	}`
	s, _ := newMockSlackReporter(t, "C42", map[string]http.HandlerFunc{
		"files.info": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		},
	})
	_, err := s.lookupShareTS(context.Background(), "F1")
	if err == nil {
		t.Fatal("expected error when share record is absent")
	}
	if !strings.Contains(err.Error(), "C42") {
		t.Errorf("err %v should mention channel ID", err)
	}
}

