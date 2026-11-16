package status

import (
	"net"
	"os"
	"time"
)

// Options are the settings Gather needs from the node configuration.
type Options struct {
	// Hostname overrides the detected hostname when not empty.
	Hostname string
	// ReportPath is the filesystem path whose disk usage is measured.
	ReportPath string
	// AgentVersion is recorded in the report so the master can show it.
	AgentVersion string
}

// Gather collects one status report from this machine.
//
// Args:
//
//	opts: overrides and the path used for the disk measurement.
//
// Returns:
//
//	A filled report with ReportedAt set to the current time.
//
// Raises:
//
//	An error if the hostname, load, memory or disk figures cannot be read.
func Gather(opts Options) (Report, error) {
	hostname, err := Hostname(opts.Hostname)
	if err != nil {
		return Report{}, err
	}

	uptime, err := uptimeSeconds()
	if err != nil {
		return Report{}, err
	}
	load, err := loadAverage()
	if err != nil {
		return Report{}, err
	}
	memory, err := memoryUsage()
	if err != nil {
		return Report{}, err
	}
	disk, err := diskUsage(opts.ReportPath)
	if err != nil {
		return Report{}, err
	}

	return Report{
		Hostname:      hostname,
		IP:            primaryIP(),
		UptimeSeconds: uptime,
		Load:          load,
		Memory:        memory,
		Disk:          disk,
		AgentVersion:  opts.AgentVersion,
		ReportedAt:    time.Now().UTC(),
	}, nil
}

// Hostname returns override when set, else the machine's own name.
//
// Args:
//
//	override: the name from the config, or empty to detect one.
//
// Raises:
//
//	An error if the override is empty and the machine has no name.
func Hostname(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return os.Hostname()
}

// The figures that differ per operating system live in system_<os>.go:
// uptimeSeconds, loadAverage, memoryUsage and diskUsage.

// primaryIP returns the first non loopback IPv4 address, or an empty string
// when the machine has none.
func primaryIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		network, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := network.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		return ip.String()
	}
	return ""
}
