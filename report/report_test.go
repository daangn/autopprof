package report

import (
	"errors"
	"reflect"
	"testing"

	"github.com/slack-go/slack"
)

func TestNewReporter(t *testing.T) {
	testCases := []struct {
		name  string
		input struct {
			app string
			opt ReporterOption
		}
		want    Reporter
		wantErr error
	}{
		{
			name: "no app name",
			input: struct {
				app string
				opt ReporterOption
			}{
				app: "",
			},
			want:    nil,
			wantErr: errNoAppName,
		},
		{
			name: "unknown reporter type",
			input: struct {
				app string
				opt ReporterOption
			}{
				app: "app",
				opt: ReporterOption{
					Type: "UNKNOWN",
				},
			},
			want:    nil,
			wantErr: errUnknownReporterType,
		},
		{
			name: "no slack reporter option",
			input: struct {
				app string
				opt ReporterOption
			}{
				app: "app",
				opt: ReporterOption{
					Type: SLACK,
				},
			},
			want:    nil,
			wantErr: errNoSlackReporterOption,
		},
		{
			name: "valid slack reporter",
			input: struct {
				app string
				opt ReporterOption
			}{
				app: "app",
				opt: ReporterOption{
					Type: SLACK,
					SlackReporterOption: &SlackReporterOption{
						Token:   "token",
						Channel: "#report-channel",
					},
				},
			},
			want: &slackReporter{
				app:     "app",
				channel: "#report-channel",
				client:  slack.New("token"),
			},
			wantErr: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewReporter(tc.input.app, tc.input.opt)
			if err != nil {
				if tc.wantErr == nil {
					t.Errorf("NewReporter() error = %v, wantErr %v", err, tc.wantErr)
					return
				}
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("NewReporter() error = %v, wantErr %v", err, tc.wantErr)
					return
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("NewReporter() = %v, want %v", got, tc.want)
			}
		})
	}
}
