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
			name: "disable flags are all true",
			opt: Option{
				DisableCPUProf: true,
				DisableMemProf: true,
			},
			want: ErrDisableAllProfiling,
		},
		{
			name: "invalid CPUThreshold value 1",
			opt: Option{
				CPUThreshold: -0.5,
			},
			want: ErrInvalidCPUThreshold,
		},
		{
			name: "invalid CPUThreshold value 2",
			opt: Option{
				CPUThreshold: 2.5,
			},
			want: ErrInvalidCPUThreshold,
		},
		{
			name: "invalid MemThreshold value 1",
			opt: Option{
				MemThreshold: -0.5,
			},
			want: ErrInvalidMemThreshold,
		},
		{
			name: "invalid MemThreshold value 2",
			opt: Option{
				MemThreshold: 2.5,
			},
			want: ErrInvalidMemThreshold,
		},
		{
			name: "when given reporter is nil",
			opt: Option{
				Reporter: nil,
			},
			want: ErrNilReporter,
		},
		{
			name: "valid option 1",
			opt: Option{
				Reporter: report.NewSlackReporter(
					&report.SlackReporterOption{
						App:     "appname",
						Token:   "token",
						Channel: "channel",
					},
				),
			},
			want: nil,
		},
		{
			name: "valid option 2",
			opt: Option{
				MemThreshold: 0.5,
				Reporter: report.NewSlackReporter(
					&report.SlackReporterOption{
						App:     "appname",
						Token:   "token",
						Channel: "channel",
					},
				),
			},
			want: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				if globalAp != nil {
					globalAp.stop()
					globalAp = nil
				}
			})
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
			t.Cleanup(func() {
				globalAp = nil
			})
			if tc.started {
				_ = Start(Option{
					MemThreshold: 0.5,
					Reporter: report.NewSlackReporter(
						&report.SlackReporterOption{
							App:     "appname",
							Token:   "token",
							Channel: "channel",
						},
					),
				})
			}
			Stop() // Expect no panic.
		})
	}
}

func fib(n int) int64 {
	if n <= 1 {
		return int64(n)
	}
	return fib(n-1) + fib(n-2)
}

func TestAutoPprof_watchCPUUsage(t *testing.T) {
	ctrl := gomock.NewController(t)

	var reported bool

	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().
		ReportCPUProfile(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(
			func(_ context.Context, _ io.Reader, _ report.CPUInfo) error {
				reported = true
				return nil
			},
		)

	qryer, _ := newQueryer()
	ap := &autoPprof{
		queryer:              qryer,
		disableMemProf:       true,
		scanInterval:         1 * time.Second,
		cpuProfilingDuration: 1 * time.Second,
		cpuThreshold:         0.5, // 50%.
		stopC:                make(chan struct{}),
		reporter:             mockReporter,
	}
	_ = qryer.setCPUQuota()

	// To stop the cpu-bound loop after the test.
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })

	// Run cpu-bound loop to make cpu usage over 50%.
	// The cpu quota of test docker container is 1.5.
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				fib(10)
			}
		}
	}()
	go ap.watchCPUUsage()
	t.Cleanup(func() { ap.stop() })

	// Wait for the goroutine to report.
	time.Sleep(5 * time.Second)
	if !reported {
		t.Errorf("cpu usage is not reported")
	}
}

func TestAutoPprof_watchCPUUsage_consecutive(t *testing.T) {
	ctrl := gomock.NewController(t)

	var reportCnt int

	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().
		ReportCPUProfile(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(
			func(_ context.Context, _ io.Reader, _ report.CPUInfo) error {
				reportCnt++
				return nil
			},
		)

	qryer, _ := newQueryer()
	ap := &autoPprof{
		queryer:                     qryer,
		disableMemProf:              true,
		scanInterval:                1 * time.Second,
		cpuProfilingDuration:        1 * time.Second,
		cpuThreshold:                0.5, // 50%.
		minConsecutiveOverThreshold: 3,
		stopC:                       make(chan struct{}),
		reporter:                    mockReporter,
	}
	_ = qryer.setCPUQuota()

	// To stop the cpu-bound loop after the test.
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })

	// Run cpu-bound loop to make cpu usage over 50%.
	// The cpu quota of test docker container is 1.5.
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				fib(10)
			}
		}
	}()

	go ap.watchCPUUsage()
	t.Cleanup(func() { ap.stop() })

	// Wait for the goroutine to report.
	time.Sleep(4 * time.Second)
	if reportCnt != 1 {
		t.Errorf("cpu usage is reported %d times, want 1", reportCnt)
	}

	// Wait for the goroutine to report. But it should not report.
	time.Sleep(1200 * time.Millisecond)
	if reportCnt != 1 {
		t.Errorf("cpu usage is reported %d times, want 1", reportCnt)
	}

	// Wait for the goroutine to report. But it should not report.
	time.Sleep(1200 * time.Millisecond)
	if reportCnt != 1 {
		t.Errorf("cpu usage is reported %d times, want 1", reportCnt)
	}

	// Wait for the goroutine to report. It should report. (3 times)
	time.Sleep(4 * time.Second)
	if reportCnt != 2 {
		t.Errorf("cpu usage is reported %d times, want 2", reportCnt)
	}
}

func TestAutoPprof_watchMemUsage(t *testing.T) {
	ctrl := gomock.NewController(t)

	var reported bool

	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().
		ReportHeapProfile(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(
			func(_ context.Context, _ io.Reader, _ report.MemInfo) error {
				reported = true
				return nil
			},
		)

	qryer, _ := newQueryer()
	ap := &autoPprof{
		queryer:        qryer,
		disableCPUProf: true,
		scanInterval:   1 * time.Second,
		memThreshold:   0.2, // 20%.
		stopC:          make(chan struct{}),
		reporter:       mockReporter,
	}

	// Occupy heap memory to make memory usage over 20%.
	// The memory limit of test docker container is 1GB.
	m := make(map[int64]string, 10000000)
	for i := 0; i < 10000000; i++ {
		m[int64(i)] = "eating heap memory"
	}

	go ap.watchMemUsage()
	t.Cleanup(func() { ap.stop() })

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
		ReportHeapProfile(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(
			func(_ context.Context, _ io.Reader, _ report.MemInfo) error {
				reportCnt++
				return nil
			},
		)

	qryer, _ := newQueryer()
	ap := &autoPprof{
		queryer:                     qryer,
		disableCPUProf:              true,
		scanInterval:                1 * time.Second,
		memThreshold:                0.2, // 20%.
		minConsecutiveOverThreshold: 3,
		stopC:                       make(chan struct{}),
		reporter:                    mockReporter,
	}

	// Occupy heap memory to make memory usage over 20%.
	// The memory limit of test docker container is 1GB.
	m := make(map[int64]string, 10000000)
	for i := 0; i < 10000000; i++ {
		m[int64(i)] = "eating heap memory"
	}

	go ap.watchMemUsage()
	t.Cleanup(func() { ap.stop() })

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
