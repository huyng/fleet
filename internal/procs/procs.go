// Package procs lists the programs running on a machine.
package procs

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// psArgs asks for the columns below without a header line. comm is the
// executable name, which keeps the output easy to split; args would contain
// spaces. The exact keywords differ per operating system, so psArgs comes
// from args_<os>.go.

// Process is one running program.
type Process struct {
	PID        int     `json:"pid"`
	User       string  `json:"user"`
	CPUPercent float64 `json:"cpu_percent"`
	MemPercent float64 `json:"mem_percent"`
	Command    string  `json:"command"`
}

// List returns the running programs on this machine, busiest CPU first.
//
// Returns:
//
//	One entry per process, empty when nothing is running.
//
// Raises:
//
//	An error if ps cannot be run or its output cannot be parsed.
func List() ([]Process, error) {
	var stdout bytes.Buffer
	cmd := exec.Command("ps", psArgs...)
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("running ps: %w", err)
	}
	return parse(stdout.String())
}

// sortByBusy puts the highest CPU users first, and breaks ties by pid so the
// order does not shuffle between collections.
//
// Args:
//
//	processes: the list to sort in place.
func sortByBusy(processes []Process) {
	sort.Slice(processes, func(i, j int) bool {
		if processes[i].CPUPercent != processes[j].CPUPercent {
			return processes[i].CPUPercent > processes[j].CPUPercent
		}
		return processes[i].PID < processes[j].PID
	})
}

// parse turns ps output into processes.
//
// Raises:
//
//	An error if a line does not have the five expected columns or a number
//	cannot be read.
func parse(out string) ([]Process, error) {
	processes := []Process{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Split in five so a command containing spaces stays whole.
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return nil, fmt.Errorf("ps line %q: expected five columns", line)
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, fmt.Errorf("ps line %q: bad pid: %w", line, err)
		}
		cpu, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			return nil, fmt.Errorf("ps line %q: bad cpu: %w", line, err)
		}
		mem, err := strconv.ParseFloat(fields[3], 64)
		if err != nil {
			return nil, fmt.Errorf("ps line %q: bad mem: %w", line, err)
		}

		processes = append(processes, Process{
			PID:        pid,
			User:       fields[1],
			CPUPercent: cpu,
			MemPercent: mem,
			Command:    strings.Join(fields[4:], " "),
		})
	}

	sortByBusy(processes)
	return processes, nil
}
