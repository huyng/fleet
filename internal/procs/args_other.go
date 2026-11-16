//go:build !linux && !darwin

package procs

// psArgs is unused on platforms List refuses to run on, but the variable keeps
// the package compiling so the rest of fleet still builds.
var psArgs = []string{"-eo", "pid=,user=,%cpu=,%mem=,comm="}
