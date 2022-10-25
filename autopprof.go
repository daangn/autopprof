//go:build linux
// +build linux

package autopprof

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/daangn/autopprof/report"
)

const (
	reportTimeout = 5 * time.Second
)

type autoPprof struct {
	queryer queryer

	// memThreshold is the memory usage threshold to trigger profile.
	// If the memory usage is over the threshold, the autopprof will
	//  report the heap profile.
	// Default: 0.75. (mean 75%)
	memThreshold float64

	// scanInterval is the interval to scan the resource usages.
	// Default: 5s.
	scanInterval time.Duration

	// minConsecutiveOverThreshold is the minimum consecutive
	// number of over a threshold for reporting profile again.
	// Default: 12.
	minConsecutiveOverThreshold int

	// reporter is the reporter to send the profiling report.
	reporter report.Reporter

	// stopC is the signal channel to stop the autopprof process.
	stopC chan struct{}
}

// globalAp is the global autopprof instance.
var globalAp *autoPprof

// Start configures and runs the autopprof process.
func Start(opt Option) error {
	qryer, err := newQueryer()
	if err != nil {
		return err
	}
	if err := opt.validate(); err != nil {
		return err
	}

	ap := &autoPprof{
		queryer:                     qryer,
		memThreshold:                defaultMemThreshold,
		scanInterval:                defaultScanInterval,
		minConsecutiveOverThreshold: defaultMinConsecutiveOverThreshold,
		reporter:                    opt.Reporter,
		stopC:                       make(chan struct{}),
	}
	if opt.MemThreshold != 0 {
		ap.memThreshold = opt.MemThreshold
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

func (ap *autoPprof) watch() {
	ticker := time.NewTicker(ap.scanInterval)
	defer ticker.Stop()

	go ap.watchMemUsage(ticker)
	// TODO(mingrammer): watch CPU usage too.

	<-ap.stopC
}

func (ap *autoPprof) watchMemUsage(ticker *time.Ticker) {
	var consecutiveOverThresholdCnt int
	for {
		select {
		case <-ticker.C:
			usage, err := ap.queryer.memUsage()
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
			// no duplicate reports are sent.
			// This is to prevent the autopprof from sending too many reports.
			if consecutiveOverThresholdCnt == 0 {
				if err := ap.reportHeapProfile(usage); err != nil {
					log.Println(fmt.Errorf(
						"autopprof: failed to report the heap profile: %w", err,
					))
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
	b, err := profileHeap()
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

func (ap *autoPprof) stop() {
	close(ap.stopC)
}
