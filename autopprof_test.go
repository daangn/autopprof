//go:build linux
// +build linux

package autopprof

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/daangn/autopprof/queryer"
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
				DisableCPUProf:       true,
				DisableMemProf:       true,
				DisableGoroutineProf: true,
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
			name: "invalid GoroutineThreshold value -1",
			opt: Option{
				GoroutineThreshold: -1,
			},
			want: ErrInvalidGoroutineThreshold,
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
						App:       "appname",
						Token:     "token",
						ChannelID: "channel_id",
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
						App:       "appname",
						Token:     "token",
						ChannelID: "channel_id",
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
							App:       "appname",
							Token:     "token",
							ChannelID: "channel_id",
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

				mockQueryer := queryer.NewMockCgroupsQueryer(ctrl)
				mockQueryer.EXPECT().
					SetCPUQuota().
					Return(nil) // Means that the quota is set correctly.

				return &autoPprof{
					cgroupQueryer:  mockQueryer,
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

				mockQueryer := queryer.NewMockCgroupsQueryer(ctrl)
				mockQueryer.EXPECT().
					SetCPUQuota().
					Return(queryer.ErrV2CPUQuotaUndefined)

				return &autoPprof{
					cgroupQueryer:  mockQueryer,
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

				mockQueryer := queryer.NewMockCgroupsQueryer(ctrl)
				mockQueryer.EXPECT().
					SetCPUQuota().
					Return(queryer.ErrV2CPUQuotaUndefined)

				return &autoPprof{
					cgroupQueryer:  mockQueryer,
					disableCPUProf: false,
					disableMemProf: true,
				}
			},
			wantDisableCPUProfFlag: false,
			wantErr:                queryer.ErrV2CPUQuotaUndefined,
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

	mockQueryer := queryer.NewMockCgroupsQueryer(ctrl)
	mockQueryer.EXPECT().
		CPUUsage().
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
		ReportCPUProfile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(
			func(_ context.Context, _ io.Reader, _ int, _ report.CPUInfo) error {
				reported = true
				return nil
			},
		)

	ap := &autoPprof{
		disableMemProf: true,
		watchInterval:  1 * time.Second,
		cpuThreshold:   0.5, // 50%.
		cgroupQueryer:  mockQueryer,
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

	mockQueryer := queryer.NewMockCgroupsQueryer(ctrl)
	mockQueryer.EXPECT().
		CPUUsage().
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
		ReportCPUProfile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(
			func(_ context.Context, _ io.Reader, _ int, _ report.CPUInfo) error {
				reportedCnt++
				return nil
			},
		)

	ap := &autoPprof{
		disableMemProf:              true,
		watchInterval:               1 * time.Second,
		cpuThreshold:                0.5, // 50%.
		minConsecutiveOverThreshold: 3,
		cgroupQueryer:               mockQueryer,
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

func TestAutoPprof_watchCPUUsage_reportAll(t *testing.T) {
	type fields struct {
		watchInterval        time.Duration
		cpuThreshold         float64
		reportAll            bool
		disableMemProf       bool
		disableGoroutineProf bool
		stopC                chan struct{}
	}
	testCases := []struct {
		name     string
		fields   fields
		mockFunc func(*queryer.MockCgroupsQueryer, *queryer.MockRuntimeQueryer, *Mockprofiler, *report.MockReporter)
	}{
		{
			name: "reportAll: true",
			fields: fields{
				watchInterval:        1 * time.Second,
				cpuThreshold:         0.5, // 50%.
				reportAll:            true,
				disableMemProf:       false,
				disableGoroutineProf: false,
				stopC:                make(chan struct{}),
			},
			mockFunc: func(mockCgroupsQueryer *queryer.MockCgroupsQueryer, mockRuntimeQueryer *queryer.MockRuntimeQueryer, mockProfiler *Mockprofiler, mockReporter *report.MockReporter) {
				gomock.InOrder(
					mockCgroupsQueryer.EXPECT().
						CPUUsage().
						AnyTimes().
						Return(0.6, nil),

					mockProfiler.EXPECT().
						profileCPU().
						AnyTimes().
						Return([]byte("cpu_prof"), nil),

					mockReporter.EXPECT().
						ReportCPUProfile(gomock.Any(), gomock.Any(), gomock.Any(), report.CPUInfo{
							ThresholdPercentage: 0.5 * 100,
							UsagePercentage:     0.6 * 100,
						}).
						AnyTimes().
						Return(nil),

					mockCgroupsQueryer.EXPECT().
						MemUsage().
						AnyTimes().
						Return(0.2, nil),

					mockProfiler.EXPECT().
						profileHeap().
						AnyTimes().
						Return([]byte("mem_prof"), nil),

					mockReporter.EXPECT().
						ReportHeapProfile(gomock.Any(), gomock.Any(), gomock.Any(), report.MemInfo{
							ThresholdPercentage: 0.5 * 100,
							UsagePercentage:     0.2 * 100,
						}).
						AnyTimes().
						Return(nil),

					mockRuntimeQueryer.EXPECT().
						GoroutineCount().
						AnyTimes().
						Return(200),

					mockProfiler.EXPECT().
						profileGoroutine().
						AnyTimes().
						Return([]byte("goroutine_prof"), nil),

					mockReporter.EXPECT().
						ReportGoroutineProfile(gomock.Any(), gomock.Any(), gomock.Any(), report.GoroutineInfo{
							ThresholdCount: 500,
							Count:          200,
						}).
						AnyTimes().
						Return(nil),
				)
			},
		},
		{
			name: "reportAll: true, disableMemProf: true",
			fields: fields{
				watchInterval:        1 * time.Second,
				cpuThreshold:         0.5, // 50%.
				reportAll:            true,
				disableMemProf:       true,
				disableGoroutineProf: false,
				stopC:                make(chan struct{}),
			},
			mockFunc: func(mockCgroupsQueryer *queryer.MockCgroupsQueryer, mockRuntimeQueryer *queryer.MockRuntimeQueryer, mockProfiler *Mockprofiler, mockReporter *report.MockReporter) {
				gomock.InOrder(
					mockCgroupsQueryer.EXPECT().
						CPUUsage().
						AnyTimes().
						Return(0.6, nil),

					mockProfiler.EXPECT().
						profileCPU().
						AnyTimes().
						Return([]byte("cpu_prof"), nil),

					mockReporter.EXPECT().
						ReportCPUProfile(gomock.Any(), gomock.Any(), gomock.Any(), report.CPUInfo{
							ThresholdPercentage: 0.5 * 100,
							UsagePercentage:     0.6 * 100,
						}).
						AnyTimes().
						Return(nil),

					mockRuntimeQueryer.EXPECT().
						GoroutineCount().
						AnyTimes().
						Return(200),

					mockProfiler.EXPECT().
						profileGoroutine().
						AnyTimes().
						Return([]byte("goroutine_prof"), nil),

					mockReporter.EXPECT().
						ReportGoroutineProfile(gomock.Any(), gomock.Any(), gomock.Any(), report.GoroutineInfo{
							ThresholdCount: 500,
							Count:          200,
						}).
						AnyTimes().
						Return(nil),
				)
			},
		},
		{
			name: "reportAll: false",
			fields: fields{
				watchInterval:  1 * time.Second,
				cpuThreshold:   0.5, // 50%.
				reportAll:      false,
				disableMemProf: false,
				stopC:          make(chan struct{}),
			},
			mockFunc: func(mockCgroupsQueryer *queryer.MockCgroupsQueryer, mockRuntimeQueryer *queryer.MockRuntimeQueryer, mockProfiler *Mockprofiler, mockReporter *report.MockReporter) {
				gomock.InOrder(
					mockCgroupsQueryer.EXPECT().
						CPUUsage().
						AnyTimes().
						Return(0.6, nil),

					mockProfiler.EXPECT().
						profileCPU().
						AnyTimes().
						Return([]byte("cpu_prof"), nil),

					mockReporter.EXPECT().
						ReportCPUProfile(gomock.Any(), gomock.Any(), gomock.Any(), report.CPUInfo{
							ThresholdPercentage: 0.5 * 100,
							UsagePercentage:     0.6 * 100,
						}).
						AnyTimes().
						Return(nil),
				)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockCgroupsQueryer := queryer.NewMockCgroupsQueryer(ctrl)
			mockRuntimeQueryer := queryer.NewMockRuntimeQueryer(ctrl)
			mockProfiler := NewMockprofiler(ctrl)
			mockReporter := report.NewMockReporter(ctrl)

			ap := &autoPprof{
				watchInterval:      tc.fields.watchInterval,
				cpuThreshold:       tc.fields.cpuThreshold,
				memThreshold:       0.5, // 50%.
				goroutineThreshold: 500,
				cgroupQueryer:      mockCgroupsQueryer,
				runtimeQueryer:     mockRuntimeQueryer,
				profiler:           mockProfiler,
				reporter:           mockReporter,
				reportBoth:         tc.fields.reportAll,
				disableMemProf:     tc.fields.disableMemProf,
				stopC:              tc.fields.stopC,
			}

			tc.mockFunc(mockCgroupsQueryer, mockRuntimeQueryer, mockProfiler, mockReporter)

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

	mockQueryer := queryer.NewMockCgroupsQueryer(ctrl)
	mockQueryer.EXPECT().
		MemUsage().
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
		ReportHeapProfile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(
			func(_ context.Context, _ io.Reader, _ int, _ report.MemInfo) error {
				reported = true
				return nil
			},
		)

	ap := &autoPprof{
		disableCPUProf: true,
		watchInterval:  1 * time.Second,
		memThreshold:   0.2, // 20%.
		cgroupQueryer:  mockQueryer,
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

	mockQueryer := queryer.NewMockCgroupsQueryer(ctrl)
	mockQueryer.EXPECT().
		MemUsage().
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
		ReportHeapProfile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(
			func(_ context.Context, _ io.Reader, _ int, _ report.MemInfo) error {
				reportedCnt++
				return nil
			},
		)

	ap := &autoPprof{
		disableCPUProf:              true,
		watchInterval:               1 * time.Second,
		memThreshold:                0.2, // 20%.
		minConsecutiveOverThreshold: 3,
		cgroupQueryer:               mockQueryer,
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

func TestAutoPprof_watchMemUsage_reportAll(t *testing.T) {
	type fields struct {
		watchInterval        time.Duration
		memThreshold         float64
		reportAll            bool
		disableCPUProf       bool
		disableGoroutineProf bool
		stopC                chan struct{}
	}
	testCases := []struct {
		name     string
		fields   fields
		mockFunc func(*queryer.MockCgroupsQueryer, *queryer.MockRuntimeQueryer, *Mockprofiler, *report.MockReporter)
	}{
		{
			name: "reportAll: true",
			fields: fields{
				watchInterval:        1 * time.Second,
				memThreshold:         0.5, // 50%.
				reportAll:            true,
				disableCPUProf:       false,
				disableGoroutineProf: false,
				stopC:                make(chan struct{}),
			},
			mockFunc: func(mockCgroupsQueryer *queryer.MockCgroupsQueryer, mockRuntimeQueryer *queryer.MockRuntimeQueryer, mockProfiler *Mockprofiler, mockReporter *report.MockReporter) {
				gomock.InOrder(
					mockCgroupsQueryer.EXPECT().
						MemUsage().
						AnyTimes().
						Return(0.6, nil),

					mockProfiler.EXPECT().
						profileHeap().
						AnyTimes().
						Return([]byte("mem_prof"), nil),

					mockReporter.EXPECT().
						ReportHeapProfile(gomock.Any(), gomock.Any(), gomock.Any(), report.MemInfo{
							ThresholdPercentage: 0.5 * 100,
							UsagePercentage:     0.6 * 100,
						}).
						AnyTimes().
						Return(nil),

					mockCgroupsQueryer.EXPECT().
						CPUUsage().
						AnyTimes().
						Return(0.2, nil),

					mockProfiler.EXPECT().
						profileCPU().
						AnyTimes().
						Return([]byte("cpu_prof"), nil),

					mockReporter.EXPECT().
						ReportCPUProfile(gomock.Any(), gomock.Any(), gomock.Any(), report.CPUInfo{
							ThresholdPercentage: 0.5 * 100,
							UsagePercentage:     0.2 * 100,
						}).
						AnyTimes().
						Return(nil),

					mockRuntimeQueryer.EXPECT().
						GoroutineCount().
						AnyTimes().
						Return(200),

					mockProfiler.EXPECT().
						profileGoroutine().
						AnyTimes().
						Return([]byte("goroutine_prof"), nil),

					mockReporter.EXPECT().
						ReportGoroutineProfile(gomock.Any(), gomock.Any(), gomock.Any(), report.GoroutineInfo{
							ThresholdCount: 500,
							Count:          200,
						}).
						AnyTimes().
						Return(nil),
				)
			},
		},
		{
			name: "reportAll: true, disableCPUProf: true",
			fields: fields{
				watchInterval:        1 * time.Second,
				memThreshold:         0.5, // 50%.
				reportAll:            true,
				disableCPUProf:       true,
				disableGoroutineProf: false,
				stopC:                make(chan struct{}),
			},
			mockFunc: func(mockCgroupsQueryer *queryer.MockCgroupsQueryer, mockRuntimeQueryer *queryer.MockRuntimeQueryer, mockProfiler *Mockprofiler, mockReporter *report.MockReporter) {
				gomock.InOrder(
					mockCgroupsQueryer.EXPECT().
						MemUsage().
						AnyTimes().
						Return(0.6, nil),

					mockProfiler.EXPECT().
						profileHeap().
						AnyTimes().
						Return([]byte("mem_prof"), nil),

					mockReporter.EXPECT().
						ReportHeapProfile(gomock.Any(), gomock.Any(), gomock.Any(), report.MemInfo{
							ThresholdPercentage: 0.5 * 100,
							UsagePercentage:     0.6 * 100,
						}).
						AnyTimes().
						Return(nil),

					mockRuntimeQueryer.EXPECT().
						GoroutineCount().
						AnyTimes().
						Return(200),

					mockProfiler.EXPECT().
						profileGoroutine().
						AnyTimes().
						Return([]byte("goroutine_prof"), nil),

					mockReporter.EXPECT().
						ReportGoroutineProfile(gomock.Any(), gomock.Any(), gomock.Any(), report.GoroutineInfo{
							ThresholdCount: 500,
							Count:          200,
						}).
						AnyTimes().
						Return(nil),
				)
			},
		},
		{
			name: "reportAll: false",
			fields: fields{
				watchInterval:        1 * time.Second,
				memThreshold:         0.5, // 50%.
				reportAll:            false,
				disableCPUProf:       false,
				disableGoroutineProf: false,
				stopC:                make(chan struct{}),
			},
			mockFunc: func(mockCgroupsQueryer *queryer.MockCgroupsQueryer, mockRuntimeQueryer *queryer.MockRuntimeQueryer, mockProfiler *Mockprofiler, mockReporter *report.MockReporter) {
				gomock.InOrder(
					mockCgroupsQueryer.EXPECT().
						MemUsage().
						AnyTimes().
						Return(0.6, nil),

					mockProfiler.EXPECT().
						profileHeap().
						AnyTimes().
						Return([]byte("mem_prof"), nil),

					mockReporter.EXPECT().
						ReportHeapProfile(gomock.Any(), gomock.Any(), gomock.Any(), report.MemInfo{
							ThresholdPercentage: 0.5 * 100,
							UsagePercentage:     0.6 * 100,
						}).
						AnyTimes().
						Return(nil),
				)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockCgroupsQueryer := queryer.NewMockCgroupsQueryer(ctrl)
			mockRuntimeQueryer := queryer.NewMockRuntimeQueryer(ctrl)
			mockProfiler := NewMockprofiler(ctrl)
			mockReporter := report.NewMockReporter(ctrl)

			ap := &autoPprof{
				watchInterval:      tc.fields.watchInterval,
				cpuThreshold:       0.5, // 50%.
				memThreshold:       tc.fields.memThreshold,
				goroutineThreshold: 500,
				cgroupQueryer:      mockCgroupsQueryer,
				runtimeQueryer:     mockRuntimeQueryer,
				profiler:           mockProfiler,
				reporter:           mockReporter,
				reportAll:          tc.fields.reportAll,
				disableCPUProf:     tc.fields.disableCPUProf,
				stopC:              tc.fields.stopC,
			}

			tc.mockFunc(mockCgroupsQueryer, mockRuntimeQueryer, mockProfiler, mockReporter)

			go ap.watchMemUsage()
			defer ap.stop()

			// Wait for profiling and reporting.
			time.Sleep(1050 * time.Millisecond)
		})
	}
}

func TestAutoPprof_watchGoroutineCount(t *testing.T) {
	ctrl := gomock.NewController(t)

	var (
		profiled bool
		reported bool
	)

	mockQueryer := queryer.NewMockRuntimeQueryer(ctrl)
	mockQueryer.EXPECT().
		GoroutineCount().
		AnyTimes().
		DoAndReturn(
			func() (int, error) {
				return 200, nil
			},
		)

	mockProfiler := NewMockprofiler(ctrl)
	mockProfiler.EXPECT().
		profileGoroutine().
		DoAndReturn(
			func() ([]byte, error) {
				profiled = true
				return []byte("prof"), nil
			},
		)

	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().
		ReportGoroutineProfile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(
			func(_ context.Context, _ io.Reader, _ int, _ report.GoroutineInfo) error {
				reported = true
				return nil
			},
		)

	ap := &autoPprof{
		disableCPUProf:     true,
		disableMemProf:     true,
		watchInterval:      1 * time.Second,
		goroutineThreshold: 100,
		runtimeQueryer:     mockQueryer,
		profiler:           mockProfiler,
		reporter:           mockReporter,
		stopC:              make(chan struct{}),
	}

	go ap.watchGoroutineCount()
	t.Cleanup(func() { ap.stop() })

	// Wait for profiling and reporting.
	time.Sleep(1050 * time.Millisecond)
	if !profiled {
		t.Errorf("goroutine count is not profiled")
	}
	if !reported {
		t.Errorf("goroutine count is not reported")
	}
}

func TestAutoPprof_watchGoroutineCount_consecutive(t *testing.T) {
	ctrl := gomock.NewController(t)

	var (
		profiledCnt int
		reportedCnt int
	)

	mockQueryer := queryer.NewMockRuntimeQueryer(ctrl)
	mockQueryer.EXPECT().
		GoroutineCount().
		AnyTimes().
		DoAndReturn(
			func() (int, error) {
				return 200, nil
			},
		)

	mockProfiler := NewMockprofiler(ctrl)
	mockProfiler.EXPECT().
		profileGoroutine().
		AnyTimes().
		DoAndReturn(
			func() ([]byte, error) {
				profiledCnt++
				return []byte("prof"), nil
			},
		)

	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().
		ReportGoroutineProfile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(
			func(_ context.Context, _ io.Reader, _ int, _ report.GoroutineInfo) error {
				reportedCnt++
				return nil
			},
		)

	ap := &autoPprof{
		disableCPUProf:              true,
		disableMemProf:              true,
		watchInterval:               1 * time.Second,
		goroutineThreshold:          100,
		minConsecutiveOverThreshold: 3,
		runtimeQueryer:              mockQueryer,
		profiler:                    mockProfiler,
		reporter:                    mockReporter,
		stopC:                       make(chan struct{}),
	}

	go ap.watchGoroutineCount()
	t.Cleanup(func() { ap.stop() })

	// Wait for profiling and reporting.
	time.Sleep(1050 * time.Millisecond)
	if profiledCnt != 1 {
		t.Errorf("goroutine count is profiled %d times, want 1", profiledCnt)
	}
	if reportedCnt != 1 {
		t.Errorf("goroutine count is reported %d times, want 1", reportedCnt)
	}

	time.Sleep(1050 * time.Millisecond)
	// 2nd time. It shouldn't be profiled and reported.
	if profiledCnt != 1 {
		t.Errorf("goroutine count is profiled %d times, want 1", profiledCnt)
	}
	if reportedCnt != 1 {
		t.Errorf("goroutine count is reported %d times, want 1", reportedCnt)
	}

	time.Sleep(1050 * time.Millisecond)
	// 3rd time. It shouldn't be profiled and reported.
	if profiledCnt != 1 {
		t.Errorf("goroutine count is profiled %d times, want 1", profiledCnt)
	}
	if reportedCnt != 1 {
		t.Errorf("goroutine count is reported %d times, want 1", reportedCnt)
	}

	time.Sleep(1050 * time.Millisecond)
	// 4th time. Now it should be profiled and reported.
	if profiledCnt != 2 {
		t.Errorf("goroutine count is profiled %d times, want 2", profiledCnt)
	}
	if reportedCnt != 2 {
		t.Errorf("goroutine count is reported %d times, want 2", reportedCnt)
	}
}

func TestAutoPprof_watchGoroutineCount_reportAll(t *testing.T) {
	type fields struct {
		watchInterval      time.Duration
		goroutineThreshold int
		reportAll          bool
		disableCPUProf     bool
		disableMemProf     bool
		stopC              chan struct{}
	}
	testCases := []struct {
		name     string
		fields   fields
		mockFunc func(*queryer.MockCgroupsQueryer, *queryer.MockRuntimeQueryer, *Mockprofiler, *report.MockReporter)
	}{
		{
			name: "reportAll: true",
			fields: fields{
				watchInterval:      1 * time.Second,
				goroutineThreshold: 100,
				reportAll:          true,
				disableCPUProf:     false,
				disableMemProf:     false,
				stopC:              make(chan struct{}),
			},
			mockFunc: func(mockCgroupsQueryer *queryer.MockCgroupsQueryer, mockRuntimeQueryer *queryer.MockRuntimeQueryer, mockProfiler *Mockprofiler, mockReporter *report.MockReporter) {
				gomock.InOrder(
					mockRuntimeQueryer.EXPECT().
						GoroutineCount().
						AnyTimes().
						Return(200),

					mockProfiler.EXPECT().
						profileGoroutine().
						AnyTimes().
						Return([]byte("goroutine_prof"), nil),

					mockReporter.EXPECT().
						ReportGoroutineProfile(gomock.Any(), gomock.Any(), gomock.Any(), report.GoroutineInfo{
							ThresholdCount: 100,
							Count:          200,
						}).
						AnyTimes().
						Return(nil),

					mockCgroupsQueryer.EXPECT().
						CPUUsage().
						AnyTimes().
						Return(0.2, nil),

					mockProfiler.EXPECT().
						profileCPU().
						AnyTimes().
						Return([]byte("cpu_prof"), nil),

					mockReporter.EXPECT().
						ReportCPUProfile(gomock.Any(), gomock.Any(), gomock.Any(), report.CPUInfo{
							ThresholdPercentage: 0.5 * 100,
							UsagePercentage:     0.2 * 100,
						}).
						AnyTimes().
						Return(nil),

					mockCgroupsQueryer.EXPECT().
						MemUsage().
						AnyTimes().
						Return(0.2, nil),

					mockProfiler.EXPECT().
						profileHeap().
						AnyTimes().
						Return([]byte("mem_prof"), nil),

					mockReporter.EXPECT().
						ReportHeapProfile(gomock.Any(), gomock.Any(), gomock.Any(), report.MemInfo{
							ThresholdPercentage: 0.5 * 100,
							UsagePercentage:     0.2 * 100,
						}).
						AnyTimes().
						Return(nil),
				)
			},
		},
		{
			name: "reportAll: true, disableCPUProf: true",
			fields: fields{
				watchInterval:      1 * time.Second,
				goroutineThreshold: 100,
				reportAll:          true,
				disableCPUProf:     true,
				disableMemProf:     false,
				stopC:              make(chan struct{}),
			},
			mockFunc: func(mockCgroupsQueryer *queryer.MockCgroupsQueryer, mockRuntimeQueryer *queryer.MockRuntimeQueryer, mockProfiler *Mockprofiler, mockReporter *report.MockReporter) {
				gomock.InOrder(
					mockRuntimeQueryer.EXPECT().
						GoroutineCount().
						AnyTimes().
						Return(200),

					mockProfiler.EXPECT().
						profileGoroutine().
						AnyTimes().
						Return([]byte("goroutine_prof"), nil),

					mockReporter.EXPECT().
						ReportGoroutineProfile(gomock.Any(), gomock.Any(), gomock.Any(), report.GoroutineInfo{
							ThresholdCount: 100,
							Count:          200,
						}).
						AnyTimes().
						Return(nil),

					mockCgroupsQueryer.EXPECT().
						MemUsage().
						AnyTimes().
						Return(0.2, nil),

					mockProfiler.EXPECT().
						profileHeap().
						AnyTimes().
						Return([]byte("mem_prof"), nil),

					mockReporter.EXPECT().
						ReportHeapProfile(gomock.Any(), gomock.Any(), gomock.Any(), report.MemInfo{
							ThresholdPercentage: 0.5 * 100,
							UsagePercentage:     0.2 * 100,
						}).
						AnyTimes().
						Return(nil),
				)
			},
		},
		{
			name: "reportAll: false",
			fields: fields{
				watchInterval:      1 * time.Second,
				goroutineThreshold: 100,
				reportAll:          false,
				disableCPUProf:     false,
				disableMemProf:     false,
				stopC:              make(chan struct{}),
			},
			mockFunc: func(mockCgroupsQueryer *queryer.MockCgroupsQueryer, mockRuntimeQueryer *queryer.MockRuntimeQueryer, mockProfiler *Mockprofiler, mockReporter *report.MockReporter) {
				gomock.InOrder(
					mockRuntimeQueryer.EXPECT().
						GoroutineCount().
						AnyTimes().
						Return(200),

					mockProfiler.EXPECT().
						profileGoroutine().
						AnyTimes().
						Return([]byte("goroutine_prof"), nil),

					mockReporter.EXPECT().
						ReportGoroutineProfile(gomock.Any(), gomock.Any(), gomock.Any(), report.GoroutineInfo{
							ThresholdCount: 100,
							Count:          200,
						}).
						AnyTimes().
						Return(nil),
				)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockCgroupsQueryer := queryer.NewMockCgroupsQueryer(ctrl)
			mockRuntimeQueryer := queryer.NewMockRuntimeQueryer(ctrl)
			mockProfiler := NewMockprofiler(ctrl)
			mockReporter := report.NewMockReporter(ctrl)

			ap := &autoPprof{
				watchInterval:      tc.fields.watchInterval,
				cpuThreshold:       0.5, // 50%.
				memThreshold:       0.5, // 50%.
				goroutineThreshold: tc.fields.goroutineThreshold,
				cgroupQueryer:      mockCgroupsQueryer,
				runtimeQueryer:     mockRuntimeQueryer,
				profiler:           mockProfiler,
				reporter:           mockReporter,
				reportAll:          tc.fields.reportAll,
				disableCPUProf:     tc.fields.disableCPUProf,
				stopC:              tc.fields.stopC,
			}

			tc.mockFunc(mockCgroupsQueryer, mockRuntimeQueryer, mockProfiler, mockReporter)

			go ap.watchGoroutineCount()
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
		qryer, _ = queryer.NewCgroupQueryer()
		ticker   = time.NewTicker(defaultWatchInterval)
	)
	for i := 0; i < b.N; i++ {
		select {
		case <-ticker.C:
			_, _ = qryer.CPUUsage()
		default:
			fib(10)
		}
	}
}

func BenchmarkLightJobWithWatchMemUsage(b *testing.B) {
	var (
		qryer, _ = queryer.NewCgroupQueryer()
		ticker   = time.NewTicker(defaultWatchInterval)
	)
	for i := 0; i < b.N; i++ {
		select {
		case <-ticker.C:
			_, _ = qryer.MemUsage()
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
		qryer, _ = queryer.NewCgroupQueryer()
		ticker   = time.NewTicker(defaultWatchInterval)
	)
	for i := 0; i < b.N; i++ {
		select {
		case <-ticker.C:
			_, _ = qryer.CPUUsage()
		default:
			fib(24)
		}
	}
}

func BenchmarkHeavyJobWithWatchMemUsage(b *testing.B) {
	var (
		qryer, _ = queryer.NewCgroupQueryer()
		ticker   = time.NewTicker(defaultWatchInterval)
	)
	for i := 0; i < b.N; i++ {
		select {
		case <-ticker.C:
			_, _ = qryer.MemUsage()
		default:
			fib(24)
		}
	}
}

func fibAsync(n int) int64 {
	if n <= 1 {
		return int64(n)
	}

	var (
		v  int64
		m  sync.Mutex
		wg sync.WaitGroup
	)

	wg.Add(1)
	go func() {
		defer wg.Done()

		m.Lock()
		defer m.Unlock()
		v = fibAsync(n-1) + fibAsync(n-2)
	}()
	wg.Wait()

	return v
}

func BenchmarkLightAsyncJob(b *testing.B) {
	for i := 0; i < b.N; i++ {
		fibAsync(10)
	}
}

func BenchmarkLightAsyncJobWithWatchGoroutineCount(b *testing.B) {
	var (
		qryer, _ = queryer.NewRuntimeQueryer()
		ticker   = time.NewTicker(defaultWatchInterval)
	)
	for i := 0; i < b.N; i++ {
		select {
		case <-ticker.C:
			_ = qryer.GoroutineCount()
		default:
			fibAsync(10)
		}
	}
}

func BenchmarkHeavyAsyncJob(b *testing.B) {
	for i := 0; i < b.N; i++ {
		fibAsync(24)
	}
}

func BenchmarkHeavyAsyncJobWithWatchGoroutineCount(b *testing.B) {
	var (
		qryer, _ = queryer.NewRuntimeQueryer()
		ticker   = time.NewTicker(defaultWatchInterval)
	)
	for i := 0; i < b.N; i++ {
		select {
		case <-ticker.C:
			_ = qryer.GoroutineCount()
		default:
			fibAsync(24)
		}
	}
}
