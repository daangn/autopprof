package report

import (
	"context"
	"io"
)

//go:generate mockgen -source=report.go -destination=report_mock.go -package=report

const (
	// CPUProfileFilenameFmt is the filename format for the CPU profile.
	// pprof.<app>.<hostname>.samples.cpu.<report_time>.pprof.
	CPUProfileFilenameFmt = "pprof.%s.%s.samples.cpu.%s.pprof"

	// HeapProfileFilenameFmt is the filename format for the heap profile.
	// pprof.<app>.<hostname>.alloc_objects.alloc_space.inuse_objects.inuse_space.<report_time>.pprof.
	HeapProfileFilenameFmt = "pprof.%s.%s.alloc_objects.alloc_space.inuse_objects.inuse_space.%s.pprof"

	// GoroutineProfileFilenameFmt is the filename format for the goroutine profile.
	// pprof.<app>.<hostname>.goroutine.<report_time>.pprof.
	GoroutineProfileFilenameFmt = "pprof.%s.%s.goroutine.%s.pprof"
)

// Reporter is responsible for reporting the profiling report to the destination.
type Reporter interface {
	// ReportCPUProfile sends the CPU profiling data to the specific destination.
	ReportCPUProfile(ctx context.Context, r io.Reader, ci CPUInfo) error

	// ReportHeapProfile sends the heap profiling data to the specific destination.
	ReportHeapProfile(ctx context.Context, r io.Reader, mi MemInfo) error

	// ReportGoroutineProfile sends the goroutine profiling data to the specific destination.
	ReportGoroutineProfile(ctx context.Context, r io.Reader, gi GoroutineInfo) error
}

// CPUInfo is the CPU usage information.
type CPUInfo struct {
	ThresholdPercentage float64
	UsagePercentage     float64
}

// MemInfo is the memory usage information.
type MemInfo struct {
	ThresholdPercentage float64
	UsagePercentage     float64
}

type GoroutineInfo struct {
	ThresholdCount int
	Count          int
}
