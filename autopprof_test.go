//go:build linux
// +build linux

package autopprof

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"runtime/trace"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/daangn/autopprof/v3/queryer"
	"github.com/daangn/autopprof/v3/report"
	"github.com/golang/mock/gomock"
)

// fakeMetric is the test stub for the Metric interface. Each field
// lets a test inject behavior without spinning up a real struct.
type fakeMetric struct {
	nameVal      string
	thresholdVal float64
	intervalVal  time.Duration
	queryFn      func() (float64, error)
	collectFn    func(v float64) (CollectResult, error)
	queryCalls   atomic.Int64
	collectCalls atomic.Int64
}

func (f *fakeMetric) Name() string            { return f.nameVal }
func (f *fakeMetric) Threshold() float64      { return f.thresholdVal }
func (f *fakeMetric) Interval() time.Duration { return f.intervalVal }
func (f *fakeMetric) Query() (float64, error) {
	f.queryCalls.Add(1)
	if f.queryFn != nil {
		return f.queryFn()
	}
	return 0, nil
}
func (f *fakeMetric) Collect(v float64) (CollectResult, error) {
	f.collectCalls.Add(1)
	if f.collectFn != nil {
		return f.collectFn(v)
	}
	return CollectResult{}, nil
}

// fakeEventMetric adds an Events channel to fakeMetric. Tests pair it
// with a long interval so only events can trigger a report.
type fakeEventMetric struct {
	*fakeMetric
	events chan float64
}

func (f *fakeEventMetric) Events() <-chan float64 { return f.events }

// resetGlobal ensures test isolation — some tests Start() and don't
// Stop() cleanly; others test CAS behavior that needs the slot empty.
func resetGlobal() {
	globalAp = nil
	startOnce = sync.Once{}
	startErr = nil
	stopOnce = sync.Once{}
	stopFlightRecorder()
}

// syncBuffer is a bytes.Buffer safe to hand to log.SetOutput while a
// watcher goroutine may be logging.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// -------------------------------------------------------------------
// Validation tests
// -------------------------------------------------------------------

func TestOption_validate(t *testing.T) {
	validMetric := &fakeMetric{
		nameVal:      "custom",
		thresholdVal: 1,
		queryFn:      func() (float64, error) { return 0, nil },
		collectFn:    func(float64) (CollectResult, error) { return CollectResult{}, nil },
	}
	stub := report.NewMockReporter(gomock.NewController(t))

	testCases := []struct {
		name string
		opt  Option
		want error
	}{
		{"disable all with no custom metrics",
			Option{DisableCPUProf: true, DisableMemProf: true, DisableGoroutineProf: true, Reporter: stub},
			ErrDisableAllProfiling},
		{"disable all but one custom metric is allowed",
			Option{DisableCPUProf: true, DisableMemProf: true, DisableGoroutineProf: true, Reporter: stub, Metrics: []Metric{validMetric}},
			nil},
		{"invalid CPUThreshold",
			Option{CPUThreshold: -0.5, Reporter: stub},
			ErrInvalidCPUThreshold},
		{"invalid MemThreshold",
			Option{MemThreshold: 1.5, Reporter: stub},
			ErrInvalidMemThreshold},
		{"invalid GoroutineThreshold",
			Option{GoroutineThreshold: -1, Reporter: stub},
			ErrInvalidGoroutineThreshold},
		{"nil Reporter",
			Option{CPUThreshold: 0.8},
			ErrNilReporter},
		{"nil Metric entry",
			Option{Reporter: stub, Metrics: []Metric{nil}},
			ErrInvalidMetric},
		{"empty name",
			Option{Reporter: stub, Metrics: []Metric{&fakeMetric{thresholdVal: 1, queryFn: func() (float64, error) { return 0, nil }, collectFn: func(float64) (CollectResult, error) { return CollectResult{}, nil }}}},
			ErrInvalidMetric},
		{"negative threshold",
			Option{Reporter: stub, Metrics: []Metric{&fakeMetric{nameVal: "x", thresholdVal: -1}}},
			ErrInvalidMetric},
		{"negative interval",
			Option{Reporter: stub, Metrics: []Metric{&fakeMetric{nameVal: "x", thresholdVal: 1, intervalVal: -time.Second}}},
			ErrInvalidMetric},
		{"negative WatchInterval",
			Option{Reporter: stub, WatchInterval: -time.Second},
			ErrInvalidWatchInterval},
		{"negative ReportCooldown",
			Option{Reporter: stub, ReportCooldown: -time.Second},
			ErrInvalidReportCooldown},
		{"negative FlightRecorder.MinAge",
			Option{Reporter: stub, FlightRecorder: &FlightRecorderOption{MinAge: -time.Second}},
			ErrInvalidFlightRecorderMinAge},
		{"valid custom metric",
			Option{Reporter: stub, Metrics: []Metric{validMetric}},
			nil},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.opt.validate(); !errors.Is(err, tc.want) {
				t.Errorf("want %v, got %v", tc.want, err)
			}
		})
	}
}

// -------------------------------------------------------------------
// Built-in Metric: watch loop & Reporter routing
// -------------------------------------------------------------------

func newTestAp(t testing.TB, reporter report.Reporter) *autoPprof {
	t.Helper()
	return &autoPprof{
		watchInterval:   20 * time.Millisecond,
		reportCooldown:  60 * time.Millisecond, // 60ms cooldown for watch-loop tests
		reporter:        reporter,
		reportTimeout:   defaultReportTimeout,
		cascadedRunners: make(map[string]*metricRunner),
		stopC:           make(chan struct{}),
	}
}

func TestWatchMetric_builtinCPU_routesToReporter(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCG := queryer.NewMockCgroupsQueryer(ctrl)
	mockCG.EXPECT().CPUUsage().AnyTimes().Return(0.9, nil)
	mockProf := NewMockprofiler(ctrl)
	mockProf.EXPECT().profileCPU().AnyTimes().Return([]byte("cpu-bytes"), nil)

	var gotInfo report.ReportInfo
	var reported atomic.Int32
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(_ context.Context, r io.Reader, info report.ReportInfo) error {
			gotInfo = info
			reported.Add(1)
			return nil
		})

	ap := newTestAp(t, mockReporter)
	ap.cgroupQueryer = mockCG
	ap.profiler = mockProf
	ap.app = "myapp"
	ap.registerBuiltIn(&cpuMetric{app: ap.app, threshold: 0.75, cg: mockCG, p: mockProf})
	t.Cleanup(func() { ap.stop() })

	waitFor(t, func() bool { return reported.Load() > 0 }, time.Second)

	if gotInfo.MetricName != "cpu" {
		t.Errorf("MetricName = %q, want cpu", gotInfo.MetricName)
	}
	if gotInfo.Value != 0.9 || gotInfo.Threshold != 0.75 {
		t.Errorf("Value=%v Threshold=%v", gotInfo.Value, gotInfo.Threshold)
	}
	if !strings.Contains(gotInfo.Filename, "samples.cpu") || !strings.Contains(gotInfo.Filename, "myapp") {
		t.Errorf("Filename %q lacks expected segments", gotInfo.Filename)
	}
	if !strings.Contains(gotInfo.Comment, "[CPU]") {
		t.Errorf("Comment %q lacks [CPU]", gotInfo.Comment)
	}
}

func TestWatchMetric_builtinMem_routesToReporter(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCG := queryer.NewMockCgroupsQueryer(ctrl)
	mockCG.EXPECT().MemUsage().AnyTimes().Return(0.9, nil)
	mockProf := NewMockprofiler(ctrl)
	mockProf.EXPECT().profileHeap().AnyTimes().Return([]byte("heap-bytes"), nil)

	var gotInfo report.ReportInfo
	var reported atomic.Int32
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(_ context.Context, r io.Reader, info report.ReportInfo) error {
			gotInfo = info
			reported.Add(1)
			return nil
		})

	ap := newTestAp(t, mockReporter)
	ap.cgroupQueryer = mockCG
	ap.profiler = mockProf
	ap.registerBuiltIn(&memMetric{threshold: 0.75, cg: mockCG, p: mockProf})
	t.Cleanup(func() { ap.stop() })

	waitFor(t, func() bool { return reported.Load() > 0 }, time.Second)

	if gotInfo.MetricName != "mem" {
		t.Errorf("MetricName = %q, want mem", gotInfo.MetricName)
	}
	if !strings.Contains(gotInfo.Filename, "alloc_objects") {
		t.Errorf("Filename %q lacks heap segments", gotInfo.Filename)
	}
}

func TestWatchMetric_builtinGoroutine_routesToReporter(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockRT := queryer.NewMockRuntimeQueryer(ctrl)
	mockRT.EXPECT().GoroutineCount().AnyTimes().Return(200)
	mockProf := NewMockprofiler(ctrl)
	mockProf.EXPECT().profileGoroutine().AnyTimes().Return([]byte("g-bytes"), nil)

	var gotInfo report.ReportInfo
	var reported atomic.Int32
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(_ context.Context, r io.Reader, info report.ReportInfo) error {
			gotInfo = info
			reported.Add(1)
			return nil
		})

	ap := newTestAp(t, mockReporter)
	ap.runtimeQueryer = mockRT
	ap.profiler = mockProf
	ap.registerBuiltIn(&goroutineMetric{threshold: 100, rt: mockRT, p: mockProf})
	t.Cleanup(func() { ap.stop() })

	waitFor(t, func() bool { return reported.Load() > 0 }, time.Second)

	if gotInfo.MetricName != "goroutine" {
		t.Errorf("MetricName = %q, want goroutine", gotInfo.MetricName)
	}
	if gotInfo.Value != 200 || gotInfo.Threshold != 100 {
		t.Errorf("Value=%v Threshold=%v", gotInfo.Value, gotInfo.Threshold)
	}
}

// -------------------------------------------------------------------
// Debounce (ReportCooldown)
// -------------------------------------------------------------------

func TestWatchMetric_debounce(t *testing.T) {
	ctrl := gomock.NewController(t)
	var reported atomic.Int32
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(_ context.Context, _ io.Reader, _ report.ReportInfo) error {
			reported.Add(1)
			return nil
		})

	fm := &fakeMetric{
		nameVal:      "dbn",
		thresholdVal: 10,
		intervalVal:  20 * time.Millisecond,
		queryFn:      func() (float64, error) { return 100, nil },
		collectFn: func(float64) (CollectResult, error) {
			return CollectResult{Reader: bytes.NewReader([]byte("x")), Filename: "f", Comment: "c"}, nil
		},
	}
	ap := newTestAp(t, mockReporter)
	ap.reportCooldown = 60 * time.Millisecond // 60ms cooldown, 20ms ticks
	if err := ap.registerMetric(fm); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ap.stop() })

	// Five ticks @20ms ≈ 100ms — first and fourth ticks should fire.
	time.Sleep(170 * time.Millisecond)
	got := reported.Load()
	if got < 2 || got > 3 {
		t.Errorf("expected 2-3 reports with 60ms cooldown, got %d", got)
	}
}

// TestWatchMetric_cooldownReArmsBelowThreshold verifies the cooldown is
// per-episode: dropping below the threshold re-arms an immediate report
// on the next breach, even while the cooldown window is still open.
func TestWatchMetric_cooldownReArmsBelowThreshold(t *testing.T) {
	ctrl := gomock.NewController(t)
	var reported atomic.Int32
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(_ context.Context, _ io.Reader, _ report.ReportInfo) error {
			reported.Add(1)
			return nil
		})

	// Query per tick: over, over, under, over, over... With a 10s cooldown
	// a pure time throttle would report once; the dip under threshold on
	// the 3rd tick must re-arm so the 4th tick reports again.
	var calls atomic.Int32
	fm := &fakeMetric{
		nameVal:      "rearm",
		thresholdVal: 1,
		intervalVal:  20 * time.Millisecond,
		queryFn: func() (float64, error) {
			if calls.Add(1) == 3 {
				return 0, nil // under threshold → re-arm
			}
			return 100, nil // over threshold
		},
		collectFn: func(float64) (CollectResult, error) {
			return CollectResult{Reader: bytes.NewReader([]byte("x"))}, nil
		},
	}
	ap := newTestAp(t, mockReporter)
	ap.reportCooldown = 10 * time.Second // only a re-arm can re-fire this fast
	if err := ap.registerMetric(fm); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ap.stop() })

	waitFor(t, func() bool { return reported.Load() >= 2 }, time.Second)
	if got := reported.Load(); got != 2 {
		t.Errorf("expected exactly 2 reports (breach + re-arm), got %d", got)
	}
}

// -------------------------------------------------------------------
// Cascade
// -------------------------------------------------------------------

func TestCascadeBuiltIn(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCG := queryer.NewMockCgroupsQueryer(ctrl)
	mockCG.EXPECT().CPUUsage().AnyTimes().Return(0.9, nil)
	mockCG.EXPECT().MemUsage().AnyTimes().Return(0.1, nil) // below threshold
	mockRT := queryer.NewMockRuntimeQueryer(ctrl)
	mockRT.EXPECT().GoroutineCount().AnyTimes().Return(1) // below threshold
	mockProf := NewMockprofiler(ctrl)
	mockProf.EXPECT().profileCPU().AnyTimes().Return([]byte("c"), nil)
	mockProf.EXPECT().profileHeap().AnyTimes().Return([]byte("h"), nil)
	mockProf.EXPECT().profileGoroutine().AnyTimes().Return([]byte("g"), nil)

	var cpuCnt, memCnt, goCnt atomic.Int32
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(_ context.Context, _ io.Reader, info report.ReportInfo) error {
			switch info.MetricName {
			case "cpu":
				cpuCnt.Add(1)
			case "mem":
				memCnt.Add(1)
			case "goroutine":
				goCnt.Add(1)
			}
			return nil
		})

	ap := newTestAp(t, mockReporter)
	ap.cgroupQueryer = mockCG
	ap.runtimeQueryer = mockRT
	ap.profiler = mockProf
	ap.reportCooldown = 20 * time.Second // long cooldown — suppress repeats
	ap.registerBuiltIn(&cpuMetric{threshold: 0.5, cg: mockCG, p: mockProf})
	ap.registerBuiltIn(&memMetric{threshold: 0.5, cg: mockCG, p: mockProf})
	ap.registerBuiltIn(&goroutineMetric{threshold: 5, rt: mockRT, p: mockProf})
	t.Cleanup(func() { ap.stop() })

	// Only CPU is over threshold; cascade should fire Mem and Goroutine too.
	waitFor(t, func() bool {
		return cpuCnt.Load() > 0 && memCnt.Load() > 0 && goCnt.Load() > 0
	}, 2*time.Second)
	if cpuCnt.Load() == 0 || memCnt.Load() == 0 || goCnt.Load() == 0 {
		t.Errorf("cascade should fire all built-ins cpu=%d mem=%d goroutine=%d",
			cpuCnt.Load(), memCnt.Load(), goCnt.Load())
	}
}

// -------------------------------------------------------------------
// User metric: trigger, independence, interval, nil reader, defaults
// -------------------------------------------------------------------

func TestUserMetric_trigger(t *testing.T) {
	ctrl := gomock.NewController(t)
	var infoSeen report.ReportInfo
	var reported atomic.Int32
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(_ context.Context, _ io.Reader, info report.ReportInfo) error {
			infoSeen = info
			reported.Add(1)
			return nil
		})

	fm := &fakeMetric{
		nameVal:      "mycustom",
		thresholdVal: 10,
		intervalVal:  20 * time.Millisecond,
		queryFn:      func() (float64, error) { return 42, nil },
		collectFn: func(v float64) (CollectResult, error) {
			return CollectResult{
				Reader:   bytes.NewReader([]byte("custom-bytes")),
				Filename: "user.dump",
				Comment:  "user comment",
			}, nil
		},
	}
	ap := newTestAp(t, mockReporter)
	if err := ap.registerMetric(fm); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ap.stop() })

	waitFor(t, func() bool { return reported.Load() > 0 }, time.Second)

	if infoSeen.MetricName != "mycustom" || infoSeen.Filename != "user.dump" ||
		infoSeen.Comment != "user comment" || infoSeen.Value != 42 ||
		infoSeen.Threshold != 10 {
		t.Errorf("unexpected info: %+v", infoSeen)
	}
}

func TestUserMetric_independent_noCascade(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCG := queryer.NewMockCgroupsQueryer(ctrl)
	mockCG.EXPECT().CPUUsage().AnyTimes().Return(0.1, nil)
	mockRT := queryer.NewMockRuntimeQueryer(ctrl)
	mockRT.EXPECT().GoroutineCount().AnyTimes().Return(1)
	mockProf := NewMockprofiler(ctrl)

	var names []string
	var mu sync.Mutex
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(_ context.Context, _ io.Reader, info report.ReportInfo) error {
			mu.Lock()
			names = append(names, info.MetricName)
			mu.Unlock()
			return nil
		})

	ap := newTestAp(t, mockReporter)
	ap.cgroupQueryer = mockCG
	ap.runtimeQueryer = mockRT
	ap.profiler = mockProf
	// Built-in cascade must not pick up user metrics.
	ap.registerBuiltIn(&cpuMetric{threshold: 0.5, cg: mockCG, p: mockProf})

	fm := &fakeMetric{
		nameVal: "customonly", thresholdVal: 10, intervalVal: 20 * time.Millisecond,
		queryFn: func() (float64, error) { return 100, nil },
		collectFn: func(float64) (CollectResult, error) {
			return CollectResult{Reader: bytes.NewReader([]byte("b"))}, nil
		},
	}
	if err := ap.registerMetric(fm); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ap.stop() })

	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, n := range names {
			if n == "customonly" {
				return true
			}
		}
		return false
	}, time.Second)

	mu.Lock()
	defer mu.Unlock()
	for _, n := range names {
		if n == "cpu" {
			t.Errorf("custom trigger caused cpu cascade: %v", names)
			return
		}
	}
}

func TestUserMetric_perMetricInterval(t *testing.T) {
	ctrl := gomock.NewController(t)
	var reported atomic.Int32
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(_ context.Context, _ io.Reader, _ report.ReportInfo) error {
			reported.Add(1)
			return nil
		})

	fm := &fakeMetric{
		nameVal: "fast", thresholdVal: 1,
		intervalVal: 10 * time.Millisecond, // much faster than global
		queryFn:     func() (float64, error) { return 10, nil },
		collectFn: func(float64) (CollectResult, error) {
			return CollectResult{Reader: bytes.NewReader([]byte("b"))}, nil
		},
	}
	ap := newTestAp(t, mockReporter)
	ap.watchInterval = 10 * time.Second // global should be ignored
	if err := ap.registerMetric(fm); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ap.stop() })

	waitFor(t, func() bool { return reported.Load() > 0 }, 500*time.Millisecond)
}

func TestUserMetric_nilReaderSkipsReporter(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockReporter := report.NewMockReporter(ctrl)
	// If Report is ever called, the test fails.
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	fm := &fakeMetric{
		nameVal: "noreport", thresholdVal: 1, intervalVal: 20 * time.Millisecond,
		queryFn: func() (float64, error) { return 10, nil },
		collectFn: func(float64) (CollectResult, error) {
			return CollectResult{Reader: nil}, nil
		},
	}
	ap := newTestAp(t, mockReporter)
	if err := ap.registerMetric(fm); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ap.stop() })
	time.Sleep(80 * time.Millisecond)
	// Test passes if the mock's Times(0) expectation is satisfied by gomock.
}

func TestUserMetric_defaultFilenameComment(t *testing.T) {
	ctrl := gomock.NewController(t)
	var info report.ReportInfo
	var done atomic.Int32
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(_ context.Context, _ io.Reader, i report.ReportInfo) error {
			info = i
			done.Add(1)
			return nil
		})

	fm := &fakeMetric{
		nameVal: "defaults", thresholdVal: 1, intervalVal: 20 * time.Millisecond,
		queryFn: func() (float64, error) { return 10, nil },
		collectFn: func(float64) (CollectResult, error) {
			return CollectResult{Reader: bytes.NewReader([]byte("x"))}, nil
		},
	}
	ap := newTestAp(t, mockReporter)
	if err := ap.registerMetric(fm); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ap.stop() })

	waitFor(t, func() bool { return done.Load() > 0 }, time.Second)
	if !strings.Contains(info.Filename, "defaults.") || !strings.Contains(info.Filename, ".bin") {
		t.Errorf("default Filename=%q not generated", info.Filename)
	}
	if !strings.Contains(info.Comment, "[defaults]") {
		t.Errorf("default Comment=%q not generated", info.Comment)
	}
}

// -------------------------------------------------------------------
// Register lifecycle
// -------------------------------------------------------------------

func TestRegister_errNotStarted(t *testing.T) {
	resetGlobal()
	m := &fakeMetric{nameVal: "x", thresholdVal: 1,
		queryFn: func() (float64, error) { return 0, nil }}
	if err := Register(m); !errors.Is(err, ErrNotStarted) {
		t.Errorf("want ErrNotStarted, got %v", err)
	}
}

func TestRegister_stopsOnAutoPprofStop(t *testing.T) {
	ctrl := gomock.NewController(t)
	var after atomic.Int32
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(_ context.Context, _ io.Reader, _ report.ReportInfo) error {
			after.Add(1)
			return nil
		})

	fm := &fakeMetric{nameVal: "gone", thresholdVal: 1, intervalVal: 20 * time.Millisecond,
		queryFn: func() (float64, error) { return 10, nil },
		collectFn: func(float64) (CollectResult, error) {
			return CollectResult{Reader: bytes.NewReader([]byte("x"))}, nil
		}}
	ap := newTestAp(t, mockReporter)
	if err := ap.registerMetric(fm); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return after.Load() > 0 }, time.Second)

	ap.stop()
	snap := after.Load()
	time.Sleep(100 * time.Millisecond)
	if delta := after.Load() - snap; delta != 0 {
		t.Errorf("after Stop, got %d more reports", delta)
	}
}

// -------------------------------------------------------------------
// Query error keeps the watcher alive
// -------------------------------------------------------------------

// TestWatchMetric_queryErrorKeepsWatching: a transient Query error
// used to return from the watcher, leaving the metric unwatched for the
// life of the process. It must now survive and report the next breach.
func TestWatchMetric_queryErrorKeepsWatching(t *testing.T) {
	ctrl := gomock.NewController(t)
	var reported atomic.Int32
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(_ context.Context, _ io.Reader, _ report.ReportInfo) error {
			reported.Add(1)
			return nil
		})

	var calls atomic.Int32
	flaky := &fakeMetric{nameVal: "flaky", thresholdVal: 1, intervalVal: 20 * time.Millisecond,
		queryFn: func() (float64, error) {
			if calls.Add(1) <= 2 {
				return 0, errors.New("boom")
			}
			return 100, nil
		},
		collectFn: func(float64) (CollectResult, error) {
			return CollectResult{Reader: bytes.NewReader([]byte("x"))}, nil
		}}
	ap := newTestAp(t, mockReporter)
	if err := ap.registerMetric(flaky); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ap.stop() })

	waitFor(t, func() bool { return reported.Load() > 0 }, time.Second)
}

// TestWatchMetric_queryErrorLogsOnTransition: a Query that keeps
// failing must not log on every tick — once on failure, once on
// recovery.
func TestWatchMetric_queryErrorLogsOnTransition(t *testing.T) {
	var logs syncBuffer
	prev := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(prev) })

	ctrl := gomock.NewController(t)
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	var calls atomic.Int32
	m := &fakeMetric{nameVal: "noisy", thresholdVal: 1, intervalVal: 10 * time.Millisecond,
		queryFn: func() (float64, error) {
			if calls.Add(1) <= 5 {
				return 0, errors.New("boom")
			}
			return 0, nil // recovered, below threshold
		}}
	ap := newTestAp(t, mockReporter)
	if err := ap.registerMetric(m); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return m.queryCalls.Load() >= 8 }, time.Second)
	ap.stop()

	out := logs.String()
	if got := strings.Count(out, `"noisy" query failed`); got != 1 {
		t.Errorf("query failed logged %d times, want 1:\n%s", got, out)
	}
	if got := strings.Count(out, `"noisy" query recovered`); got != 1 {
		t.Errorf("query recovered logged %d times, want 1:\n%s", got, out)
	}
}

// -------------------------------------------------------------------
// EventMetric: immediate trigger, independent cooldown
// -------------------------------------------------------------------

func newFakeEventMetric(name string, collect func(float64) (CollectResult, error)) *fakeEventMetric {
	return &fakeEventMetric{
		fakeMetric: &fakeMetric{
			nameVal:      name,
			thresholdVal: 1,
			intervalVal:  time.Hour, // the ticker never fires within a test
			collectFn:    collect,
		},
		events: make(chan float64, 1),
	}
}

func readerCollect(float64) (CollectResult, error) {
	return CollectResult{Reader: bytes.NewReader([]byte("x"))}, nil
}

func TestEventMetric_firesWithoutTick(t *testing.T) {
	ctrl := gomock.NewController(t)
	var gotInfo report.ReportInfo
	var reported atomic.Int32
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(_ context.Context, _ io.Reader, info report.ReportInfo) error {
			gotInfo = info
			reported.Add(1)
			return nil
		})

	em := newFakeEventMetric("evt", readerCollect)
	ap := newTestAp(t, mockReporter)
	ap.reportCooldown = 10 * time.Second
	if err := ap.registerMetric(em); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ap.stop() })

	em.events <- 0.42
	waitFor(t, func() bool { return reported.Load() == 1 }, 500*time.Millisecond)

	if gotInfo.MetricName != "evt" || gotInfo.Value != 0.42 {
		t.Errorf("unexpected info: %+v", gotInfo)
	}
	if n := em.queryCalls.Load(); n != 0 {
		t.Errorf("event path must not Query, got %d calls", n)
	}
}

// TestEventMetric_tickerReArmDoesNotResetEventCooldown: the bulkhead's
// live ratio drops right after a trip, so below-threshold ticks follow
// every event. If they re-armed the event path, cooldown would be moot.
func TestEventMetric_tickerReArmDoesNotResetEventCooldown(t *testing.T) {
	ctrl := gomock.NewController(t)
	var reported atomic.Int32
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(_ context.Context, _ io.Reader, _ report.ReportInfo) error {
			reported.Add(1)
			return nil
		})

	em := newFakeEventMetric("rearm-evt", readerCollect)
	em.intervalVal = 10 * time.Millisecond
	em.queryFn = func() (float64, error) { return 0, nil } // always below → ticker re-arms
	ap := newTestAp(t, mockReporter)
	ap.reportCooldown = 10 * time.Second
	if err := ap.registerMetric(em); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ap.stop() })

	em.events <- 1
	waitFor(t, func() bool { return reported.Load() == 1 }, 500*time.Millisecond)
	ticks := em.queryCalls.Load()
	waitFor(t, func() bool { return em.queryCalls.Load() >= ticks+3 }, time.Second)

	em.events <- 1
	time.Sleep(100 * time.Millisecond)
	if got := em.collectCalls.Load(); got != 1 {
		t.Errorf("second event within cooldown collected: collectCalls=%d, want 1", got)
	}
}

// TestEventMetric_eventReportDoesNotSuppressTicker: the reverse
// direction — an event report leaves the ticker's own debounce alone.
func TestEventMetric_eventReportDoesNotSuppressTicker(t *testing.T) {
	ctrl := gomock.NewController(t)
	var reported atomic.Int32
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(func(_ context.Context, _ io.Reader, _ report.ReportInfo) error {
			reported.Add(1)
			return nil
		})

	var over atomic.Bool
	em := newFakeEventMetric("both", readerCollect)
	em.intervalVal = 10 * time.Millisecond
	em.queryFn = func() (float64, error) {
		if over.Load() {
			return 100, nil
		}
		return 0, nil
	}
	ap := newTestAp(t, mockReporter)
	ap.reportCooldown = 10 * time.Second
	if err := ap.registerMetric(em); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ap.stop() })

	em.events <- 1
	waitFor(t, func() bool { return reported.Load() == 1 }, 500*time.Millisecond)
	over.Store(true)
	waitFor(t, func() bool { return reported.Load() == 2 }, 500*time.Millisecond)
}

// TestEventMetric_eventsDuringCollectDropped: Collect can run for 30s+.
// Events arriving meanwhile collapse into the 1-slot buffer and must
// then be swallowed by the cooldown instead of firing a second bundle.
func TestEventMetric_eventsDuringCollectDropped(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	em := newFakeEventMetric("slow", func(v float64) (CollectResult, error) {
		entered <- struct{}{}
		<-release
		return readerCollect(v)
	})
	ap := newTestAp(t, mockReporter)
	ap.reportCooldown = 10 * time.Second
	if err := ap.registerMetric(em); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ap.stop() })

	em.events <- 1
	select {
	case <-entered:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Collect never started")
	}
	// The sender's non-blocking send: the first lands in the buffer,
	// the rest are dropped at the source.
	for i := 0; i < 5; i++ {
		select {
		case em.events <- 2:
		default:
		}
	}
	close(release)
	time.Sleep(100 * time.Millisecond)

	if got := em.collectCalls.Load(); got != 1 {
		t.Errorf("collectCalls=%d, want 1 (queued event must hit cooldown)", got)
	}
}

// TestEventMetric_cooldownStartsAfterCollect: when Collect outlasts the
// cooldown, the event buffered during it must still be dropped — the
// cooldown runs from when Collect returns. A fresh event after the
// cooldown fires again.
func TestEventMetric_cooldownStartsAfterCollect(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	const cooldown = 50 * time.Millisecond
	entered := make(chan struct{}, 4)
	var slow atomic.Bool
	slow.Store(true)
	em := newFakeEventMetric("long", func(v float64) (CollectResult, error) {
		entered <- struct{}{}
		if slow.Load() {
			time.Sleep(4 * cooldown) // Collect outlasts the cooldown
		}
		return readerCollect(v)
	})
	ap := newTestAp(t, mockReporter)
	ap.reportCooldown = cooldown
	if err := ap.registerMetric(em); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ap.stop() })

	em.events <- 1
	<-entered
	em.events <- 2 // buffered while Collect runs
	waitFor(t, func() bool { return len(em.events) == 0 }, time.Second)
	time.Sleep(cooldown / 5)
	if got := em.collectCalls.Load(); got != 1 {
		t.Fatalf("buffered event fired after a long Collect: collectCalls=%d, want 1", got)
	}

	slow.Store(false)
	time.Sleep(2 * cooldown)
	em.events <- 3
	waitFor(t, func() bool { return em.collectCalls.Load() == 2 }, time.Second)
}

// TestEventMetric_closedChannel: a closed channel is always ready; the
// watcher must stop selecting on it rather than spin reporting zeros.
func TestEventMetric_closedChannel(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	em := newFakeEventMetric("closed", readerCollect)
	em.intervalVal = 10 * time.Millisecond
	em.queryFn = func() (float64, error) { return 0, nil }
	close(em.events)
	ap := newTestAp(t, mockReporter)
	ap.reportCooldown = 0
	if err := ap.registerMetric(em); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ap.stop() })

	waitFor(t, func() bool { return em.queryCalls.Load() >= 3 }, time.Second)
	if got := em.collectCalls.Load(); got != 0 {
		t.Errorf("closed channel produced %d collects, want 0", got)
	}
}

// -------------------------------------------------------------------
// Start / Stop own the flight recorder
// -------------------------------------------------------------------

func startOnlyCustom(t *testing.T, fr *FlightRecorderOption) error {
	t.Helper()
	stub := report.NewMockReporter(gomock.NewController(t))
	stub.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	return Start(Option{
		DisableCPUProf:       true,
		DisableMemProf:       true,
		DisableGoroutineProf: true,
		Reporter:             stub,
		Metrics:              []Metric{&fakeMetric{nameVal: "idle", thresholdVal: 1, intervalVal: time.Hour}},
		FlightRecorder:       fr,
	})
}

func TestStart_flightRecorderLifecycle(t *testing.T) {
	resetGlobal()
	t.Cleanup(resetGlobal)

	if err := startOnlyCustom(t, &FlightRecorderOption{MaxBytes: 1 << 20}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	spawnSome()

	var buf bytes.Buffer
	if _, err := WriteTrace(&buf); err != nil {
		t.Fatalf("WriteTrace after Start: %v", err)
	}
	if h := buf.Bytes()[:16]; !traceHeader.Match(h) {
		t.Errorf("trace header = %q", h)
	}

	Stop()
	if _, err := WriteTrace(io.Discard); !errors.Is(err, ErrFlightRecorderDisabled) {
		t.Errorf("WriteTrace after Stop = %v, want ErrFlightRecorderDisabled", err)
	}
}

func TestStart_withoutFlightRecorder(t *testing.T) {
	resetGlobal()
	t.Cleanup(resetGlobal)

	if err := startOnlyCustom(t, nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := WriteTrace(io.Discard); !errors.Is(err, ErrFlightRecorderDisabled) {
		t.Errorf("WriteTrace = %v, want ErrFlightRecorderDisabled", err)
	}
}

func TestStart_flightRecorderFailFast(t *testing.T) {
	resetGlobal()
	t.Cleanup(resetGlobal)

	other := trace.NewFlightRecorder(trace.FlightRecorderConfig{})
	if err := other.Start(); err != nil {
		t.Fatalf("start other recorder: %v", err)
	}
	t.Cleanup(other.Stop)

	err := startOnlyCustom(t, &FlightRecorderOption{})
	if err == nil || !strings.Contains(err.Error(), "flight recorder") {
		t.Fatalf("Start = %v, want flight recorder start error", err)
	}
	if globalAp != nil {
		t.Error("globalAp set despite failed Start")
	}
}

// -------------------------------------------------------------------
// Stop idempotency
// -------------------------------------------------------------------

func TestStop_idempotent(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockReporter := report.NewMockReporter(ctrl)
	ap := newTestAp(t, mockReporter)
	ap.stop()
	ap.stop() // must not panic (sync.Once)
}

// -------------------------------------------------------------------
// Concurrency: Register under -race
// -------------------------------------------------------------------

func TestRegister_concurrent(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockReporter := report.NewMockReporter(ctrl)
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	ap := newTestAp(t, mockReporter)
	t.Cleanup(func() { ap.stop() })

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := fmt.Sprintf("m%d", id)
			m := &fakeMetric{nameVal: name, thresholdVal: 1, intervalVal: 10 * time.Millisecond,
				queryFn:   func() (float64, error) { return 0, nil },
				collectFn: func(float64) (CollectResult, error) { return CollectResult{}, nil }}
			_ = ap.registerMetric(m)
		}(i)
	}
	wg.Wait()
}

// -------------------------------------------------------------------
// NewMetric nil-function defense
// -------------------------------------------------------------------

func TestNewMetric_nilQueryCollect(t *testing.T) {
	m := NewMetric("x", 0, 0, nil, nil)
	if _, err := m.Query(); !errors.Is(err, ErrInvalidMetric) {
		t.Errorf("Query with nil fn should be ErrInvalidMetric, got %v", err)
	}
	if _, err := m.Collect(0); !errors.Is(err, ErrInvalidMetric) {
		t.Errorf("Collect with nil fn should be ErrInvalidMetric, got %v", err)
	}
}

// -------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------

func waitFor(t *testing.T, cond func() bool, total time.Duration) {
	t.Helper()
	deadline := time.Now().Add(total)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v", total)
}

// -------------------------------------------------------------------
// Benchmarks — measure overhead of watching vs. a bare workload.
// -------------------------------------------------------------------

func fib(n int) int64 {
	if n <= 1 {
		return int64(n)
	}
	return fib(n-1) + fib(n-2)
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

func BenchmarkLightJob(b *testing.B) {
	for i := 0; i < b.N; i++ {
		fib(10)
	}
}

func BenchmarkLightJobWithWatchCPUUsage(b *testing.B) {
	qryer, _ := queryer.NewCgroupQueryer()
	ticker := time.NewTicker(defaultWatchInterval)
	defer ticker.Stop()
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
	qryer, _ := queryer.NewCgroupQueryer()
	ticker := time.NewTicker(defaultWatchInterval)
	defer ticker.Stop()
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
	qryer, _ := queryer.NewCgroupQueryer()
	ticker := time.NewTicker(defaultWatchInterval)
	defer ticker.Stop()
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
	qryer, _ := queryer.NewCgroupQueryer()
	ticker := time.NewTicker(defaultWatchInterval)
	defer ticker.Stop()
	for i := 0; i < b.N; i++ {
		select {
		case <-ticker.C:
			_, _ = qryer.MemUsage()
		default:
			fib(24)
		}
	}
}

func BenchmarkLightAsyncJob(b *testing.B) {
	for i := 0; i < b.N; i++ {
		fibAsync(10)
	}
}

func BenchmarkLightAsyncJobWithWatchGoroutineCount(b *testing.B) {
	qryer, _ := queryer.NewRuntimeQueryer()
	ticker := time.NewTicker(defaultWatchInterval)
	defer ticker.Stop()
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
	qryer, _ := queryer.NewRuntimeQueryer()
	ticker := time.NewTicker(defaultWatchInterval)
	defer ticker.Stop()
	for i := 0; i < b.N; i++ {
		select {
		case <-ticker.C:
			_ = qryer.GoroutineCount()
		default:
			fibAsync(24)
		}
	}
}
