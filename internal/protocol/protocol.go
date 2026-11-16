// Package protocol names the commands a master may send a node, and the
// header both sides use to prove they share the same secret.
//
// The master and the node both import this package so the names cannot drift
// apart.
package protocol

// HeaderSecret carries the shared secret on every request.
const HeaderSecret = "X-Fleet-Secret"

// Commands a master can ask a node to carry out.
const (
	// CommandReboot restarts the node.
	CommandReboot = "reboot"
	// CommandProcs asks the node to send back its list of running programs.
	CommandProcs = "procs"
)

// Known says whether name is a command this version understands. Unknown
// commands are ignored rather than run, so an old node can talk to a newer
// master.
func Known(name string) bool {
	switch name {
	case CommandReboot, CommandProcs:
		return true
	}
	return false
}
