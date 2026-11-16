package master

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/huyng/fleet/internal/config"
	"github.com/huyng/fleet/internal/procs"
	"github.com/huyng/fleet/internal/protocol"
	"github.com/huyng/fleet/internal/status"
)

const testSecret = "s3cret"

// newTestServer starts a master on a random port and returns it with the
// server value so tests can move its clock.
func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()

	cfg := config.Master{Secret: testSecret, ReportInterval: config.Duration{Duration: 15 * time.Second}}
	server := NewServer(cfg, log.New(io.Discard, "", 0))
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	return server, httpServer
}

// do sends a request to the test master and returns the response.
//
// Args:
//
//	t: the test, used for failure reporting.
//	httpServer: the running test master.
//	method: HTTP method.
//	path: path to request.
//	body: request body, or empty.
//	secret: secret header value, or empty to send none.
func do(t *testing.T, httpServer *httptest.Server, method, path, body, secret string) *http.Response {
	t.Helper()

	request, err := http.NewRequest(method, httpServer.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	if secret != "" {
		request.Header.Set(protocol.HeaderSecret, secret)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	t.Cleanup(func() { response.Body.Close() })
	return response
}

func TestReportNeedsSecret(t *testing.T) {
	_, httpServer := newTestServer(t)

	response := do(t, httpServer, http.MethodPost, "/api/v1/report", `{"hostname":"web-1"}`, "")
	if response.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", response.StatusCode)
	}
	response = do(t, httpServer, http.MethodPost, "/api/v1/report", `{"hostname":"web-1"}`, "wrong")
	if response.StatusCode != http.StatusUnauthorized {
		t.Errorf("status with wrong secret = %d, want 401", response.StatusCode)
	}
}

func TestReportThenList(t *testing.T) {
	_, httpServer := newTestServer(t)

	body := `{"hostname":"web-1","ip":"10.0.0.12","uptime_seconds":86400,"load":{"1m":0.4,"5m":0.5,"15m":0.6},"memory":{"used":10,"total":20},"disk":{"used":5,"total":10},"agent_version":"0.1.0"}`
	response := do(t, httpServer, http.MethodPost, "/api/v1/report", body, testSecret)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.StatusCode)
	}

	response = do(t, httpServer, http.MethodGet, "/api/v1/nodes", "", testSecret)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	var nodes []NodeStatus
	if err := json.NewDecoder(response.Body).Decode(&nodes); err != nil {
		t.Fatalf("decoding nodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Report.Hostname != "web-1" {
		t.Fatalf("nodes = %+v", nodes)
	}
	if nodes[0].Report.Load.OneMinute != 0.4 {
		t.Errorf("load = %+v", nodes[0].Report.Load)
	}
	if nodes[0].Stale {
		t.Error("fresh node marked stale")
	}
}

func TestReportWithoutHostname(t *testing.T) {
	_, httpServer := newTestServer(t)

	response := do(t, httpServer, http.MethodPost, "/api/v1/report", `{"ip":"10.0.0.12"}`, testSecret)
	if response.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", response.StatusCode)
	}
}

func TestReportBadJSON(t *testing.T) {
	_, httpServer := newTestServer(t)

	response := do(t, httpServer, http.MethodPost, "/api/v1/report", `{`, testSecret)
	if response.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", response.StatusCode)
	}
}

func TestStaleAfterQuietPeriod(t *testing.T) {
	server, httpServer := newTestServer(t)
	reported := time.Now()
	server.now = func() time.Time { return reported }
	do(t, httpServer, http.MethodPost, "/api/v1/report", `{"hostname":"web-1"}`, testSecret)

	// Move the clock past three report intervals.
	server.now = func() time.Time { return reported.Add(time.Minute) }

	response := do(t, httpServer, http.MethodGet, "/api/v1/nodes", "", testSecret)
	var nodes []NodeStatus
	if err := json.NewDecoder(response.Body).Decode(&nodes); err != nil {
		t.Fatalf("decoding nodes: %v", err)
	}
	if len(nodes) != 1 || !nodes[0].Stale {
		t.Errorf("nodes = %+v, want one stale node", nodes)
	}
}

func TestRebootQueueAndPoll(t *testing.T) {
	_, httpServer := newTestServer(t)
	do(t, httpServer, http.MethodPost, "/api/v1/report", `{"hostname":"web-1"}`, testSecret)

	response := do(t, httpServer, http.MethodPost, "/api/v1/nodes/web-1/reboot", "", testSecret)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("reboot status = %d, want 202", response.StatusCode)
	}

	response = do(t, httpServer, http.MethodGet, "/api/v1/commands?hostname=web-1", "", testSecret)
	var reply struct {
		Commands []string `json:"commands"`
	}
	if err := json.NewDecoder(response.Body).Decode(&reply); err != nil {
		t.Fatalf("decoding commands: %v", err)
	}
	if len(reply.Commands) != 1 || reply.Commands[0] != protocol.CommandReboot {
		t.Fatalf("commands = %v, want [reboot]", reply.Commands)
	}

	// The queue is cleared once taken.
	response = do(t, httpServer, http.MethodGet, "/api/v1/commands?hostname=web-1", "", testSecret)
	if err := json.NewDecoder(response.Body).Decode(&reply); err != nil {
		t.Fatalf("decoding commands: %v", err)
	}
	if len(reply.Commands) != 0 {
		t.Errorf("commands = %v, want none", reply.Commands)
	}
}

func TestRebootUnknownNode(t *testing.T) {
	_, httpServer := newTestServer(t)

	response := do(t, httpServer, http.MethodPost, "/api/v1/nodes/nope/reboot", "", testSecret)
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", response.StatusCode)
	}
}

func TestCommandsNeedsHostname(t *testing.T) {
	_, httpServer := newTestServer(t)

	response := do(t, httpServer, http.MethodGet, "/api/v1/commands", "", testSecret)
	if response.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", response.StatusCode)
	}
}

func TestCommandsForUnknownNodeIsEmpty(t *testing.T) {
	_, httpServer := newTestServer(t)

	// A node sending its first report that fails still polls for commands. It
	// must get an empty list, not an error, or the real problem is hidden
	// behind a confusing 404.
	response := do(t, httpServer, http.MethodGet, "/api/v1/commands?hostname=never-seen", "", testSecret)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}

	var reply struct {
		Commands []string `json:"commands"`
	}
	if err := json.NewDecoder(response.Body).Decode(&reply); err != nil {
		t.Fatalf("decoding commands: %v", err)
	}
	if len(reply.Commands) != 0 {
		t.Errorf("commands = %v, want none", reply.Commands)
	}
}

func TestPutAndGetProcs(t *testing.T) {
	_, httpServer := newTestServer(t)
	do(t, httpServer, http.MethodPost, "/api/v1/report", `{"hostname":"web-1"}`, testSecret)

	response := do(t, httpServer, http.MethodGet, "/api/v1/nodes/web-1/procs", "", testSecret)
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("status before upload = %d, want 404", response.StatusCode)
	}

	list := `[{"pid":1,"user":"root","cpu_percent":0.5,"mem_percent":1.0,"command":"systemd"}]`
	response = do(t, httpServer, http.MethodPut, "/api/v1/nodes/web-1/procs", list, testSecret)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("upload status = %d, want 204", response.StatusCode)
	}

	response = do(t, httpServer, http.MethodGet, "/api/v1/nodes/web-1/procs", "", testSecret)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	var reply struct {
		Procs     []procs.Process `json:"procs"`
		Collected time.Time       `json:"collected_at"`
	}
	if err := json.NewDecoder(response.Body).Decode(&reply); err != nil {
		t.Fatalf("decoding procs: %v", err)
	}
	if len(reply.Procs) != 1 || reply.Procs[0].Command != "systemd" {
		t.Errorf("procs = %+v", reply.Procs)
	}
	if reply.Collected.IsZero() {
		t.Error("collected_at was not set")
	}
}

func TestRequestProcsQueuesCommand(t *testing.T) {
	_, httpServer := newTestServer(t)
	do(t, httpServer, http.MethodPost, "/api/v1/report", `{"hostname":"web-1"}`, testSecret)

	response := do(t, httpServer, http.MethodPost, "/api/v1/nodes/web-1/procs", "", testSecret)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", response.StatusCode)
	}

	response = do(t, httpServer, http.MethodGet, "/api/v1/commands?hostname=web-1", "", testSecret)
	var reply struct {
		Commands []string `json:"commands"`
	}
	if err := json.NewDecoder(response.Body).Decode(&reply); err != nil {
		t.Fatalf("decoding commands: %v", err)
	}
	if len(reply.Commands) != 1 || reply.Commands[0] != protocol.CommandProcs {
		t.Errorf("commands = %v, want [procs]", reply.Commands)
	}
}

func TestRequestProcsUnknownNode(t *testing.T) {
	_, httpServer := newTestServer(t)

	response := do(t, httpServer, http.MethodPost, "/api/v1/nodes/nope/procs", "", testSecret)
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", response.StatusCode)
	}
}

func TestUISnapshotHasNodes(t *testing.T) {
	server, httpServer := newTestServer(t)
	server.registry.Upsert(status.Report{Hostname: "web-1", IP: "10.0.0.12", AgentVersion: "0.1.0"}, server.now())

	// The page reads this without a secret: a browser cannot keep one.
	response := do(t, httpServer, http.MethodGet, "/ui/api/nodes", "", "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}

	var snapshot struct {
		ServerTime time.Time    `json:"server_time"`
		Nodes      []NodeStatus `json:"nodes"`
	}
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decoding snapshot: %v", err)
	}

	if snapshot.ServerTime.IsZero() {
		t.Error("snapshot has no server_time; the page needs it to measure ages")
	}
	if len(snapshot.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(snapshot.Nodes))
	}
	node := snapshot.Nodes[0]
	if node.Report.Hostname != "web-1" || node.Report.IP != "10.0.0.12" {
		t.Errorf("node = %+v", node.Report)
	}
	if node.Report.AgentVersion != "0.1.0" {
		t.Errorf("agent version = %q", node.Report.AgentVersion)
	}
}

func TestUISnapshotIsEmptyListNotNil(t *testing.T) {
	_, httpServer := newTestServer(t)

	response := do(t, httpServer, http.MethodGet, "/ui/api/nodes", "", "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("reading snapshot: %v", err)
	}
	// null would make the app's Array.isArray check fall back to an empty
	// list, which happens to work, but an empty array is the honest answer.
	if strings.Contains(string(body), `"nodes":null`) {
		t.Errorf("nodes came back as null: %s", body)
	}
	if !strings.Contains(string(body), `"nodes":[]`) {
		t.Errorf("nodes is not an empty array: %s", body)
	}
}

func TestAppIsServedAtRoot(t *testing.T) {
	_, httpServer := newTestServer(t)

	response := do(t, httpServer, http.MethodGet, "/", "", "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	if got := response.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("content type = %q, want html", got)
	}
}

func TestUnknownAPIPathIsNotFound(t *testing.T) {
	_, httpServer := newTestServer(t)

	// The app answers unknown paths, but never under /api/: a typo there has
	// to be visible.
	response := do(t, httpServer, http.MethodGet, "/api/v1/nope", "", "")
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", response.StatusCode)
	}
}

func TestRequestLogShowsEveryRequest(t *testing.T) {
	var logged bytes.Buffer
	cfg := config.Master{Secret: testSecret, ReportInterval: config.Duration{Duration: 15 * time.Second}}
	server := NewServer(cfg, log.New(&logged, "", 0))
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	do(t, httpServer, http.MethodPost, "/api/v1/report", `{"hostname":"web-1"}`, testSecret)
	do(t, httpServer, http.MethodGet, "/api/v1/nodes", "", testSecret)
	// A path this master does not serve must leave a trace, since that is what
	// a node pointed at the wrong address produces. The page handler answers
	// unknown GET paths with 404 and the router answers a POST to an unknown
	// path with 405, so both appear here.
	do(t, httpServer, http.MethodGet, "/api/v1/nope", "", "")
	do(t, httpServer, http.MethodPost, "/api/v2/report", `{}`, testSecret)

	lines := logged.String()
	want := []string{
		"POST /api/v1/report 204",
		"GET /api/v1/nodes 200",
		"GET /api/v1/nope 404",
		"POST /api/v2/report 405",
	}
	for _, line := range want {
		if !strings.Contains(lines, line) {
			t.Errorf("log does not contain %q; log was:\n%s", line, lines)
		}
	}
}
