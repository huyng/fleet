//go:build linux

package status

import (
	"fmt"
	"os"
	"syscall"
)

// The kernel files Linux exposes these figures through.
const (
	procUptimePath  = "/proc/uptime"
	procLoadavgPath = "/proc/loadavg"
	procMeminfoPath = "/proc/meminfo"
)

// uptimeSeconds is how long the machine has been up.
func uptimeSeconds() (float64, error) {
	file, err := os.Open(procUptimePath)
	if err != nil {
		return 0, fmt.Errorf("reading uptime: %w", err)
	}
	defer file.Close()
	return procUptime(file)
}

// loadAverage is the current 1, 5 and 15 minute load.
func loadAverage() (Load, error) {
	file, err := os.Open(procLoadavgPath)
	if err != nil {
		return Load{}, fmt.Errorf("reading load average: %w", err)
	}
	defer file.Close()
	return procLoad(file)
}

// memoryUsage is the current memory use from /proc/meminfo.
func memoryUsage() (Usage, error) {
	file, err := os.Open(procMeminfoPath)
	if err != nil {
		return Usage{}, fmt.Errorf("reading memory: %w", err)
	}
	defer file.Close()
	return procMemory(file)
}

// diskUsage measures the filesystem that holds path.
func diskUsage(path string) (Usage, error) {
	var stats syscall.Statfs_t
	if err := syscall.Statfs(path, &stats); err != nil {
		return Usage{}, fmt.Errorf("reading disk usage for %s: %w", path, err)
	}
	blockSize := uint64(stats.Bsize)
	total := stats.Blocks * blockSize
	// Space available to an unprivileged user, which is what a human expects
	// from "free space".
	free := stats.Bavail * blockSize
	return Usage{Used: total - free, Total: total}, nil
}
