//go:build darwin

package status

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// toolDirs are where macOS keeps the command line tools used below. Absolute
// paths are tried first because a node started by launchd can have a bare
// PATH.
var toolDirs = []string{"/usr/sbin", "/usr/bin", "/sbin", "/bin"}

// uptimeSeconds is how long the machine has been up, worked out from the
// kernel's boot time.
//
// The boot time is read as text rather than through the syscall package
// because that package returns raw struct bytes with the trailing NUL
// stripped, which truncates a binary value.
func uptimeSeconds() (float64, error) {
	values, err := sysctlValues("kern.boottime")
	if err != nil {
		return 0, err
	}
	raw, ok := values["kern.boottime"]
	if !ok {
		return 0, fmt.Errorf("reading uptime: sysctl did not return kern.boottime")
	}

	bootTime, err := parseBoottime(raw)
	if err != nil {
		return 0, fmt.Errorf("reading boot time: %w", err)
	}

	seconds := time.Since(bootTime).Seconds()
	if seconds < 0 {
		// A clock that moved backwards is not worth a failed report.
		return 0, nil
	}
	return seconds, nil
}

// loadAverage is the current 1, 5 and 15 minute load.
func loadAverage() (Load, error) {
	values, err := sysctlValues("vm.loadavg")
	if err != nil {
		return Load{}, err
	}
	raw, ok := values["vm.loadavg"]
	if !ok {
		return Load{}, fmt.Errorf("reading load average: sysctl did not return vm.loadavg")
	}

	load, err := parseLoadavg(raw)
	if err != nil {
		return Load{}, fmt.Errorf("reading load average: %w", err)
	}
	return load, nil
}

// memoryUsage combines the installed memory with vm_stat's page counters.
func memoryUsage() (Usage, error) {
	values, err := sysctlValues("hw.memsize")
	if err != nil {
		return Usage{}, err
	}
	rawSize, ok := values["hw.memsize"]
	if !ok {
		return Usage{}, fmt.Errorf("reading memory: sysctl did not return hw.memsize")
	}

	total, err := parseMemSize(rawSize)
	if err != nil {
		return Usage{}, fmt.Errorf("reading memory: %w", err)
	}

	output, err := runTool("vm_stat")
	if err != nil {
		return Usage{}, fmt.Errorf("reading memory: %w", err)
	}

	usage, err := parseVMStat(output, total)
	if err != nil {
		return Usage{}, fmt.Errorf("reading memory: %w", err)
	}
	return usage, nil
}

// diskUsage measures the filesystem that holds path.
func diskUsage(path string) (Usage, error) {
	var stats syscall.Statfs_t
	if err := syscall.Statfs(path, &stats); err != nil {
		return Usage{}, fmt.Errorf("reading disk usage for %s: %w", path, err)
	}
	blockSize := uint64(stats.Bsize)
	total := stats.Blocks * blockSize
	// Space available to a user, which is what "free space" usually means.
	free := stats.Bavail * blockSize
	return Usage{Used: total - free, Total: total}, nil
}

// sysctlValues runs sysctl for the given names and returns one value per
// name. Several names are asked for in one call, so a report does not spawn a
// process per figure.
//
// Args:
//
//	names: sysctl names, such as "kern.boottime".
//
// Returns:
//
//	A value for each name, keyed by name.
//
// Raises:
//
//	An error if sysctl cannot be run, fails, or prints an unexpected number of
//	lines.
func sysctlValues(names ...string) (map[string]string, error) {
	output, err := runTool("sysctl", append([]string{"-n"}, names...)...)
	if err != nil {
		return nil, err
	}

	// -n prints just the values, one line each.
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) != len(names) {
		return nil, fmt.Errorf("sysctl %s: expected %d lines of output, got %d",
			strings.Join(names, " "), len(names), len(lines))
	}

	values := make(map[string]string, len(names))
	for i, name := range names {
		values[name] = strings.TrimSpace(lines[i])
	}
	return values, nil
}

// runTool runs one of the macOS command line tools and returns its output.
//
// Args:
//
//	name: the tool, such as "vm_stat".
//	args: arguments for it.
//
// Raises:
//
//	An error if the tool cannot be found or exits with a failure.
func runTool(name string, args ...string) (string, error) {
	path, err := findTool(name)
	if err != nil {
		return "", err
	}

	output, err := exec.Command(path, args...).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return "", fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return string(output), nil
}

// findTool looks for a tool in the usual macOS directories, falling back to
// PATH.
//
// Raises:
//
//	An error if the tool is in neither place.
func findTool(name string) (string, error) {
	for _, dir := range toolDirs {
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found in %s or on PATH", name, strings.Join(toolDirs, ", "))
	}
	return path, nil
}
