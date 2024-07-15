//go:build linux
// +build linux

package queryer

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	cgroupsv2 "github.com/containerd/cgroups/v2"
	"github.com/containerd/cgroups/v2/stats"
)

const (
	cgroupV2MountPoint = "/sys/fs/cgroup"

	cgroupV2CPUMaxFile     = "cpu.max"
	cgroupV2CPUMaxQuotaMax = "max"

	cgroupV2CPUMaxDefaultPeriod = 100000

	cgroupV2UsageUnit = time.Microsecond
)

type cgroupV2 struct {
	groupPath  string
	mountPoint string
	cpuMaxFile string

	cpuQuota float64

	q cpuUsageSnapshotQueuer
}

func newCgroupsV2() *cgroupV2 {
	q := newCPUUsageSnapshotQueue(
		cpuUsageSnapshotQueueSize,
	)
	return &cgroupV2{
		groupPath:  "",
		mountPoint: cgroupV2MountPoint,
		cpuMaxFile: cgroupV2CPUMaxFile,
		q:          q,
	}
}

func (c *cgroupV2) CPUUsage() (float64, error) {
	stat, err := c.stat()
	if err != nil {
		return 0, err
	}
	c.snapshotCPUUsage(stat.CPU.UsageUsec) // In microseconds.

	// Calculate the usage only if there are enough snapshots.
	if !c.q.isFull() {
		return 0, nil
	}

	s1, s2 := c.q.head(), c.q.tail()
	delta := time.Duration(s2.usage-s1.usage) * cgroupV2UsageUnit
	duration := s2.timestamp.Sub(s1.timestamp)
	return (float64(delta) / float64(duration)) / c.cpuQuota, nil
}

func (c *cgroupV2) MemUsage() (float64, error) {
	stat, err := c.stat()
	if err != nil {
		return 0, err
	}
	var (
		sm    = stat.Memory
		usage = sm.Usage - sm.InactiveFile
		limit = sm.UsageLimit
	)
	return float64(usage) / float64(limit), nil
}

func (c *cgroupV2) SetCPUQuota() error {
	f, err := os.Open(
		path.Join(c.mountPoint, c.cpuMaxFile),
	)
	if os.IsNotExist(err) {
		return ErrV2CPUQuotaUndefined
	}
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 1 && len(fields) != 2 {
			return fmt.Errorf(
				"autopprof: invalid cpu.max format",
			)
		}
		if fields[0] == cgroupV2CPUMaxQuotaMax {
			return ErrV2CPUQuotaUndefined
		}

		max, err := strconv.Atoi(fields[0])
		if err != nil {
			return err
		}

		period := cgroupV2CPUMaxDefaultPeriod
		if len(fields) > 1 {
			period, err = strconv.Atoi(fields[1])
			if err != nil {
				return err
			}
		}
		c.cpuQuota = float64(max) / float64(period)
		return nil
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return ErrV2CPUMaxEmpty
}

func (c *cgroupV2) snapshotCPUUsage(usage uint64) {
	c.q.enqueue(&cpuUsageSnapshot{
		usage:     usage,
		timestamp: time.Now(),
	})
}

func (c *cgroupV2) stat() (*stats.Metrics, error) {
	path, err := cgroupsv2.NestedGroupPath(c.groupPath)
	if err != nil {
		return nil, err
	}
	m, err := cgroupsv2.LoadManager(c.mountPoint, path)
	if err != nil {
		return nil, err
	}
	stat, err := m.Stat()
	if err != nil {
		return nil, err
	}
	return stat, nil
}
