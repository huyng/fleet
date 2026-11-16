package procs

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	out := `    1 root       0.0  0.1 systemd
  421 postgres   1.5  2.4 postgres
 1000 alice      0.0  0.0 tmux: server
`
	got, err := parse(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d processes, want 3", len(got))
	}

	// The busiest process comes first.
	want := Process{PID: 421, User: "postgres", CPUPercent: 1.5, MemPercent: 2.4, Command: "postgres"}
	if got[0] != want {
		t.Errorf("first process = %+v, want %+v", got[0], want)
	}
	// A command with spaces stays whole.
	if got[2].Command != "tmux: server" {
		t.Errorf("command = %q, want %q", got[2].Command, "tmux: server")
	}
}

func TestSortByBusy(t *testing.T) {
	processes := []Process{
		{PID: 3, CPUPercent: 0.0},
		{PID: 9, CPUPercent: 12.5},
		{PID: 1, CPUPercent: 0.0},
		{PID: 7, CPUPercent: 2.0},
	}

	sortByBusy(processes)

	wantOrder := []int{9, 7, 1, 3}
	for i, want := range wantOrder {
		if processes[i].PID != want {
			t.Errorf("position %d = pid %d, want %d", i, processes[i].PID, want)
		}
	}
}

func TestParseEmpty(t *testing.T) {
	got, err := parse("\n  \n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want no processes", got)
	}
}

func TestParseBadLine(t *testing.T) {
	if _, err := parse("1 root 0.0\n"); err == nil {
		t.Error("expected an error for a short line")
	}
	if _, err := parse("x root 0.0 0.0 cmd\n"); err == nil {
		t.Error("expected an error for a bad pid")
	}
	if _, err := parse("1 root x 0.0 cmd\n"); err == nil {
		t.Error("expected an error for a bad cpu figure")
	}
}

func TestListRunsPs(t *testing.T) {
	// The test machine must at least run itself.
	list, err := List()
	if err != nil {
		t.Skipf("ps not usable here: %v", err)
	}
	if len(list) == 0 {
		t.Error("expected at least one process")
	}
	for _, p := range list {
		if p.PID <= 0 {
			t.Errorf("suspicious pid %d in %+v", p.PID, p)
		}
		if strings.TrimSpace(p.Command) == "" {
			t.Errorf("empty command for pid %d", p.PID)
		}
	}
}
