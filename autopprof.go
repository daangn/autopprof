//go:build linux
// +build linux

package autopprof

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/daangn/autopprof/v2/queryer"
	"github.com/daangn/autopprof/v2/report"
)

const (
	reportTimeout = 5 * time.Second
)

// autoPprof is the internal singleton holding all live Metric watchers.
// The unified Metric interface lets CPU / Mem / Goroutine and any
// user-registered metric share one watch loop, one debounce counter
// per metric, and one Reporter path.
type autoPprof struct {
	watchInterval               time.Duration
	minConsecutiveOverThreshold int

	reporter report.Reporter
	// app is the "<app>" segment used by built-in filenames
	// (sourced from Option.App).
	app string

	reportAll bool

	disableCPUProf       bool
	disableMemProf       bool
	disableGoroutineProf bool

	cgroupQueryer  queryer.CgroupsQueryer
	runtimeQueryer queryer.RuntimeQueryer
	profiler       profiler

	// metricsMu guards only the metrics map: Register/Unregister
	// insert and delete, cascadeBuiltIn snapshots the target list,
	// stop drains the map. Work *outside* the map (Query, Collect,
	// Reporter.Report, pprof.StartCPUProfile) runs without the lock;
	// the cgroup queryer and profiler protect their own shared state
	// internally.
	metricsMu sync.Mutex
	metrics   map[string]*metricRunner

	// wg tracks every live watcher goroutine so Stop() blocks until
	// in-flight pprof work (CPU profiling can run up to ~10s) unwinds.
	wg sync.WaitGroup

	// stopOnce guards against double close of stopC / wg.Wait.
	stopOnce sync.Once

	// stopC broadcasts shutdown to every watcher goroutine.
	stopC chan struct{}
}

// metricRunner wraps a Metric with the runtime bookkeeping for its
// watcher goroutine.
//
// The underlying Metric implementations protect their own shared
// state: cgroup queryer has qMu for its CPU snapshot queue, profiler
// has cpuMu around pprof.StartCPUProfile. That keeps the built-in
// concurrency invariants at the source and lets this layer stay thin.
type metricRunner struct {
	metric    Metric
	name      string        // cached m.Name() at registration
	threshold float64       // cached m.Threshold() at registration
	interval  time.Duration // cached m.Interval() (0 resolved to global)
	builtIn   bool

	// stopC stops just this metric (via Unregister) without affecting
	// the rest of the instance. Only one code path ever closes it —
	// Unregister removes the metric from the map before closing,
	// stop() iterates the map under metricsMu — so double-close is
	// structurally impossible.
	stopC chan struct{}
}

// globalAp holds the current running autoPprof instance, or nil when
// no Start() has succeeded yet. Start and Stop are expected at process
// init / shutdown only, so no atomic protection is needed.
var globalAp *autoPprof

// Start configures and runs the autopprof process. Call it once at
// startup; a second Start replaces the previous instance's pointer
// but the previous watchers keep running until they observe their
// own stopC (i.e. it is the caller's responsibility not to call
// Start concurrently with itself or Stop).
func Start(opt Option) error {
	cgroupQryer, err := queryer.NewCgroupQueryer()
	if err != nil {
		return err
	}
	runtimeQryer, err := queryer.NewRuntimeQueryer()
	if err != nil {
		return err
	}
	if err := opt.validate(); err != nil {
		return err
	}

	profr := newDefaultProfiler(defaultCPUProfilingDuration)
	ap := &autoPprof{
		watchInterval:               defaultWatchInterval,
		minConsecutiveOverThreshold: defaultMinConsecutiveOverThreshold,
		reporter:                    opt.Reporter,
		app:                         opt.App,
		reportAll:                   opt.ReportAll,
		disableCPUProf:              opt.DisableCPUProf,
		disableMemProf:              opt.DisableMemProf,
		disableGoroutineProf:        opt.DisableGoroutineProf,
		cgroupQueryer:               cgroupQryer,
		runtimeQueryer:              runtimeQryer,
		profiler:                    profr,
		metrics:                     make(map[string]*metricRunner),
		stopC:                       make(chan struct{}),
	}
	if !ap.disableCPUProf {
		if err := ap.loadCPUQuota(); err != nil {
			return err
		}
	}
	ap.registerBuiltinMetrics(opt)
	for _, m := range opt.Metrics {
		if err := ap.registerMetric(m); err != nil {
			ap.stop()
			return err
		}
	}
	globalAp = ap
	return nil
}

// Stop stops the global autopprof process. sync.Once inside ap.stop()
// keeps repeated calls safe.
func Stop() {
	if globalAp == nil {
		return
	}
	globalAp.stop()
	globalAp = nil
}

// Register adds a user Metric to the running autopprof instance.
// Must be called after Start.
//
// Returns:
//   - ErrNotStarted                     if Start has not been called.
//   - ErrInvalidMetric / ErrReservedMetricName / ErrMetricAlreadyRegistered
//     for validation failures.
func Register(m Metric) error {
	if globalAp == nil {
		return ErrNotStarted
	}
	return globalAp.registerMetric(m)
}

// Unregister removes a user Metric by name and stops its watcher.
// Built-in metrics (cpu/mem/goroutine) return ErrCannotUnregisterBuiltIn;
// unknown names return ErrMetricNotRegistered.
func Unregister(name string) error {
	if globalAp == nil {
		return ErrNotStarted
	}
	return globalAp.unregisterMetric(name)
}

// loadCPUQuota resolves CPU limits for the container. If the cgroup
// quota is not available we log and silently disable CPU profiling so
// the rest of the library can keep working (same behavior as v1).
func (ap *autoPprof) loadCPUQuota() error {
	err := ap.cgroupQueryer.SetCPUQuota()
	if err == nil {
		return nil
	}

	// If memory profiling is disabled and CPU quota isn't set,
	//  returns an error immediately.
	if ap.disableMemProf {
		return err
	}
	// If memory profiling is enabled, just logs the error and
	//  disables the cpu profiling.
	log.Println(
		"autopprof: disable the cpu profiling due to the CPU quota isn't set",
	)
	ap.disableCPUProf = true
	return nil
}

// registerBuiltinMetrics installs the pre-defined CPU / Mem / Goroutine
// metrics unless their Disable flag says otherwise. Built-in
// registration cannot fail, so this function is void; user-supplied
// Option.Metrics are registered separately by Start().
func (ap *autoPprof) registerBuiltinMetrics(opt Option) {
	cpuThreshold := defaultCPUThreshold
	if opt.CPUThreshold != 0 {
		cpuThreshold = opt.CPUThreshold
	}
	memThreshold := defaultMemThreshold
	if opt.MemThreshold != 0 {
		memThreshold = opt.MemThreshold
	}
	goroutineThreshold := defaultGoroutineThreshold
	if opt.GoroutineThreshold != 0 {
		goroutineThreshold = opt.GoroutineThreshold
	}

	if !ap.disableCPUProf {
		ap.registerBuiltIn(&cpuMetric{
			app: ap.app, threshold: cpuThreshold,
			cg: ap.cgroupQueryer, p: ap.profiler,
		})
	}
	if !ap.disableMemProf {
		ap.registerBuiltIn(&memMetric{
			app: ap.app, threshold: memThreshold,
			cg: ap.cgroupQueryer, p: ap.profiler,
		})
	}
	if !ap.disableGoroutineProf {
		ap.registerBuiltIn(&goroutineMetric{
			app: ap.app, threshold: goroutineThreshold,
			rt: ap.runtimeQueryer, p: ap.profiler,
		})
	}
}

// registerBuiltIn installs a pre-defined Metric. It also records the
// name in the package-level reservedMetricNames so user Register calls
// can't collide — this keeps reservation logic in lockstep with the
// set of built-in metrics we actually expose.
func (ap *autoPprof) registerBuiltIn(m Metric) {
	reservedMetricNames.Store(m.Name(), struct{}{})

	ap.metricsMu.Lock()
	defer ap.metricsMu.Unlock()

	runner := newRunner(m, true, ap.watchInterval)
	ap.metrics[runner.name] = runner

	ap.wg.Add(1)
	go func() {
		defer ap.wg.Done()
		ap.watchMetric(runner)
	}()
}

// registerMetric handles user-initiated registration (both Option.Metrics
// and autopprof.Register). The stopC check under metricsMu ensures a
// watcher is never spawned after stop() has closed the channel, which
// would panic when wg.Add(1) races with stop()'s wg.Wait().
func (ap *autoPprof) registerMetric(m Metric) error {
	if err := validateMetric(m); err != nil {
		return err
	}
	name := m.Name()
	if _, reserved := reservedMetricNames.Load(name); reserved {
		return ErrReservedMetricName
	}

	ap.metricsMu.Lock()
	defer ap.metricsMu.Unlock()

	select {
	case <-ap.stopC:
		return ErrNotStarted
	default:
	}

	if _, exists := ap.metrics[name]; exists {
		return ErrMetricAlreadyRegistered
	}

	runner := newRunner(m, false, ap.watchInterval)
	ap.metrics[name] = runner

	ap.wg.Add(1)
	go func() {
		defer ap.wg.Done()
		ap.watchMetric(runner)
	}()
	return nil
}

// unregisterMetric deletes a user Metric from the map and signals its
// watcher to exit. The stopC close happens outside metricsMu so we
// don't hold the lock across the watcher's shutdown.
func (ap *autoPprof) unregisterMetric(name string) error {
	ap.metricsMu.Lock()
	r, ok := ap.metrics[name]
	if !ok {
		ap.metricsMu.Unlock()
		return ErrMetricNotRegistered
	}
	if r.builtIn {
		ap.metricsMu.Unlock()
		return ErrCannotUnregisterBuiltIn
	}
	delete(ap.metrics, name)
	ap.metricsMu.Unlock()

	close(r.stopC)
	return nil
}

// removeFromMapIfPresent is called by a watcher goroutine when it
// exits due to a Query error. We only drop *user* metrics so the
// caller can re-Register the same name; built-in entries stay so
// Start/Stop semantics remain well-defined.
func (ap *autoPprof) removeFromMapIfPresent(name string) {
	ap.metricsMu.Lock()
	defer ap.metricsMu.Unlock()
	if r, ok := ap.metrics[name]; ok && !r.builtIn {
		delete(ap.metrics, name)
	}
}

// newRunner caches the Metric's meta values at registration time so
// the watch loop can rely on stable name/threshold/interval even if a
// user's implementation mutates its return values later.
func newRunner(m Metric, builtIn bool, globalInterval time.Duration) *metricRunner {
	interval := m.Interval()
	if interval == 0 {
		interval = globalInterval
	}
	return &metricRunner{
		metric:    m,
		name:      m.Name(),
		threshold: m.Threshold(),
		interval:  interval,
		builtIn:   builtIn,
		stopC:     make(chan struct{}),
	}
}

// watchMetric is the unified watch loop that replaces v1's three
// type-specific watchers. The debounce mechanic
// (minConsecutiveOverThreshold) is identical to v1: fire on the first
// tick above threshold, suppress subsequent ticks until either the
// counter resets (drops below threshold) or reaches
// minConsecutiveOverThreshold at which point it wraps around to 0.
func (ap *autoPprof) watchMetric(runner *metricRunner) {
	ticker := time.NewTicker(runner.interval)
	defer ticker.Stop()

	var cnt int
	for {
		select {
		case <-ticker.C:
			value, err := runner.metric.Query()
			if err != nil {
				log.Println(fmt.Errorf(
					"autopprof: metric %q query failed: %w", runner.name, err,
				))
				// Let callers re-Register the same name after a transient
				// failure instead of leaving a "zombie" entry that
				// permanently shadows the name.
				ap.removeFromMapIfPresent(runner.name)
				return
			}
			if value < runner.threshold {
				cnt = 0
				continue
			}
			if cnt == 0 {
				if err := ap.fireReport(runner, value); err != nil {
					log.Println(fmt.Errorf(
						"autopprof: metric %q report failed: %w", runner.name, err,
					))
				}
				// Custom metrics don't cascade — only the three
				// built-in metrics participate in ReportAll.
				if runner.builtIn {
					ap.cascadeBuiltIn(runner.name)
				}
			}
			cnt++
			if cnt >= ap.minConsecutiveOverThreshold {
				cnt = 0
			}
		case <-runner.stopC:
			return
		case <-ap.stopC:
			return
		}
	}
}

// fireReport drives one cycle of Collect+Report. Concurrency-sensitive
// state (pprof CPU profiler, cgroup snapshot queue) is protected at
// its source inside profiler / cgroup queryer, so this layer just
// calls Collect and forwards the bytes to the Reporter.
func (ap *autoPprof) fireReport(runner *metricRunner, value float64) error {
	result, err := runner.metric.Collect(value)
	if err != nil {
		return fmt.Errorf("collect: %w", err)
	}
	if result.Reader == nil {
		// "Handled internally, skip the Reporter call" — useful for
		// side-effect-only hooks that already pushed data elsewhere.
		return nil
	}

	info := report.ReportInfo{
		MetricName: runner.name,
		Filename:   result.Filename,
		Comment:    result.Comment,
		Value:      value,
		Threshold:  runner.threshold,
	}
	if info.Filename == "" {
		info.Filename = defaultFilename(runner.name)
	}
	if info.Comment == "" {
		info.Comment = defaultComment(runner.name, value, runner.threshold)
	}

	ctx, cancel := context.WithTimeout(context.Background(), reportTimeout)
	defer cancel()
	return ap.reporter.Report(ctx, result.Reader, info)
}

// cascadeBuiltIn implements the ReportAll cascade: when any built-in
// metric breaches, report the other built-in metrics too. Only
// built-in metrics participate (custom metrics stay independent).
// Targets are snapshotted under metricsMu and all I/O happens outside
// the lock so a slow profileCPU / Slack upload cannot block
// Register / Unregister / Stop.
func (ap *autoPprof) cascadeBuiltIn(triggered string) {
	if !ap.reportAll {
		return
	}

	ap.metricsMu.Lock()
	targets := make([]*metricRunner, 0, len(ap.metrics))
	for name, r := range ap.metrics {
		if !r.builtIn || name == triggered {
			continue
		}
		targets = append(targets, r)
	}
	ap.metricsMu.Unlock()

	for _, r := range targets {
		value, err := r.metric.Query()
		if err != nil {
			log.Println(fmt.Errorf(
				"autopprof: cascade query %q: %w", r.name, err,
			))
			continue
		}
		if err := ap.fireReport(r, value); err != nil {
			log.Println(fmt.Errorf(
				"autopprof: cascade report %q: %w", r.name, err,
			))
		}
	}
}

// stop shuts down every watcher and blocks until they exit. Guarded by
// sync.Once so double-Stop is safe. wg.Wait runs last so Stop() doesn't
// return until every pprof.StartCPUProfile has unwound.
//
// close(ap.stopC) happens *under* metricsMu so registerMetric's
// `select <-ap.stopC` check is load-bearing: a Register that races
// with Stop either acquires the lock first and spawns its watcher
// (which Stop then waits for), or acquires the lock after Stop and
// observes the closed channel, returning ErrNotStarted without
// touching wg. wg.Add therefore never interleaves with wg.Wait.
func (ap *autoPprof) stop() {
	ap.stopOnce.Do(func() {
		ap.metricsMu.Lock()
		close(ap.stopC)
		for name, r := range ap.metrics {
			close(r.stopC)
			delete(ap.metrics, name)
		}
		ap.metricsMu.Unlock()

		ap.wg.Wait()
	})
}
