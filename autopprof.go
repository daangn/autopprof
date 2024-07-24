//go:build linux
// +build linux

package autopprof

import (
	"bytes"
	"context"
	"fmt"
	"github.com/daangn/autopprof/queryer"
	"log"
	"time"

	"github.com/daangn/autopprof/report"
)

const (
	reportTimeout = 5 * time.Second
)

type autoPprof struct {
	// watchInterval is the interval to watch the resource usages.
	// Default: 5s.
	watchInterval time.Duration

	// cpuThreshold is the cpu usage threshold to trigger profile.
	// If the cpu usage is over the threshold, the autopprof will
	//  report the cpu profile.
	// Default: 0.75. (mean 75%)
	cpuThreshold float64

	// memThreshold is the memory usage threshold to trigger profile.
	// If the memory usage is over the threshold, the autopprof will
	//  report the heap profile.
	// Default: 0.75. (mean 75%)
	memThreshold float64

	// goroutineThreshold is the goroutine count threshold to trigger profile.
	// If the goroutine count is over the threshold, the autopprof will
	//  report the goroutine profile.
	// Default: 50000.
	goroutineThreshold int

	// minConsecutiveOverThreshold is the minimum consecutive
	// number of over a threshold for reporting profile again.
	// Default: 12.
	minConsecutiveOverThreshold int

	// cgroupQueryer is used to query the quota and the cgroup stat.
	cgroupQueryer queryer.CgroupsQueryer

	// runtimeQueryer is used to query the runtime stat.
	runtimeQueryer queryer.RuntimeQueryer

	// profiler is used to profile the cpu and the heap memory.
	profiler profiler

	// reporter is the reporter to send the profiling reports.
	reporter report.Reporter

	// reportBoth sets whether to trigger reports for both CPU and memory when either threshold is exceeded.
	// If some profiling is disabled, exclude it.
	reportBoth bool

	// Flags to disable the profiling.
	disableCPUProf       bool
	disableMemProf       bool
	disableGoroutineProf bool

	// stopC is the signal channel to stop the watch processes.
	stopC chan struct{}
}

// globalAp is the global autopprof instance.
var globalAp *autoPprof

// Start configures and runs the autopprof process.
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
		cpuThreshold:                defaultCPUThreshold,
		memThreshold:                defaultMemThreshold,
		goroutineThreshold:          defaultGoroutineThreshold,
		minConsecutiveOverThreshold: defaultMinConsecutiveOverThreshold,
		cgroupQueryer:               cgroupQryer,
		runtimeQueryer:              runtimeQryer,
		profiler:                    profr,
		reporter:                    opt.Reporter,
		reportBoth:                  opt.ReportBoth,
		disableCPUProf:              opt.DisableCPUProf,
		disableMemProf:              opt.DisableMemProf,
		stopC:                       make(chan struct{}),
	}
	if opt.CPUThreshold != 0 {
		ap.cpuThreshold = opt.CPUThreshold
	}
	if opt.MemThreshold != 0 {
		ap.memThreshold = opt.MemThreshold
	}
	if opt.GoroutineThreshold != 0 {
		ap.goroutineThreshold = opt.GoroutineThreshold
	}
	if !ap.disableCPUProf {
		if err := ap.loadCPUQuota(); err != nil {
			return err
		}
	}

	go ap.watch()
	globalAp = ap
	return nil
}

// Stop stops the global autopprof process.
func Stop() {
	if globalAp != nil {
		globalAp.stop()
	}
}

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

func (ap *autoPprof) watch() {
	go ap.watchCPUUsage()
	go ap.watchMemUsage()
	go ap.watchGoroutineCount()
	<-ap.stopC
}

func (ap *autoPprof) watchCPUUsage() {
	if ap.disableCPUProf {
		return
	}

	ticker := time.NewTicker(ap.watchInterval)
	defer ticker.Stop()

	var consecutiveOverThresholdCnt int
	for {
		select {
		case <-ticker.C:
			usage, err := ap.cgroupQueryer.CPUUsage()
			if err != nil {
				log.Println(err)
				return
			}
			if usage < ap.cpuThreshold {
				// Reset the count if the cpu usage goes under the threshold.
				consecutiveOverThresholdCnt = 0
				continue
			}

			// If cpu utilization remains high for a short period of time, no
			//  duplicate reports are sent.
			// This is to prevent the autopprof from sending too many reports.
			if consecutiveOverThresholdCnt == 0 {
				if err := ap.reportCPUProfile(usage); err != nil {
					log.Println(fmt.Errorf(
						"autopprof: failed to report the cpu profile: %w", err,
					))
				}
				if ap.reportBoth && !ap.disableMemProf {
					memUsage, err := ap.cgroupQueryer.MemUsage()
					if err != nil {
						log.Println(err)
						return
					}
					if err := ap.reportHeapProfile(memUsage); err != nil {
						log.Println(fmt.Errorf(
							"autopprof: failed to report the heap profile: %w", err,
						))
					}
				}
			}

			consecutiveOverThresholdCnt++
			if consecutiveOverThresholdCnt >= ap.minConsecutiveOverThreshold {
				// Reset the count and ready to report the cpu profile again.
				consecutiveOverThresholdCnt = 0
			}
		case <-ap.stopC:
			return
		}
	}
}

func (ap *autoPprof) reportCPUProfile(cpuUsage float64) error {
	b, err := ap.profiler.profileCPU()
	if err != nil {
		return fmt.Errorf("autopprof: failed to profile the cpu: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), reportTimeout)
	defer cancel()

	ci := report.CPUInfo{
		ThresholdPercentage: ap.cpuThreshold * 100,
		UsagePercentage:     cpuUsage * 100,
	}
	bReader := bytes.NewReader(b)
	if err := ap.reporter.ReportCPUProfile(ctx, bReader, ci); err != nil {
		return err
	}
	return nil
}

func (ap *autoPprof) watchMemUsage() {
	if ap.disableMemProf {
		return
	}

	ticker := time.NewTicker(ap.watchInterval)
	defer ticker.Stop()

	var consecutiveOverThresholdCnt int
	for {
		select {
		case <-ticker.C:
			usage, err := ap.cgroupQueryer.MemUsage()
			if err != nil {
				log.Println(err)
				return
			}
			if usage < ap.memThreshold {
				// Reset the count if the memory usage goes under the threshold.
				consecutiveOverThresholdCnt = 0
				continue
			}

			// If memory utilization remains high for a short period of time,
			//  no duplicate reports are sent.
			// This is to prevent the autopprof from sending too many reports.
			if consecutiveOverThresholdCnt == 0 {
				if err := ap.reportHeapProfile(usage); err != nil {
					log.Println(fmt.Errorf(
						"autopprof: failed to report the heap profile: %w", err,
					))
				}
				if ap.reportBoth && !ap.disableCPUProf {
					cpuUsage, err := ap.cgroupQueryer.CPUUsage()
					if err != nil {
						log.Println(err)
						return
					}
					if err := ap.reportCPUProfile(cpuUsage); err != nil {
						log.Println(fmt.Errorf(
							"autopprof: failed to report the cpu profile: %w", err,
						))
					}
				}
			}

			consecutiveOverThresholdCnt++
			if consecutiveOverThresholdCnt >= ap.minConsecutiveOverThreshold {
				// Reset the count and ready to report the heap profile again.
				consecutiveOverThresholdCnt = 0
			}
		case <-ap.stopC:
			return
		}
	}
}

func (ap *autoPprof) reportHeapProfile(memUsage float64) error {
	b, err := ap.profiler.profileHeap()
	if err != nil {
		return fmt.Errorf("autopprof: failed to profile the heap: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), reportTimeout)
	defer cancel()

	mi := report.MemInfo{
		ThresholdPercentage: ap.memThreshold * 100,
		UsagePercentage:     memUsage * 100,
	}
	bReader := bytes.NewReader(b)
	if err := ap.reporter.ReportHeapProfile(ctx, bReader, mi); err != nil {
		return err
	}
	return nil
}

func (ap *autoPprof) watchGoroutineCount() {
	if ap.disableGoroutineProf {
		return
	}

	ticker := time.NewTicker(ap.watchInterval)
	defer ticker.Stop()

	var consecutiveOverThresholdCnt int
	for {
		select {
		case <-ticker.C:
			count := ap.runtimeQueryer.GoroutineCount()

			if count < ap.goroutineThreshold {
				// Reset the count if the goroutine count goes under the threshold.
				consecutiveOverThresholdCnt = 0
				continue
			}

			// If goroutine count remains high for a short period of time, no
			//  duplicate reports are sent.
			// This is to prevent the autopprof from sending too many reports.
			if consecutiveOverThresholdCnt == 0 {
				if err := ap.reportGoroutineProfile(count); err != nil {
					log.Println(fmt.Errorf(
						"autopprof: failed to report the goroutine profile: %w", err,
					))
				}
			}

			consecutiveOverThresholdCnt++
			if consecutiveOverThresholdCnt >= ap.minConsecutiveOverThreshold {
				// Reset the count and ready to report the goroutine profile again.
				consecutiveOverThresholdCnt = 0
			}
		case <-ap.stopC:
			return
		}
	}
}

func (ap *autoPprof) reportGoroutineProfile(goroutineCount int) error {
	b, err := ap.profiler.profileGoroutine()
	if err != nil {
		return fmt.Errorf("autopprof: failed to profile the goroutine: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), reportTimeout)
	defer cancel()

	gi := report.GoroutineInfo{
		ThresholdCount: ap.goroutineThreshold,
		Count:          goroutineCount,
	}
	bReader := bytes.NewReader(b)
	if err := ap.reporter.ReportGoroutineProfile(ctx, bReader, gi); err != nil {
		return err
	}
	return nil
}

func (ap *autoPprof) stop() {
	close(ap.stopC)
}
