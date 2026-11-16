package status

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// pageSizePattern finds the page size in the first line of vm_stat output,
// which looks like "Mach Virtual Memory Statistics: (page size of 16384 bytes)".
var pageSizePattern = regexp.MustCompile(`page size of (\d+) bytes`)

// boottimePattern picks the seconds and microseconds out of a kern.boottime
// value, which looks like
// "{ sec = 1757700000, usec = 123456 } Sat Sep 13 20:13:20 2025".
var boottimePattern = regexp.MustCompile(`sec = (\d+), usec = (\d+)`)

// parseBoottime turns a kern.boottime value into the time the machine booted.
//
// Args:
//
//	raw: the value sysctl printed for kern.boottime.
//
// Raises:
//
//	An error if the value does not hold a seconds and microseconds pair.
func parseBoottime(raw string) (time.Time, error) {
	match := boottimePattern.FindStringSubmatch(raw)
	if match == nil {
		return time.Time{}, fmt.Errorf("boot time %q: expected "+
			"\"{ sec = <seconds>, usec = <microseconds> }\"", raw)
	}

	seconds, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("boot time %q: bad seconds: %w", raw, err)
	}
	microseconds, err := strconv.ParseInt(match[2], 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("boot time %q: bad microseconds: %w", raw, err)
	}
	return time.Unix(seconds, microseconds*1000), nil
}

// parseMemSize reads an hw.memsize value, which is a plain byte count.
//
// Args:
//
//	raw: the value sysctl printed for hw.memsize.
//
// Raises:
//
//	An error if the value is not a number.
func parseMemSize(raw string) (uint64, error) {
	value, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("memory size %q: %w", raw, err)
	}
	return value, nil
}

// parseLoadavg reads the load averages out of a vm.loadavg sysctl value, which
// looks like "{ 1.23 2.34 3.45 }".
//
// Args:
//
//	raw: the sysctl value.
//
// Returns:
//
//	The 1, 5 and 15 minute load averages.
//
// Raises:
//
//	An error if the value does not hold three numbers.
func parseLoadavg(raw string) (Load, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "{")
	trimmed = strings.TrimSuffix(trimmed, "}")

	fields := strings.Fields(trimmed)
	if len(fields) < 3 {
		return Load{}, fmt.Errorf("load average %q: expected three numbers", raw)
	}

	var values [3]float64
	for i := range values {
		parsed, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return Load{}, fmt.Errorf("load average %q: %w", raw, err)
		}
		values[i] = parsed
	}
	return Load{OneMinute: values[0], FiveMinutes: values[1], FifteenMinute: values[2]}, nil
}

// vmStatPages are the vm_stat counters that together mean "in use". Cached and
// free pages are deliberately left out, which matches how Activity Monitor
// shows Memory Used.
var vmStatPages = []string{"Pages active", "Pages wired down", "Pages occupied by compressor"}

// parseVMStat works out used memory from vm_stat output.
//
// Args:
//
//	output: the text vm_stat printed.
//	total: installed memory in bytes, from hw.memsize.
//
// Returns:
//
//	Used and total memory.
//
// Raises:
//
//	An error if the page size or any of the needed counters is missing.
func parseVMStat(output string, total uint64) (Usage, error) {
	match := pageSizePattern.FindStringSubmatch(output)
	if match == nil {
		return Usage{}, fmt.Errorf("vm_stat: no page size in output")
	}
	pageSize, err := strconv.ParseUint(match[1], 10, 64)
	if err != nil {
		return Usage{}, fmt.Errorf("vm_stat: bad page size: %w", err)
	}

	counters := map[string]uint64{}
	for _, line := range strings.Split(output, "\n") {
		// Each line looks like "Pages active:                          123456.".
		key, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		number := strings.TrimSuffix(strings.TrimSpace(rest), ".")
		pages, err := strconv.ParseUint(number, 10, 64)
		if err != nil {
			continue
		}
		counters[strings.TrimSpace(key)] = pages
	}

	var usedPages uint64
	for _, name := range vmStatPages {
		pages, ok := counters[name]
		if !ok {
			return Usage{}, fmt.Errorf("vm_stat: no %q counter", name)
		}
		usedPages += pages
	}

	used := usedPages * pageSize
	if used > total {
		used = total
	}
	return Usage{Used: used, Total: total}, nil
}
