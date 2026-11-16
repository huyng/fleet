package master

import (
	"testing"
	"time"

	"github.com/huyng/fleet/internal/procs"
	"github.com/huyng/fleet/internal/status"
)

// sampleReport is a report from a made up node.
func sampleReport(hostname string) status.Report {
	return status.Report{
		Hostname: hostname,
		IP:       "10.0.0.12",
		Memory:   status.Usage{Used: 1, Total: 2},
	}
}

func TestUpsertCreatesAndUpdates(t *testing.T) {
	registry := NewRegistry()
	now := time.Now()

	if first := registry.Upsert(sampleReport("web-1"), now); !first {
		t.Error("first report should be reported as new")
	}
	updated := sampleReport("web-1")
	updated.IP = "10.0.0.13"
	if first := registry.Upsert(updated, now.Add(time.Second)); first {
		t.Error("second report should not be reported as new")
	}

	nodes := registry.Snapshot(now.Add(time.Second), time.Minute)
	if len(nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(nodes))
	}
	if nodes[0].Report.IP != "10.0.0.13" {
		t.Errorf("ip = %q, want the newer one", nodes[0].Report.IP)
	}
}

func TestSnapshotSortsAndMarksStale(t *testing.T) {
	registry := NewRegistry()
	now := time.Now()
	registry.Upsert(sampleReport("zeta"), now.Add(-2*time.Minute))
	registry.Upsert(sampleReport("alpha"), now)

	nodes := registry.Snapshot(now, time.Minute)
	if len(nodes) != 2 {
		t.Fatalf("got %d nodes, want 2", len(nodes))
	}
	if nodes[0].Report.Hostname != "alpha" || nodes[1].Report.Hostname != "zeta" {
		t.Errorf("nodes not sorted by hostname: %v, %v",
			nodes[0].Report.Hostname, nodes[1].Report.Hostname)
	}
	if nodes[0].Stale {
		t.Error("fresh node marked stale")
	}
	if !nodes[1].Stale {
		t.Error("quiet node not marked stale")
	}
}

func TestGetUnknownNode(t *testing.T) {
	if _, err := NewRegistry().Get("nope", time.Now(), time.Minute); err != ErrUnknownNode {
		t.Errorf("err = %v, want ErrUnknownNode", err)
	}
}

func TestQueueAndTake(t *testing.T) {
	registry := NewRegistry()
	registry.Upsert(sampleReport("web-1"), time.Now())

	if err := registry.Queue("web-1", "reboot"); err != nil {
		t.Fatalf("Queue: %v", err)
	}
	got := registry.Take("web-1")
	if len(got) != 1 || got[0] != "reboot" {
		t.Fatalf("Take = %v, want [reboot]", got)
	}
	if again := registry.Take("web-1"); len(again) != 0 {
		t.Errorf("second Take = %v, want nothing", again)
	}
}

func TestQueueUnknownNode(t *testing.T) {
	if err := NewRegistry().Queue("nope", "reboot"); err != ErrUnknownNode {
		t.Errorf("err = %v, want ErrUnknownNode", err)
	}
}

func TestSetProcs(t *testing.T) {
	registry := NewRegistry()
	registry.Upsert(sampleReport("web-1"), time.Now())

	list := []procs.Process{{PID: 1, Command: "init"}}
	if err := registry.SetProcs("web-1", list, time.Now()); err != nil {
		t.Fatalf("SetProcs: %v", err)
	}

	node, err := registry.Get("web-1", time.Now(), time.Minute)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(node.Procs) != 1 || node.Procs[0].Command != "init" {
		t.Errorf("procs = %v", node.Procs)
	}
	if node.ProcsAt.IsZero() {
		t.Error("ProcsAt was not set")
	}
}

func TestSetProcsUnknownNode(t *testing.T) {
	if err := NewRegistry().SetProcs("nope", nil, time.Now()); err != ErrUnknownNode {
		t.Errorf("err = %v, want ErrUnknownNode", err)
	}
}

func TestGetAgreesWithSnapshotOnStaleness(t *testing.T) {
	registry := NewRegistry()
	reported := time.Now()
	registry.Upsert(sampleReport("web-1"), reported)

	// Well inside the window, then well outside it.
	fresh := reported.Add(time.Second)
	quiet := reported.Add(time.Hour)

	node, err := registry.Get("web-1", fresh, time.Minute)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if node.Stale {
		t.Error("Get called a fresh node stale")
	}

	node, err = registry.Get("web-1", quiet, time.Minute)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !node.Stale {
		t.Error("Get called a quiet node fresh")
	}

	nodes := registry.Snapshot(quiet, time.Minute)
	if len(nodes) != 1 || !nodes[0].Stale {
		t.Error("Snapshot and Get disagree about staleness")
	}
}

func TestStatusProcsAreNeverNil(t *testing.T) {
	// The page and the JSON clients both expect a list, so a node that has
	// not been asked for its programs must send [] rather than null.
	registry := NewRegistry()
	registry.Upsert(sampleReport("web-1"), time.Now())

	node, err := registry.Get("web-1", time.Now(), time.Minute)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if node.Procs == nil {
		t.Error("Get returned a nil process list")
	}

	nodes := registry.Snapshot(time.Now(), time.Minute)
	if nodes[0].Procs == nil {
		t.Error("Snapshot returned a nil process list")
	}
}
