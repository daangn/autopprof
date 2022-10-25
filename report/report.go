package report

import (
	"context"
	"io"
)

//go:generate mockgen -source=report.go -destination=report_mock.go -package=report

const (
	// HeapProfileFilenameFmt is the filename format for the heap profile.
	// pprof.<app>.<hostname>.alloc_objects.alloc_space.inuse_objects.inuse_space.<formattedTime>.pprof
	HeapProfileFilenameFmt = "pprof.%s.%s.alloc_objects.alloc_space.inuse_objects.inuse_space.%s.pprof"
)

// Reporter is responsible for reporting the profiling report to the destination.
type Reporter interface {
	ReportHeapProfile(ctx context.Context, r io.Reader, mi MemInfo) error
}

// MemInfo is the memory information.
type MemInfo struct {
	ThresholdPercentage float64
	UsagePercentage     float64
}
