//go:build linux
// +build linux

package autopprof

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/daangn/autopprof/v3/queryer"
	"github.com/daangn/autopprof/v3/report"
)

type autoPprof struct {
	watchInterval  time.Duration
	reportCooldown time.Duration

	reporter      report.Reporter
	reportTimeout time.Duration
	app           string

	disableCPUProf       bool
	disableMemProf       bool
	disableGoroutineProf bool

	cgroupQueryer  queryer.CgroupsQueryer
	runtimeQueryer queryer.RuntimeQueryer
	profiler       profiler

	// cascadedRunners holds only the built-in metrics so cascadeBuiltIn
	// can iterate them. Populated during Start and thereafter read-only —
	// no mutex needed.
	cascadedRunners map[string]*metricRunner

	// hasFlightRecorder is set when this instance started the
	// global flight recorder, so only that instance stops it.
	hasFlightRecorder bool

	// wg tracks every live watcher goroutine so Stop blocks until
	// in-flight pprof work (CPU profiling runs up to ~10s) unwinds.
	wg       sync.WaitGroup
	stopOnce sync.Once
	stopC    chan struct{}
}

type metricRunner struct {
	metric    Metric
	name      string
	threshold float64
	interval  time.Duration
}

// globalAp is the running instance, or nil before Start. Access is
// guarded by startOnce / stopOnce — Start and Stop each fire at most
// once per process.
var (
	globalAp  *autoPprof
	startOnce sync.Once
	startErr  error
	stopOnce  sync.Once
)

// Start configures and runs the autopprof process. It executes at
// most once per process — subsequent calls return the same error (or
// nil) as the first invocation. Safe to call concurrently; later
// callers block on the first one.
func Start(opt Option) error {
	startOnce.Do(func() {
		startErr = start(opt)
	})
	return startErr
}

func start(opt Option) error {
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

	app := opt.App
	if app == "" {
		app = defaultApp
	}
	reportTimeout := defaultReportTimeout
	if opt.ReportTimeout > 0 {
		reportTimeout = opt.ReportTimeout
	}
	watchInterval := defaultWatchInterval
	if opt.WatchInterval > 0 {
		watchInterval = opt.WatchInterval
	}
	reportCooldown := defaultReportCooldown
	if opt.ReportCooldown > 0 {
		reportCooldown = opt.ReportCooldown
	}
	profr := newDefaultProfiler(defaultCPUProfilingDuration)
	ap := &autoPprof{
		watchInterval:        watchInterval,
		reportCooldown:       reportCooldown,
		reporter:             opt.Reporter,
		reportTimeout:        reportTimeout,
		app:                  app,
		disableCPUProf:       opt.DisableCPUProf,
		disableMemProf:       opt.DisableMemProf,
		disableGoroutineProf: opt.DisableGoroutineProf,
		cgroupQueryer:        cgroupQryer,
		runtimeQueryer:       runtimeQryer,
		profiler:             profr,
		cascadedRunners:      make(map[string]*metricRunner),
		stopC:                make(chan struct{}),
	}
	if !ap.disableCPUProf {
		if err := ap.loadCPUQuota(); err != nil {
			return err
		}
	}
	// Start the recorder before any watcher runs, so the first Collect
	// can already snapshot it. Nothing to unwind on failure yet.
	if err := startFlightRecorder(opt.FlightRecorder); err != nil {
		return err
	}
	ap.hasFlightRecorder = opt.FlightRecorder != nil
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

// Stop stops the global autopprof process. It executes at most once
// per process; subsequent calls are no-ops. Safe to call concurrently.
func Stop() {
	stopOnce.Do(func() {
		if globalAp == nil {
			return
		}
		globalAp.stop()
		globalAp = nil
	})
}

// Register adds a user Metric to the running autopprof instance. The
// metric's watcher runs until Stop.
func Register(m Metric) error {
	if globalAp == nil {
		return ErrNotStarted
	}
	return globalAp.registerMetric(m)
}

// loadCPUQuota resolves the container CPU limit. If the cgroup quota
// isn't set we log and silently disable CPU profiling (matching v1).
func (ap *autoPprof) loadCPUQuota() error {
	err := ap.cgroupQueryer.SetCPUQuota()
	if err == nil {
		return nil
	}
	if ap.disableMemProf {
		return err
	}
	log.Println(
		"autopprof: disable the cpu profiling due to the CPU quota isn't set",
	)
	ap.disableCPUProf = true
	return nil
}

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

func (ap *autoPprof) registerBuiltIn(m Metric) {
	runner := newRunner(m, ap.watchInterval)
	ap.cascadedRunners[runner.name] = runner
	ap.wg.Add(1)
	go func() {
		defer ap.wg.Done()
		ap.watchMetric(runner, true)
	}()
}

func (ap *autoPprof) registerMetric(m Metric) error {
	if err := validateMetric(m); err != nil {
		return err
	}
	select {
	case <-ap.stopC:
		return ErrNotStarted
	default:
	}
	runner := newRunner(m, ap.watchInterval)
	ap.wg.Add(1)
	go func() {
		defer ap.wg.Done()
		ap.watchMetric(runner, false)
	}()
	return nil
}

// newRunner caches Metric's meta values so the watch loop uses a
// stable name/threshold/interval even if the implementation mutates
// them later.
func newRunner(m Metric, globalInterval time.Duration) *metricRunner {
	interval := m.Interval()
	if interval == 0 {
		interval = globalInterval
	}
	return &metricRunner{
		metric:    m,
		name:      m.Name(),
		threshold: m.Threshold(),
		interval:  interval,
	}
}

// watchMetric runs the unified watch loop. reportCooldown debounces
// repeat fires: report on the first tick above threshold, then suppress
// further reports for that metric until the cooldown has elapsed.
// Dropping below the threshold re-arms an immediate report on the next
// breach.
//
// An EventMetric additionally reports whatever arrives on Events(),
// debounced on its own clock so the ticker's re-arm can't defeat it: a
// bulkhead's live ratio drops right after it trips, which would
// otherwise re-arm the event path on the very next tick.
func (ap *autoPprof) watchMetric(runner *metricRunner, isBuiltin bool) {
	ticker := time.NewTicker(runner.interval)
	defer ticker.Stop()

	// A nil channel never becomes ready, so plain Metrics skip the
	// events case entirely.
	var events <-chan float64
	if em, ok := runner.metric.(EventMetric); ok {
		events = em.Events()
	}

	var lastReport, lastEventReport time.Time
	for {
		select {
		case <-ticker.C:
			value, err := runner.metric.Query()
			if err != nil {
				log.Println(fmt.Errorf(
					"autopprof: metric %q query failed: %w", runner.name, err,
				))
				return
			}
			if value < runner.threshold {
				lastReport = time.Time{}
				continue
			}
			now := time.Now()
			if lastReport.IsZero() || now.Sub(lastReport) >= ap.reportCooldown {
				if err := ap.fireReport(runner, value); err != nil {
					log.Println(fmt.Errorf(
						"autopprof: metric %q report failed: %w", runner.name, err,
					))
				}
				if isBuiltin {
					ap.cascadeBuiltIn(runner.name)
				}
				lastReport = now
			}
		case v, ok := <-events:
			if !ok {
				// Closed: a closed channel is always ready, so stop
				// selecting on it and fall back to polling only.
				events = nil
				continue
			}
			if !lastEventReport.IsZero() && time.Since(lastEventReport) < ap.reportCooldown {
				continue
			}
			if err := ap.fireReport(runner, v); err != nil {
				log.Println(fmt.Errorf(
					"autopprof: metric %q event report failed: %w", runner.name, err,
				))
			}
			// Measured from when Collect returns, not when it started:
			// Collect can outlast the cooldown, and the event it buffered
			// meanwhile belongs to the episode just reported.
			lastEventReport = time.Now()
		case <-ap.stopC:
			return
		}
	}
}

func (ap *autoPprof) fireReport(runner *metricRunner, value float64) error {
	result, err := runner.metric.Collect(value)
	if err != nil {
		return fmt.Errorf("collect: %w", err)
	}
	if result.Reader == nil {
		// Side-effect-only hook; nothing to ship.
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

	ctx, cancel := context.WithTimeout(context.Background(), ap.reportTimeout)
	defer cancel()
	return ap.reporter.Report(ctx, result.Reader, info)
}

// cascadeBuiltIn reports the other enabled built-in metrics whenever
// any built-in breaches. Custom metrics stay independent.
// cascadedRunners is read-only after Start, so no lock.
func (ap *autoPprof) cascadeBuiltIn(triggered string) {
	for name, r := range ap.cascadedRunners {
		if name == triggered {
			continue
		}
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

// stop signals every watcher and blocks until they exit. wg.Wait
// ensures Stop() doesn't return while pprof.StartCPUProfile is in
// flight, and that no Collect is still snapshotting the flight
// recorder when it is stopped.
func (ap *autoPprof) stop() {
	ap.stopOnce.Do(func() {
		close(ap.stopC)
		ap.wg.Wait()
		if ap.hasFlightRecorder {
			stopFlightRecorder()
		}
	})
}
