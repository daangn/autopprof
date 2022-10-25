//go:build linux
// +build linux

package autopprof

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/golang/mock/gomock"

	"github.com/daangn/autopprof/report"
)

func TestStart(t *testing.T) {
	testCases := []struct {
		name string
		opt  Option
		want error
	}{
		{
			name: "invalid MemThreshold value 1",
			opt: Option{
				App:          "app",
				MemThreshold: -0.5,
			},
			want: ErrInvalidMemThreshold,
		},
		{
			name: "invalid MemThreshold value 2",
			opt: Option{
				App:          "app",
				MemThreshold: 2.5,
			},
			want: ErrInvalidMemThreshold,
		},
		{
			name: "valid option 1",
			opt: Option{
				App: "app",
				Reporter: report.ReporterOption{
					Type: report.SLACK,
					SlackReporterOption: &report.SlackReporterOption{
						Token:   "token",
						Channel: "channel",
					},
				},
			},
			want: nil,
		},
		{
			name: "valid option 2",
			opt: Option{
				App:          "app",
				MemThreshold: 0.5,
				Reporter: report.ReporterOption{
					Type: report.SLACK,
					SlackReporterOption: &report.SlackReporterOption{
						Token:   "token",
						Channel: "channel",
					},
				},
			},
			want: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Start(tc.opt); !errors.Is(err, tc.want) {
				t.Errorf("Start() = %v, want %v", err, tc.want)
			}
			if tc.want == nil && globalAp == nil {
				t.Errorf("globalAp is nil, want non-nil value")
			}
		})
	}
}

func TestStop(t *testing.T) {
	testCases := []struct {
		name    string
		started bool
	}{
		{
			name:    "stop before start",
			started: false,
		},
		{
			name:    "stop after start",
			started: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.started {
				_ = Start(Option{
					App:          "app",
					MemThreshold: 0.5,
					Reporter: report.ReporterOption{
						Type: report.SLACK,
						SlackReporterOption: &report.SlackReporterOption{
							Token:   "token",
							Channel: "channel",
						},
					},
				})
			}
			Stop() // Expect no panic.
		})
	}
}

func TestAutoPprof_watchMemUsage(t *testing.T) {
	ctrl := gomock.NewController(t)

	var reported bool

	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().
		ReportMem(gomock.Any(), gomock.Any()).
		DoAndReturn(
			func(_ context.Context, _ io.Reader) error {
				reported = true
				return nil
			},
		)

	ap := &autoPprof{
		memThreshold: 0.2, // 20%.
		scanInterval: 1 * time.Second,
		stopC:        make(chan struct{}),
		reporter:     mockReporter,
	}

	// The memory limit of test docker container is 1GB.
	m := make(map[int64]string, 10000000)
	for i := 0; i < 10000000; i++ {
		m[int64(i)] = "eating heap memory"
	}

	ticker := time.NewTicker(1 * time.Second)
	go ap.watchMemUsage(ticker)
	defer ap.stop()

	// Wait for the goroutine to report.
	time.Sleep(1200 * time.Millisecond)
	if !reported {
		t.Errorf("mem usage is not reported")
	}
}

func TestAutoPprof_watchMemUsage_consecutive(t *testing.T) {
	ctrl := gomock.NewController(t)

	var reportCnt int

	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().
		ReportMem(gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(
			func(_ context.Context, _ io.Reader) error {
				reportCnt++
				return nil
			},
		)

	ap := &autoPprof{
		memThreshold:                0.2, // 20%.
		scanInterval:                1 * time.Second,
		minConsecutiveOverThreshold: 3,
		stopC:                       make(chan struct{}),
		reporter:                    mockReporter,
	}

	// The memory limit of test docker container is 1GB.
	m := make(map[int64]string, 10000000)
	for i := 0; i < 10000000; i++ {
		m[int64(i)] = "eating heap memory"
	}

	ticker := time.NewTicker(1 * time.Second)
	go ap.watchMemUsage(ticker)
	defer ap.stop()

	// Wait for the goroutine to report.
	time.Sleep(1200 * time.Millisecond)
	if reportCnt != 1 {
		t.Errorf("mem usage is reported %d times, want 1", reportCnt)
	}

	// Wait for the goroutine to report. But it should not report.
	time.Sleep(1200 * time.Millisecond)
	if reportCnt != 1 {
		t.Errorf("mem usage is reported %d times, want 1", reportCnt)
	}

	// Wait for the goroutine to report. But it should not report.
	time.Sleep(1200 * time.Millisecond)
	if reportCnt != 1 {
		t.Errorf("mem usage is reported %d times, want 1", reportCnt)
	}

	// Wait for the goroutine to report. It should report. (3 times)
	time.Sleep(1200 * time.Millisecond)
	if reportCnt != 2 {
		t.Errorf("mem usage is reported %d times, want 2", reportCnt)
	}
}
