package agent

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/huyng/fleet/internal/config"
	"github.com/huyng/fleet/internal/master"
	"github.com/huyng/fleet/internal/procs"
	"github.com/huyng/fleet/internal/protocol"
	"github.com/huyng/fleet/internal/status"
)

const testSecret = "s3cret"

// fixedReport is the status a test agent sends, so the real machine is never
// read.
var fixedReport = status.Report{
	Hostname: "web-1",
	IP:       "10.0.0.12",
	Load:     status.Load{OneMinute: 0.4, FiveMinutes: 0.5, FifteenMinute: 0.6},
	Memory:   status.Usage{Used: 10, Total: 20},
	Disk:     status.Usage{Used: 5, Total: 10},
}

// harness is a master and an agent pointed at it.
type harness struct {
	masterURL string
	agent     *Agent
}

// newHarness starts a master on a random port and an agent aimed at it.
//
// Args:
//
//	t: the test.
//	cfg: node configuration to start from; the master URL and secret are
//	  filled in when left empty.
func newHarness(t *testing.T, cfg config.Node) harness {
	t.Helper()

	masterCfg := config.Master{Secret: testSecret, ReportInterval: config.Duration{Duration: 15 * time.Second}}
	httpServer := httptest.NewServer(master.NewServer(masterCfg, log.New(io.Discard, "", 0)).Handler())
	t.Cleanup(httpServer.Close)

	cfg.Master = httpServer.URL
	if cfg.Secret == "" {
		cfg.Secret = testSecret
	}
	if cfg.Hostname == "" {
		cfg.Hostname = "web-1"
	}

	agent, err := newAgent(cfg, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("newAgent: %v", err)
	}
	agent.gather = func() (status.Report, error) { return fixedReport, nil }
	agent.list = func() ([]procs.Process, error) {
		return []procs.Process{{PID: 1, User: "root", Command: "systemd"}}, nil
	}
	return harness{masterURL: httpServer.URL, agent: agent}
}

// call sends a secret-bearing request to the test master.
func (h harness) call(t *testing.T, method, path string) *http.Response {
	t.Helper()

	request, err := http.NewRequest(method, h.masterURL+path, strings.NewReader(""))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	request.Header.Set(protocol.HeaderSecret, testSecret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	t.Cleanup(func() { response.Body.Close() })
	return response
}

// nodeState asks the master what it knows about a node.
func (h harness) nodeState(t *testing.T, hostname string) master.NodeStatus {
	t.Helper()

	response := h.call(t, http.MethodGet, "/api/v1/nodes/"+hostname)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status for %s = %d, want 200", hostname, response.StatusCode)
	}
	var node master.NodeStatus
	if err := json.NewDecoder(response.Body).Decode(&node); err != nil {
		t.Fatalf("decoding node: %v", err)
	}
	return node
}

func TestReportReachesMaster(t *testing.T) {
	h := newHarness(t, config.Node{})

	if err := h.agent.report(context.Background()); err != nil {
		t.Fatalf("report: %v", err)
	}

	node := h.nodeState(t, "web-1")
	if node.Report.IP != "10.0.0.12" {
		t.Errorf("ip = %q, want 10.0.0.12", node.Report.IP)
	}
}

func TestReportWithWrongSecret(t *testing.T) {
	h := newHarness(t, config.Node{Secret: "wrong"})

	if err := h.agent.report(context.Background()); err == nil {
		t.Error("expected the master to refuse a wrong secret")
	}
}

func TestPollRunsProcsCommand(t *testing.T) {
	h := newHarness(t, config.Node{})
	if err := h.agent.report(context.Background()); err != nil {
		t.Fatalf("report: %v", err)
	}

	// Asking for the program list is what the master's procs command does.
	response := h.call(t, http.MethodPost, "/api/v1/nodes/web-1/procs")
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("request procs status = %d, want 202", response.StatusCode)
	}

	if err := h.agent.poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}

	node := h.nodeState(t, "web-1")
	if len(node.Procs) != 1 || node.Procs[0].Command != "systemd" {
		t.Errorf("procs = %+v", node.Procs)
	}
}

func TestPollRunsRebootCommand(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "rebooted")
	// A harmless stand-in for systemctl reboot that leaves proof it ran.
	h := newHarness(t, config.Node{RebootCommand: []string{"touch", marker}})
	if err := h.agent.report(context.Background()); err != nil {
		t.Fatalf("report: %v", err)
	}

	response := h.call(t, http.MethodPost, "/api/v1/nodes/web-1/reboot")
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("reboot status = %d, want 202", response.StatusCode)
	}
	if err := h.agent.poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}

	if _, err := os.Stat(marker); err != nil {
		t.Errorf("reboot command did not run: %v", err)
	}
}

func TestFailingRebootCommandIsLogged(t *testing.T) {
	h := newHarness(t, config.Node{RebootCommand: []string{"false"}})

	// A failing command must not panic or stop the agent.
	h.agent.run(context.Background(), protocol.CommandReboot)
}

func TestUnknownCommandIsIgnored(t *testing.T) {
	h := newHarness(t, config.Node{})

	// Must not panic or run anything.
	h.agent.run(context.Background(), "self-destruct")
}

func TestParseMasterURL(t *testing.T) {
	good := []string{"http://10.0.0.1:8080", "https://fleet.example.com", "http://127.0.0.1:8080"}
	for _, raw := range good {
		if _, err := parseMasterURL(raw); err != nil {
			t.Errorf("%q: unexpected error: %v", raw, err)
		}
	}

	bad := []string{"", "10.0.0.1:8080", "ftp://10.0.0.1", "http://", "http://10.0.0.1:8080/fleet"}
	for _, raw := range bad {
		if _, err := parseMasterURL(raw); err == nil {
			t.Errorf("%q: expected an error", raw)
		}
	}
}

func TestDescribeTransportFailureExplainsRefused(t *testing.T) {
	// What net/http reports when nothing is listening on the port.
	err := describeTransportFailure(http.MethodGet,
		"http://127.0.0.1:8080/api/v1/commands",
		syscall.ECONNREFUSED)

	for _, want := range []string{
		"GET http://127.0.0.1:8080/api/v1/commands",
		"nothing is listening at that address",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not contain %q: %v", want, err)
		}
	}
}

func TestDescribeTransportFailureLeavesOtherErrorsAlone(t *testing.T) {
	err := describeTransportFailure(http.MethodGet, "http://10.0.0.1:8080/x", syscall.ETIMEDOUT)

	if strings.Contains(err.Error(), "nothing is listening") {
		t.Errorf("a timeout should not be called a refused connection: %v", err)
	}
	if !strings.Contains(err.Error(), "http://10.0.0.1:8080/x") {
		t.Errorf("error does not name the URL: %v", err)
	}
}

func TestDescribeFailureExplains404(t *testing.T) {
	// What a node sees when the address it was given serves something else.
	response := &http.Response{
		Status:     "404 Not Found",
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader("404 page not found")),
	}

	err := describeFailure(http.MethodPost, "http://10.0.0.1:8080/api/v1/report", response)
	for _, want := range []string{
		"POST http://10.0.0.1:8080/api/v1/report",
		"404 Not Found",
		"404 page not found",
		"no fleet master answered that path",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not contain %q: %v", want, err)
		}
	}
}

func TestDescribeFailureLeavesOtherStatusesAlone(t *testing.T) {
	response := &http.Response{
		Status:     "401 Unauthorized",
		StatusCode: http.StatusUnauthorized,
		Body:       io.NopCloser(strings.NewReader(`{"error":"bad or missing secret"}`)),
	}

	err := describeFailure(http.MethodPost, "http://10.0.0.1:8080/api/v1/report", response)
	if strings.Contains(err.Error(), "no fleet master answered") {
		t.Errorf("401 should not be described as a wrong address: %v", err)
	}
	if !strings.Contains(err.Error(), "bad or missing secret") {
		t.Errorf("error does not pass on the master's message: %v", err)
	}
}
