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

func TestAutoPprof_loadCPUQuota(t *testing.T) {
	testCases := []struct {
		name                   string
		newAp                  func() *autoPprof
		wantDisableCPUProfFlag bool
		wantErr                error
	}{
		{
			name: "cpu quota is set",
			newAp: func() *autoPprof {
				ctrl := gomock.NewController(t)

				mockQueryer := NewMockqueryer(ctrl)
				mockQueryer.EXPECT().
					setCPUQuota().
					Return(nil) // Means that the quota is set correctly.

				return &autoPprof{
					queryer:        mockQueryer,
					disableCPUProf: false,
					disableMemProf: false,
				}
			},
			wantDisableCPUProfFlag: false,
			wantErr:                nil,
		},
		{
			name: "cpu quota isn't set and memory profiling is enabled",
			newAp: func() *autoPprof {
				ctrl := gomock.NewController(t)

				mockQueryer := NewMockqueryer(ctrl)
				mockQueryer.EXPECT().
					setCPUQuota().
					Return(ErrV2CPUQuotaUndefined)

				return &autoPprof{
					queryer:        mockQueryer,
					disableCPUProf: false,
					disableMemProf: false,
				}
			},
			wantDisableCPUProfFlag: true,
			wantErr:                nil,
		},
		{
			name: "cpu quota isn't set and memory profiling is disabled",
			newAp: func() *autoPprof {
				ctrl := gomock.NewController(t)

				mockQueryer := NewMockqueryer(ctrl)
				mockQueryer.EXPECT().
					setCPUQuota().
					Return(ErrV2CPUQuotaUndefined)

				return &autoPprof{
					queryer:        mockQueryer,
					disableCPUProf: false,
					disableMemProf: true,
				}
			},
			wantDisableCPUProfFlag: false,
			wantErr:                ErrV2CPUQuotaUndefined,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ap := tc.newAp()
			err := ap.loadCPUQuota()
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("loadCPUQuota() = %v, want %v", err, tc.wantErr)
			}
			if ap.disableCPUProf != tc.wantDisableCPUProfFlag {
				t.Errorf("disableCPUProf = %v, want %v", ap.disableCPUProf, tc.wantDisableCPUProfFlag)
			}
		})
	}
}

func TestAutoPprof_watchCPUUsage(t *testing.T) {
	ctrl := gomock.NewController(t)

	var (
		profiled bool
		reported bool
	)

	mockQueryer := NewMockqueryer(ctrl)
	mockQueryer.EXPECT().
		cpuUsage().
		AnyTimes().
		DoAndReturn(
			func() (float64, error) {
				return 0.6, nil
			},
		)

	mockProfiler := NewMockprofiler(ctrl)
	mockProfiler.EXPECT().
		profileCPU().
		AnyTimes().
		DoAndReturn(
			func() ([]byte, error) {
				profiled = true
				return []byte("prof"), nil
			},
		)

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

	ap := &autoPprof{
		disableMemProf: true,
		watchInterval:  1 * time.Second,
		cpuThreshold:   0.5, // 50%.
		queryer:        mockQueryer,
		profiler:       mockProfiler,
		reporter:       mockReporter,
		stopC:          make(chan struct{}),
	}

	go ap.watchCPUUsage()
	t.Cleanup(func() { ap.stop() })

	// Wait for profiling and reporting.
	time.Sleep(1050 * time.Millisecond)
	if !profiled {
		t.Errorf("cpu usage is not profiled")
	}
	if !reported {
		t.Errorf("cpu usage is not reported")
	}
}

func TestAutoPprof_watchCPUUsage_consecutive(t *testing.T) {
	ctrl := gomock.NewController(t)

	var (
		profiledCnt int
		reportedCnt int
	)

	mockQueryer := NewMockqueryer(ctrl)
	mockQueryer.EXPECT().
		cpuUsage().
		AnyTimes().
		DoAndReturn(
			func() (float64, error) {
				return 0.6, nil
			},
		)

	mockProfiler := NewMockprofiler(ctrl)
	mockProfiler.EXPECT().
		profileCPU().
		AnyTimes().
		DoAndReturn(
			func() ([]byte, error) {
				profiledCnt++
				return []byte("prof"), nil
			},
		)

	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().
		ReportCPUProfile(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(
			func(_ context.Context, _ io.Reader, _ report.CPUInfo) error {
				reportedCnt++
				return nil
			},
		)

	ap := &autoPprof{
		disableMemProf:              true,
		watchInterval:               1 * time.Second,
		cpuThreshold:                0.5, // 50%.
		minConsecutiveOverThreshold: 3,
		queryer:                     mockQueryer,
		profiler:                    mockProfiler,
		reporter:                    mockReporter,
		stopC:                       make(chan struct{}),
	}

	go ap.watchCPUUsage()
	t.Cleanup(func() { ap.stop() })

	// Wait for profiling and reporting.
	time.Sleep(1050 * time.Millisecond)
	if profiledCnt != 1 {
		t.Errorf("cpu usage is profiled %d times, want 1", profiledCnt)
	}
	if reportedCnt != 1 {
		t.Errorf("cpu usage is reported %d times, want 1", reportedCnt)
	}

	time.Sleep(1050 * time.Millisecond)
	// 2nd time. It shouldn't be profiled and reported.
	if profiledCnt != 1 {
		t.Errorf("cpu usage is profiled %d times, want 1", profiledCnt)
	}
	if reportedCnt != 1 {
		t.Errorf("cpu usage is reported %d times, want 1", reportedCnt)
	}

	time.Sleep(1050 * time.Millisecond)
	// 3rd time. It shouldn't be profiled and reported.
	if profiledCnt != 1 {
		t.Errorf("cpu usage is profiled %d times, want 1", profiledCnt)
	}
	if reportedCnt != 1 {
		t.Errorf("cpu usage is reported %d times, want 1", reportedCnt)
	}

	time.Sleep(1050 * time.Millisecond)
	// 4th time. Now it should be profiled and reported.
	if profiledCnt != 2 {
		t.Errorf("cpu usage is profiled %d times, want 2", profiledCnt)
	}
	if reportedCnt != 2 {
		t.Errorf("cpu usage is reported %d times, want 2", reportedCnt)
	}
}

func TestAutoPprof_watchCPUUsage_reportBoth(t *testing.T) {
	type fields struct {
		watchInterval  time.Duration
		cpuThreshold   float64
		reportBoth     bool
		disableMemProf bool
		stopC          chan struct{}
	}
	tests := []struct {
		name     string
		fields   fields
		mockFunc func(*Mockqueryer, *Mockprofiler, *report.MockReporter)
	}{
		{
			name: "reportBoth: true",
			fields: fields{
				watchInterval:  1 * time.Second,
				cpuThreshold:   0.5, // 50%.
				reportBoth:     true,
				disableMemProf: false,
				stopC:          make(chan struct{}),
			},
			mockFunc: func(mockQueryer *Mockqueryer, mockProfiler *Mockprofiler, mockReporter *report.MockReporter) {
				gomock.InOrder(
					mockQueryer.EXPECT().
						cpuUsage().
						AnyTimes().
						Return(0.6, nil),

					mockProfiler.EXPECT().
						profileCPU().
						AnyTimes().
						Return([]byte("cpu_prof"), nil),

					mockReporter.EXPECT().
						ReportCPUProfile(gomock.Any(), gomock.Any(), report.CPUInfo{
							ThresholdPercentage: 0.5 * 100,
							UsagePercentage:     0.6 * 100,
						}).
						AnyTimes().
						Return(nil),

					mockQueryer.EXPECT().
						memUsage().
						AnyTimes().
						Return(0.2, nil),

					mockProfiler.EXPECT().
						profileHeap().
						AnyTimes().
						Return([]byte("mem_prof"), nil),

					mockReporter.EXPECT().
						ReportHeapProfile(gomock.Any(), gomock.Any(), report.MemInfo{
							ThresholdPercentage: 0.5 * 100,
							UsagePercentage:     0.2 * 100,
						}).
						AnyTimes().
						Return(nil),
				)
			},
		},
		{
			name: "reportBoth: true, disableMemProf: true",
			fields: fields{
				watchInterval:  1 * time.Second,
				cpuThreshold:   0.5, // 50%.
				reportBoth:     true,
				disableMemProf: true,
				stopC:          make(chan struct{}),
			},
			mockFunc: func(mockQueryer *Mockqueryer, mockProfiler *Mockprofiler, mockReporter *report.MockReporter) {
				gomock.InOrder(
					mockQueryer.EXPECT().
						cpuUsage().
						AnyTimes().
						Return(0.6, nil),

					mockProfiler.EXPECT().
						profileCPU().
						AnyTimes().
						Return([]byte("cpu_prof"), nil),

					mockReporter.EXPECT().
						ReportCPUProfile(gomock.Any(), gomock.Any(), report.CPUInfo{
							ThresholdPercentage: 0.5 * 100,
							UsagePercentage:     0.6 * 100,
						}).
						AnyTimes().
						Return(nil),
				)
			},
		},
		{
			name: "reportBoth: false",
			fields: fields{
				watchInterval:  1 * time.Second,
				cpuThreshold:   0.5, // 50%.
				reportBoth:     false,
				disableMemProf: false,
				stopC:          make(chan struct{}),
			},
			mockFunc: func(mockQueryer *Mockqueryer, mockProfiler *Mockprofiler, mockReporter *report.MockReporter) {
				gomock.InOrder(
					mockQueryer.EXPECT().
						cpuUsage().
						AnyTimes().
						Return(0.6, nil),

					mockProfiler.EXPECT().
						profileCPU().
						AnyTimes().
						Return([]byte("cpu_prof"), nil),

					mockReporter.EXPECT().
						ReportCPUProfile(gomock.Any(), gomock.Any(), report.CPUInfo{
							ThresholdPercentage: 0.5 * 100,
							UsagePercentage:     0.6 * 100,
						}).
						AnyTimes().
						Return(nil),
				)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockQueryer := NewMockqueryer(ctrl)
			mockProfiler := NewMockprofiler(ctrl)
			mockReporter := report.NewMockReporter(ctrl)

			ap := &autoPprof{
				watchInterval:  tt.fields.watchInterval,
				cpuThreshold:   tt.fields.cpuThreshold,
				memThreshold:   0.5, // 50%.
				queryer:        mockQueryer,
				profiler:       mockProfiler,
				reporter:       mockReporter,
				reportBoth:     tt.fields.reportBoth,
				disableMemProf: tt.fields.disableMemProf,
				stopC:          tt.fields.stopC,
			}

			tt.mockFunc(mockQueryer, mockProfiler, mockReporter)

			go ap.watchCPUUsage()
			defer ap.stop()

			// Wait for profiling and reporting.
			time.Sleep(1050 * time.Millisecond)
		})
	}
}

func TestAutoPprof_watchMemUsage(t *testing.T) {
	ctrl := gomock.NewController(t)

	var (
		profiled bool
		reported bool
	)

	mockQueryer := NewMockqueryer(ctrl)
	mockQueryer.EXPECT().
		memUsage().
		AnyTimes().
		DoAndReturn(
			func() (float64, error) {
				return 0.3, nil
			},
		)

	mockProfiler := NewMockprofiler(ctrl)
	mockProfiler.EXPECT().
		profileHeap().
		DoAndReturn(
			func() ([]byte, error) {
				profiled = true
				return []byte("prof"), nil
			},
		)

	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().
		ReportHeapProfile(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(
			func(_ context.Context, _ io.Reader, _ report.MemInfo) error {
				reported = true
				return nil
			},
		)

	ap := &autoPprof{
		disableCPUProf: true,
		watchInterval:  1 * time.Second,
		memThreshold:   0.2, // 20%.
		queryer:        mockQueryer,
		profiler:       mockProfiler,
		reporter:       mockReporter,
		stopC:          make(chan struct{}),
	}

	go ap.watchMemUsage()
	t.Cleanup(func() { ap.stop() })

	// Wait for profiling and reporting.
	time.Sleep(1050 * time.Millisecond)
	if !profiled {
		t.Errorf("mem usage is not profiled")
	}
	if !reported {
		t.Errorf("mem usage is not reported")
	}
}

func TestAutoPprof_watchMemUsage_consecutive(t *testing.T) {
	ctrl := gomock.NewController(t)

	var (
		profiledCnt int
		reportedCnt int
	)

	mockQueryer := NewMockqueryer(ctrl)
	mockQueryer.EXPECT().
		memUsage().
		AnyTimes().
		DoAndReturn(
			func() (float64, error) {
				return 0.3, nil
			},
		)

	mockProfiler := NewMockprofiler(ctrl)
	mockProfiler.EXPECT().
		profileHeap().
		AnyTimes().
		DoAndReturn(
			func() ([]byte, error) {
				profiledCnt++
				return []byte("prof"), nil
			},
		)

	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().
		ReportHeapProfile(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(
			func(_ context.Context, _ io.Reader, _ report.MemInfo) error {
				reportedCnt++
				return nil
			},
		)

	ap := &autoPprof{
		disableCPUProf:              true,
		watchInterval:               1 * time.Second,
		memThreshold:                0.2, // 20%.
		minConsecutiveOverThreshold: 3,
		queryer:                     mockQueryer,
		profiler:                    mockProfiler,
		reporter:                    mockReporter,
		stopC:                       make(chan struct{}),
	}

	go ap.watchMemUsage()
	t.Cleanup(func() { ap.stop() })

	// Wait for profiling and reporting.
	time.Sleep(1050 * time.Millisecond)
	if profiledCnt != 1 {
		t.Errorf("mem usage is profiled %d times, want 1", profiledCnt)
	}
	if reportedCnt != 1 {
		t.Errorf("mem usage is reported %d times, want 1", reportedCnt)
	}

	time.Sleep(1050 * time.Millisecond)
	// 2nd time. It shouldn't be profiled and reported.
	if profiledCnt != 1 {
		t.Errorf("mem usage is profiled %d times, want 1", profiledCnt)
	}
	if reportedCnt != 1 {
		t.Errorf("mem usage is reported %d times, want 1", reportedCnt)
	}

	time.Sleep(1050 * time.Millisecond)
	// 3rd time. It shouldn't be profiled and reported.
	if profiledCnt != 1 {
		t.Errorf("mem usage is profiled %d times, want 1", profiledCnt)
	}
	if reportedCnt != 1 {
		t.Errorf("mem usage is reported %d times, want 1", reportedCnt)
	}

	time.Sleep(1050 * time.Millisecond)
	// 4th time. Now it should be profiled and reported.
	if profiledCnt != 2 {
		t.Errorf("mem usage is profiled %d times, want 2", profiledCnt)
	}
	if reportedCnt != 2 {
		t.Errorf("mem usage is reported %d times, want 2", reportedCnt)
	}
}

func TestAutoPprof_watchMemUsage_reportBoth(t *testing.T) {
	type fields struct {
		watchInterval  time.Duration
		memThreshold   float64
		reportBoth     bool
		disableCPUProf bool
		stopC          chan struct{}
	}
	tests := []struct {
		name     string
		fields   fields
		mockFunc func(*Mockqueryer, *Mockprofiler, *report.MockReporter)
	}{
		{
			name: "reportBoth: true",
			fields: fields{
				watchInterval:  1 * time.Second,
				memThreshold:   0.5, // 50%.
				reportBoth:     true,
				disableCPUProf: false,
				stopC:          make(chan struct{}),
			},
			mockFunc: func(mockQueryer *Mockqueryer, mockProfiler *Mockprofiler, mockReporter *report.MockReporter) {
				gomock.InOrder(
					mockQueryer.EXPECT().
						memUsage().
						AnyTimes().
						Return(0.6, nil),

					mockProfiler.EXPECT().
						profileHeap().
						AnyTimes().
						Return([]byte("cpu_prof"), nil),

					mockReporter.EXPECT().
						ReportHeapProfile(gomock.Any(), gomock.Any(), report.MemInfo{
							ThresholdPercentage: 0.5 * 100,
							UsagePercentage:     0.6 * 100,
						}).
						AnyTimes().
						Return(nil),

					mockQueryer.EXPECT().
						cpuUsage().
						AnyTimes().
						Return(0.2, nil),

					mockProfiler.EXPECT().
						profileCPU().
						AnyTimes().
						Return([]byte("mem_prof"), nil),

					mockReporter.EXPECT().
						ReportCPUProfile(gomock.Any(), gomock.Any(), report.CPUInfo{
							ThresholdPercentage: 0.5 * 100,
							UsagePercentage:     0.2 * 100,
						}).
						AnyTimes().
						Return(nil),
				)
			},
		},
		{
			name: "reportBoth: true, disableCPUProf: true",
			fields: fields{
				watchInterval:  1 * time.Second,
				memThreshold:   0.5, // 50%.
				reportBoth:     true,
				disableCPUProf: true,
				stopC:          make(chan struct{}),
			},
			mockFunc: func(mockQueryer *Mockqueryer, mockProfiler *Mockprofiler, mockReporter *report.MockReporter) {
				gomock.InOrder(
					mockQueryer.EXPECT().
						memUsage().
						AnyTimes().
						Return(0.6, nil),

					mockProfiler.EXPECT().
						profileHeap().
						AnyTimes().
						Return([]byte("cpu_prof"), nil),

					mockReporter.EXPECT().
						ReportHeapProfile(gomock.Any(), gomock.Any(), report.MemInfo{
							ThresholdPercentage: 0.5 * 100,
							UsagePercentage:     0.6 * 100,
						}).
						AnyTimes().
						Return(nil),
				)
			},
		},
		{
			name: "reportBoth: false",
			fields: fields{
				watchInterval:  1 * time.Second,
				memThreshold:   0.5, // 50%.
				reportBoth:     false,
				disableCPUProf: false,
				stopC:          make(chan struct{}),
			},
			mockFunc: func(mockQueryer *Mockqueryer, mockProfiler *Mockprofiler, mockReporter *report.MockReporter) {
				gomock.InOrder(
					mockQueryer.EXPECT().
						memUsage().
						AnyTimes().
						Return(0.6, nil),

					mockProfiler.EXPECT().
						profileHeap().
						AnyTimes().
						Return([]byte("cpu_prof"), nil),

					mockReporter.EXPECT().
						ReportHeapProfile(gomock.Any(), gomock.Any(), report.MemInfo{
							ThresholdPercentage: 0.5 * 100,
							UsagePercentage:     0.6 * 100,
						}).
						AnyTimes().
						Return(nil),
				)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockQueryer := NewMockqueryer(ctrl)
			mockProfiler := NewMockprofiler(ctrl)
			mockReporter := report.NewMockReporter(ctrl)

			ap := &autoPprof{
				watchInterval:  tt.fields.watchInterval,
				cpuThreshold:   0.5, // 50%.
				memThreshold:   tt.fields.memThreshold,
				queryer:        mockQueryer,
				profiler:       mockProfiler,
				reporter:       mockReporter,
				reportBoth:     tt.fields.reportBoth,
				disableCPUProf: tt.fields.disableCPUProf,
				stopC:          tt.fields.stopC,
			}

			tt.mockFunc(mockQueryer, mockProfiler, mockReporter)

			go ap.watchMemUsage()
			defer ap.stop()

			// Wait for profiling and reporting.
			time.Sleep(1050 * time.Millisecond)
		})
	}
}

func fib(n int) int64 {
	if n <= 1 {
		return int64(n)
	}
	return fib(n-1) + fib(n-2)
}

func BenchmarkLightJob(b *testing.B) {
	for i := 0; i < b.N; i++ {
		fib(10)
	}
}

func BenchmarkLightJobWithWatchCPUUsage(b *testing.B) {
	var (
		qryer, _ = newQueryer()
		ticker   = time.NewTicker(defaultWatchInterval)
	)
	for i := 0; i < b.N; i++ {
		select {
		case <-ticker.C:
			_, _ = qryer.cpuUsage()
		default:
			fib(10)
		}
	}
}

func BenchmarkLightJobWithWatchMemUsage(b *testing.B) {
	var (
		qryer, _ = newQueryer()
		ticker   = time.NewTicker(defaultWatchInterval)
	)
	for i := 0; i < b.N; i++ {
		select {
		case <-ticker.C:
			_, _ = qryer.memUsage()
		default:
			fib(10)
		}
	}
}

func BenchmarkHeavyJob(b *testing.B) {
	for i := 0; i < b.N; i++ {
		fib(24)
	}
}

func BenchmarkHeavyJobWithWatchCPUUsage(b *testing.B) {
	var (
		qryer, _ = newQueryer()
		ticker   = time.NewTicker(defaultWatchInterval)
	)
	for i := 0; i < b.N; i++ {
		select {
		case <-ticker.C:
			_, _ = qryer.cpuUsage()
		default:
			fib(24)
		}
	}
}

func BenchmarkHeavyJobWithWatchMemUsage(b *testing.B) {
	var (
		qryer, _ = newQueryer()
		ticker   = time.NewTicker(defaultWatchInterval)
	)
	for i := 0; i < b.N; i++ {
		select {
		case <-ticker.C:
			_, _ = qryer.memUsage()
		default:
			fib(24)
		}
	}
}
