//go:build linux

package status

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// procUptime reads the uptime in seconds from the contents of /proc/uptime.
func procUptime(r io.Reader) (float64, error) {
	fields, err := firstLineFields(r)
	if err != nil {
		return 0, err
	}
	if len(fields) < 1 {
		return 0, fmt.Errorf("uptime: expected at least one field")
	}
	return strconv.ParseFloat(fields[0], 64)
}

// procLoad reads the 1, 5 and 15 minute load averages from /proc/loadavg.
func procLoad(r io.Reader) (Load, error) {
	fields, err := firstLineFields(r)
	if err != nil {
		return Load{}, err
	}
	if len(fields) < 3 {
		return Load{}, fmt.Errorf("loadavg: expected three fields, got %d", len(fields))
	}

	var values [3]float64
	for i := range values {
		values[i], err = strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return Load{}, fmt.Errorf("loadavg: %w", err)
		}
	}
	return Load{OneMinute: values[0], FiveMinutes: values[1], FifteenMinute: values[2]}, nil
}

// procMemory reads total and available memory in bytes from /proc/meminfo.
//
// Used is total minus available, which matches what tools like free report as
// in use, including reclaimable cache.
func procMemory(r io.Reader) (Usage, error) {
	values := map[string]uint64{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		// Each line looks like "MemTotal:       16316136 kB".
		key, rest, found := strings.Cut(scanner.Text(), ":")
		if !found {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 || (len(fields) > 1 && fields[1] != "kB") {
			continue
		}
		kb, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		values[key] = kb * 1024
	}
	if err := scanner.Err(); err != nil {
		return Usage{}, err
	}

	total, ok := values["MemTotal"]
	if !ok {
		return Usage{}, fmt.Errorf("meminfo: no MemTotal")
	}
	available, ok := values["MemAvailable"]
	if !ok {
		return Usage{}, fmt.Errorf("meminfo: no MemAvailable")
	}
	if available > total {
		available = total
	}
	return Usage{Used: total - available, Total: total}, nil
}

// firstLineFields splits the first line of r into whitespace separated fields.
func firstLineFields(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("empty input")
	}
	return strings.Fields(scanner.Text()), nil
}
