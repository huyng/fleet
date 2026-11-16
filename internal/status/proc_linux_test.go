//go:build linux

package status

import (
	"strings"
	"testing"
)

func TestProcUptime(t *testing.T) {
	got, err := procUptime(strings.NewReader("86400.52 172345.10\n"))
	if err != nil {
		t.Fatalf("procUptime: %v", err)
	}
	if got != 86400.52 {
		t.Errorf("uptime = %v, want 86400.52", got)
	}
}

func TestProcUptimeEmpty(t *testing.T) {
	if _, err := procUptime(strings.NewReader("")); err == nil {
		t.Error("expected an error for empty input")
	}
}

func TestProcLoad(t *testing.T) {
	got, err := procLoad(strings.NewReader("0.40 0.50 0.60 1/234 5678\n"))
	if err != nil {
		t.Fatalf("procLoad: %v", err)
	}
	want := Load{OneMinute: 0.40, FiveMinutes: 0.50, FifteenMinute: 0.60}
	if got != want {
		t.Errorf("load = %+v, want %+v", got, want)
	}
}

func TestProcLoadTooFewFields(t *testing.T) {
	if _, err := procLoad(strings.NewReader("0.40 0.50\n")); err == nil {
		t.Error("expected an error for two fields")
	}
}

// meminfo is a trimmed /proc/meminfo with the two lines that matter.
const meminfo = `MemTotal:       16316136 kB
MemFree:         1234567 kB
MemAvailable:    9876543 kB
Buffers:          123456 kB
`

func TestProcMemory(t *testing.T) {
	got, err := procMemory(strings.NewReader(meminfo))
	if err != nil {
		t.Fatalf("procMemory: %v", err)
	}
	want := Usage{Used: (16316136 - 9876543) * 1024, Total: 16316136 * 1024}
	if got != want {
		t.Errorf("memory = %+v, want %+v", got, want)
	}
}

func TestProcMemoryMissingAvailable(t *testing.T) {
	_, err := procMemory(strings.NewReader("MemTotal: 100 kB\n"))
	if err == nil {
		t.Error("expected an error when MemAvailable is missing")
	}
}
