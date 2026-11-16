//go:build !linux && !darwin

package status

import (
	"fmt"
	"runtime"
)

// unsupported explains that this build has no way to read status figures.
func unsupported(what string) error {
	return fmt.Errorf("reading %s is not implemented on %s yet", what, runtime.GOOS)
}

// uptimeSeconds is unavailable on this platform.
func uptimeSeconds() (float64, error) {
	return 0, unsupported("uptime")
}

// loadAverage is unavailable on this platform.
func loadAverage() (Load, error) {
	return Load{}, unsupported("load average")
}

// memoryUsage is unavailable on this platform.
func memoryUsage() (Usage, error) {
	return Usage{}, unsupported("memory use")
}

// diskUsage is unavailable on this platform.
func diskUsage(string) (Usage, error) {
	return Usage{}, unsupported("disk use")
}
