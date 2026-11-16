package master

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/huyng/fleet/internal/procs"
	"github.com/huyng/fleet/internal/status"
)

// ErrUnknownNode is returned when a command names a node that has never
// reported. Nodes are only known once they check in.
var ErrUnknownNode = errors.New("unknown node")

// NodeStatus is what the master knows about one node.
type NodeStatus struct {
	// Report is the most recent status sample.
	Report status.Report `json:"report"`
	// LastSeen is when that sample arrived at the master.
	LastSeen time.Time `json:"last_seen"`
	// Stale is true when the node has been quiet for too long.
	Stale bool `json:"stale"`
	// Procs is the most recent running program list, if one was requested.
	Procs []procs.Process `json:"procs"`
	// ProcsAt is when that list arrived.
	ProcsAt time.Time `json:"procs_at"`
}

// Registry keeps the latest state of every node in memory. A master restart
// forgets everything until the nodes report again.
type Registry struct {
	mu    sync.Mutex
	nodes map[string]*record
}

// record is the mutable state behind one node.
type record struct {
	report   status.Report
	lastSeen time.Time
	pending  []string
	procs    []procs.Process
	procsAt  time.Time
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{nodes: make(map[string]*record)}
}

// Upsert stores a report, creating the node if this is its first check-in.
//
// Args:
//
//	rep: the status sample that arrived.
//	at: when it arrived, taken from the caller's clock.
//
// Returns:
//
//	True when this was the first report from that hostname.
func (r *Registry) Upsert(rep status.Report, at time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	node, known := r.nodes[rep.Hostname]
	if !known {
		node = &record{}
		r.nodes[rep.Hostname] = node
	}
	node.report = rep
	node.lastSeen = at
	return !known
}

// statusOf builds the outward view of one record.
//
// The process list is never nil, so a client can treat it as a list without
// special casing JSON null.
//
// Args:
//
//	node: the stored record.
//	now: the time stale nodes are measured against.
//	staleAfter: how long a node may be quiet before it is called stale.
func statusOf(node *record, now time.Time, staleAfter time.Duration) NodeStatus {
	list := node.procs
	if list == nil {
		list = []procs.Process{}
	}

	return NodeStatus{
		Report:   node.report,
		LastSeen: node.lastSeen,
		Stale:    now.Sub(node.lastSeen) > staleAfter,
		Procs:    list,
		ProcsAt:  node.procsAt,
	}
}

// Snapshot returns every known node, sorted by hostname.
//
// Args:
//
//	now: the time stale nodes are measured against.
//	staleAfter: how long a node may be quiet before it is called stale.
func (r *Registry) Snapshot(now time.Time, staleAfter time.Duration) []NodeStatus {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]NodeStatus, 0, len(r.nodes))
	for _, node := range r.nodes {
		out = append(out, statusOf(node, now, staleAfter))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Report.Hostname < out[j].Report.Hostname
	})
	return out
}

// Get returns one node's status.
//
// Args:
//
//	hostname: the node to look up.
//	now: the time stale nodes are measured against.
//	staleAfter: how long a node may be quiet before it is called stale.
//
// Raises:
//
//	ErrUnknownNode if the node has never reported.
func (r *Registry) Get(hostname string, now time.Time, staleAfter time.Duration) (NodeStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	node, ok := r.nodes[hostname]
	if !ok {
		return NodeStatus{}, ErrUnknownNode
	}
	return statusOf(node, now, staleAfter), nil
}

// Queue adds a command for a node to pick up on its next poll.
//
// Raises:
//
//	ErrUnknownNode if the node has never reported.
func (r *Registry) Queue(hostname, command string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	node, ok := r.nodes[hostname]
	if !ok {
		return ErrUnknownNode
	}
	node.pending = append(node.pending, command)
	return nil
}

// Take returns and clears the commands waiting for a node. A node that stops
// polling does not collect a backlog; the commands it left behind are dropped
// once taken by a later poll.
func (r *Registry) Take(hostname string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	node, ok := r.nodes[hostname]
	if !ok {
		return nil
	}
	pending := node.pending
	node.pending = nil
	return pending
}

// SetProcs stores a running program list returned by a node.
//
// Raises:
//
//	ErrUnknownNode if the node has never reported.
func (r *Registry) SetProcs(hostname string, list []procs.Process, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	node, ok := r.nodes[hostname]
	if !ok {
		return ErrUnknownNode
	}
	node.procs = list
	node.procsAt = at
	return nil
}
