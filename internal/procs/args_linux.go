//go:build linux

package procs

// psArgs asks for pid, user, cpu, memory and command with no header line.
var psArgs = []string{"-eo", "pid=,user=,pcpu=,pmem=,comm="}
