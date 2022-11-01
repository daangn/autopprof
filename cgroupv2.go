//go:build linux
// +build linux

package autopprof

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
	cgroupV2CPUMaxFile = "cpu.max"

	cgroupV2CPUMaxDefaultPeriod = 100000
)

type cgroupV2 struct {
	groupPath  string
	mountPoint string
	cpuMaxFile string

	cpuQuota float64
}

func newCgroupsV2() *cgroupV2 {
	return &cgroupV2{
		groupPath:  "",
		mountPoint: cgroupV2MountPoint,
		cpuMaxFile: cgroupV2CPUMaxFile,
	}
}

func (c *cgroupV2) setCPUQuota() error {
	f, err := os.Open(
		path.Join(c.mountPoint, c.cpuMaxFile),
	)
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

func (c *cgroupV2) cpuUsage() (float64, error) {
	stat, err := c.stat()
	if err != nil {
		return 0, err
	}
	prev := stat.CPU.UsageUsec

	time.Sleep(cpuSnapshotDuration)

	stat, err = c.stat()
	if err != nil {
		return 0, err
	}
	curr := stat.CPU.UsageUsec

	delta := time.Duration(curr-prev) * time.Microsecond
	avg := float64(delta) / float64(cpuSnapshotDuration)
	return avg / c.cpuQuota, nil
}

func (c *cgroupV2) memUsage() (float64, error) {
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
