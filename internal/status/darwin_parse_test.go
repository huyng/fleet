package status

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestParseBoottime(t *testing.T) {
	// Shape of the value sysctl prints for kern.boottime on macOS.
	raw := "{ sec = 1757700000, usec = 123456 } Sat Sep 13 20:13:20 2025"

	got, err := parseBoottime(raw)
	if err != nil {
		t.Fatalf("parseBoottime: %v", err)
	}
	want := time.Unix(1757700000, 123456*1000)
	if !got.Equal(want) {
		t.Errorf("boottime = %v, want %v", got, want)
	}
}

func TestParseBoottimeIsRecentEnoughForUptime(t *testing.T) {
	// A boot time a day ago should give an uptime of about 24 hours.
	raw := fmt.Sprintf("{ sec = %d, usec = 0 }", time.Now().Add(-24*time.Hour).Unix())

	got, err := parseBoottime(raw)
	if err != nil {
		t.Fatalf("parseBoottime: %v", err)
	}
	if hours := time.Since(got).Hours(); hours < 23.9 || hours > 24.1 {
		t.Errorf("time since boot = %v hours, want about 24", hours)
	}
}

func TestParseBoottimeBad(t *testing.T) {
	// Binary garbage is what the old code got and could not decode.
	cases := []string{"", "{}", "{ sec = 1 }", "garbage"}
	for _, raw := range cases {
		if _, err := parseBoottime(raw); err == nil {
			t.Errorf("%q: expected an error", raw)
		}
	}
}

func TestParseMemSize(t *testing.T) {
	got, err := parseMemSize("17179869184\n")
	if err != nil {
		t.Fatalf("parseMemSize: %v", err)
	}
	if got != 16<<30 {
		t.Errorf("memsize = %d, want %d", got, uint64(16<<30))
	}
}

func TestParseMemSizeBad(t *testing.T) {
	for _, raw := range []string{"", "lots"} {
		if _, err := parseMemSize(raw); err == nil {
			t.Errorf("%q: expected an error", raw)
		}
	}
}

func TestParseLoadavg(t *testing.T) {
	got, err := parseLoadavg("{ 1.23 2.34 3.45 }")
	if err != nil {
		t.Fatalf("parseLoadavg: %v", err)
	}
	want := Load{OneMinute: 1.23, FiveMinutes: 2.34, FifteenMinute: 3.45}
	if got != want {
		t.Errorf("load = %+v, want %+v", got, want)
	}
}

func TestParseLoadavgBad(t *testing.T) {
	cases := []string{"", "{ }", "{ 1.0 2.0 }", "{ a b c }"}
	for _, raw := range cases {
		if _, err := parseLoadavg(raw); err == nil {
			t.Errorf("%q: expected an error", raw)
		}
	}
}

// vmStat is trimmed output from a real macOS machine.
const vmStat = `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                              123456.
Pages active:                            234567.
Pages inactive:                          345678.
Pages speculative:                         4567.
Pages throttled:                              0.
Pages wired down:                         56789.
Pages purgeable:                           1234.
Pages occupied by compressor:             12345.
`

func TestParseVMStat(t *testing.T) {
	total := uint64(16) << 30

	got, err := parseVMStat(vmStat, total)
	if err != nil {
		t.Fatalf("parseVMStat: %v", err)
	}

	// active + wired + compressor, in 16 KiB pages.
	wantUsed := uint64(234567+56789+12345) * 16384
	if got.Used != wantUsed {
		t.Errorf("used = %d, want %d", got.Used, wantUsed)
	}
	if got.Total != total {
		t.Errorf("total = %d, want %d", got.Total, total)
	}
}

func TestParseVMStatMissingCounter(t *testing.T) {
	// No "Pages wired down" line.
	output := `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages active:  100.
`
	if _, err := parseVMStat(output, 1<<30); err == nil {
		t.Error("expected an error when a counter is missing")
	}
}

func TestParseVMStatNoPageSize(t *testing.T) {
	if _, err := parseVMStat("Pages active: 1.\n", 1<<30); err == nil {
		t.Error("expected an error when the page size is missing")
	}
}

func TestParseVMStatClampsToTotal(t *testing.T) {
	// More used than installed should not report more than 100%.
	got, err := parseVMStat(vmStat, 1)
	if err != nil {
		t.Fatalf("parseVMStat: %v", err)
	}
	if got.Used != 1 {
		t.Errorf("used = %d, want the total of 1", got.Used)
	}
	if got.Percent() > 100 {
		t.Errorf("percent = %v, want at most 100", got.Percent())
	}
}

func TestParseVMStatRealishOutput(t *testing.T) {
	// Trailing spaces and a missing final newline must not break parsing.
	output := strings.ReplaceAll(vmStat, ".\n", ".  \n") + "Pages active:  100."
	// The later "Pages active" line wins, which is what the parser does.
	got, err := parseVMStat(output, 16<<30)
	if err != nil {
		t.Fatalf("parseVMStat: %v", err)
	}
	wantUsed := uint64(100+56789+12345) * 16384
	if got.Used != wantUsed {
		t.Errorf("used = %d, want %d", got.Used, wantUsed)
	}
}
