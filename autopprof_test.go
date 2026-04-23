//go:build linux
// +build linux

package autopprof

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/daangn/autopprof/v2/queryer"
	"github.com/daangn/autopprof/v2/report"
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

// resetGlobal ensures test isolation — some tests Start() and don't
// Stop() cleanly; others test CAS behavior that needs the slot empty.
func resetGlobal() { globalAp = nil }

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

func newTestAp(t *testing.T, reporter report.Reporter) *autoPprof {
	t.Helper()
	return &autoPprof{
		watchInterval:               20 * time.Millisecond,
		minConsecutiveOverThreshold: 3,
		reporter:                    reporter,
		cascadedRunners:             make(map[string]*metricRunner),
		stopC:                       make(chan struct{}),
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
// Debounce (minConsecutiveOverThreshold)
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
	ap.minConsecutiveOverThreshold = 3
	if err := ap.registerMetric(fm); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ap.stop() })

	// Five ticks @20ms ≈ 100ms — first and fourth ticks should fire.
	time.Sleep(170 * time.Millisecond)
	got := reported.Load()
	if got < 2 || got > 3 {
		t.Errorf("expected 2-3 reports with debounce=3, got %d", got)
	}
}

// -------------------------------------------------------------------
// Cascade (reportAll)
// -------------------------------------------------------------------

func TestCascadeBuiltIn_reportAll(t *testing.T) {
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
	ap.reportAll = true
	ap.minConsecutiveOverThreshold = 1000
	ap.registerBuiltIn(&cpuMetric{threshold: 0.5, cg: mockCG, p: mockProf})
	ap.registerBuiltIn(&memMetric{threshold: 0.5, cg: mockCG, p: mockProf})
	ap.registerBuiltIn(&goroutineMetric{threshold: 5, rt: mockRT, p: mockProf})
	t.Cleanup(func() { ap.stop() })

	// Only CPU is over threshold; reportAll should cascade to Mem and Goroutine too.
	waitFor(t, func() bool {
		return cpuCnt.Load() > 0 && memCnt.Load() > 0 && goCnt.Load() > 0
	}, 2*time.Second)
	if cpuCnt.Load() == 0 || memCnt.Load() == 0 || goCnt.Load() == 0 {
		t.Errorf("ReportAll should cascade cpu=%d mem=%d goroutine=%d",
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
	ap.reportAll = true // confirms cascade is limited to built-in
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
// Query error terminates the watcher
// -------------------------------------------------------------------

func TestWatchMetric_queryErrorExitsWatcher(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockReporter := report.NewMockReporter(ctrl)
	// Reporter must never be called; Query errored before any fire.
	mockReporter.EXPECT().Report(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	ap := newTestAp(t, mockReporter)

	broken := &fakeMetric{nameVal: "boom", thresholdVal: 1, intervalVal: 20 * time.Millisecond,
		queryFn:   func() (float64, error) { return 0, errors.New("boom") },
		collectFn: func(float64) (CollectResult, error) { return CollectResult{}, nil }}
	if err := ap.registerMetric(broken); err != nil {
		t.Fatal(err)
	}

	// Give the ticker one or two fires then Stop should return promptly,
	// proving the watcher exited on its own.
	time.Sleep(60 * time.Millisecond)
	ap.stop()
	if n := broken.queryCalls.Load(); n < 1 {
		t.Errorf("expected at least one Query call, got %d", n)
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
