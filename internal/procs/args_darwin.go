//go:build darwin

package procs

// psArgs asks for pid, user, cpu, memory and command with no header line.
// BSD ps spells the percentage columns with a percent sign rather than pcpu
// and pmem.
var psArgs = []string{"-eo", "pid=,user=,%cpu=,%mem=,comm="}
