// Package agent is the node side: it reports status to the master on a timer
// and carries out the commands the master sends back.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"syscall"
	"time"

	"github.com/huyng/fleet/internal/config"
	"github.com/huyng/fleet/internal/procs"
	"github.com/huyng/fleet/internal/protocol"
	"github.com/huyng/fleet/internal/status"
	"github.com/huyng/fleet/internal/version"
)

// requestTimeout caps a single call to the master, so a silent master cannot
// stall the reporting loop.
const requestTimeout = 10 * time.Second

// rebootTimeout caps how long a reboot command may run before it is given up
// on. A machine that reboots normally returns well before this.
const rebootTimeout = 30 * time.Second

// Agent reports this machine's status and runs the commands it is given.
type Agent struct {
	cfg      config.Node
	master   *url.URL
	hostname string
	client   *http.Client
	logger   *log.Logger

	// These are fields so tests can supply fixed data.
	gather func() (status.Report, error)
	list   func() ([]procs.Process, error)
}

// Run reports to the master until ctx is cancelled.
//
// Args:
//
//	ctx: cancelled to stop reporting cleanly.
//	cfg: validated node configuration.
//
// Raises:
//
//	An error if the master URL is unusable or the hostname is unknown. Bad
//	reports are logged and retried on the next tick rather than stopping.
func Run(ctx context.Context, cfg config.Node) error {
	agent, err := newAgent(cfg, log.New(log.Writer(), "fleet node: ", log.LstdFlags))
	if err != nil {
		return err
	}

	agent.logger.Printf("reporting to %s every %s as %s", agent.master, cfg.Interval.Duration, agent.hostname)

	// Report straight away so the master does not wait a full interval.
	agent.tick(ctx)

	ticker := time.NewTicker(cfg.Interval.Duration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			agent.tick(ctx)
		}
	}
}

// newAgent builds an agent from a validated configuration. Tests replace the
// gathering functions after this returns to avoid touching the real machine.
//
// Raises:
//
//	An error if the master URL is unusable or the hostname is unknown.
func newAgent(cfg config.Node, logger *log.Logger) (*Agent, error) {
	master, err := parseMasterURL(cfg.Master)
	if err != nil {
		return nil, err
	}
	hostname, err := status.Hostname(cfg.Hostname)
	if err != nil {
		return nil, fmt.Errorf("finding hostname: %w", err)
	}

	agent := &Agent{
		cfg:      cfg,
		master:   master,
		hostname: hostname,
		client:   &http.Client{Timeout: requestTimeout},
		logger:   logger,
	}
	agent.gather = func() (status.Report, error) {
		return status.Gather(status.Options{
			Hostname:     agent.hostname,
			ReportPath:   cfg.ReportPath,
			AgentVersion: version.Version,
		})
	}
	agent.list = procs.List
	return agent, nil
}

// parseMasterURL checks the configured master address.
//
// Raises:
//
//	An error if the address is not an absolute http or https URL.
func parseMasterURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("bad master URL %q: %w", raw, err)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("bad master URL %q: no host", raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("bad master URL %q: scheme must be http or https", raw)
	}
	// Every request is built from this URL by replacing the path, so a path
	// given here would be dropped and each call would land somewhere the
	// master does not serve. Say so instead of failing later with a 404.
	if parsed.Path != "" && parsed.Path != "/" {
		return nil, fmt.Errorf("bad master URL %q: drop the path, use the scheme and address only, such as http://%s", raw, parsed.Host)
	}
	return parsed, nil
}

// tick sends one report and then picks up any commands waiting for this node.
func (a *Agent) tick(ctx context.Context) {
	if err := a.report(ctx); err != nil {
		a.logger.Printf("report failed: %v", err)
		// Still ask for commands: the master may have work queued from when
		// reporting worked.
	}
	if err := a.poll(ctx); err != nil {
		a.logger.Printf("polling for commands failed: %v", err)
	}
}

// report gathers this machine's status and posts it to the master.
//
// Raises:
//
//	An error if the status cannot be gathered or the master refuses it.
func (a *Agent) report(ctx context.Context) error {
	rep, err := a.gather()
	if err != nil {
		return err
	}

	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(rep); err != nil {
		return fmt.Errorf("encoding report: %w", err)
	}
	return a.send(ctx, http.MethodPost, "/api/v1/report", &body, http.StatusNoContent)
}

// poll fetches the queued commands and runs them in order.
//
// Raises:
//
//	An error if the master cannot be reached or the reply is unreadable.
func (a *Agent) poll(ctx context.Context) error {
	endpoint := a.endpoint("/api/v1/commands")
	query := endpoint.Query()
	query.Set("hostname", a.hostname)
	endpoint.RawQuery = query.Encode()

	request, err := a.newRequest(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return err
	}
	response, err := a.client.Do(request)
	if err != nil {
		return describeTransportFailure(http.MethodGet, endpoint.String(), err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return describeFailure(http.MethodGet, endpoint.String(), response)
	}

	var reply struct {
		Commands []string `json:"commands"`
	}
	if err := json.NewDecoder(response.Body).Decode(&reply); err != nil {
		return fmt.Errorf("reading commands: %w", err)
	}

	for _, command := range reply.Commands {
		a.run(ctx, command)
	}
	return nil
}

// run carries out one command, logging anything that goes wrong. An unknown
// command is ignored so a newer master does not break an older node.
//
// Args:
//
//	ctx: used to bound how long the command may run.
//	command: one of the protocol command names.
func (a *Agent) run(ctx context.Context, command string) {
	switch command {
	case protocol.CommandReboot:
		a.reboot(ctx)
	case protocol.CommandProcs:
		if err := a.sendProcs(ctx); err != nil {
			a.logger.Printf("listing programs failed: %v", err)
		}
	case "":
		// Nothing to do.
	default:
		a.logger.Printf("ignoring unknown command %q", command)
	}
}

// reboot runs the configured reboot command.
func (a *Agent) reboot(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, rebootTimeout)
	defer cancel()

	a.logger.Printf("reboot requested, running %v", a.cfg.RebootCommand)
	output, err := exec.CommandContext(ctx, a.cfg.RebootCommand[0], a.cfg.RebootCommand[1:]...).CombinedOutput()
	if err != nil {
		a.logger.Printf("reboot command failed: %v: %s", err, bytes.TrimSpace(output))
		return
	}
	a.logger.Print("reboot command finished")
}

// sendProcs collects the running programs and posts them to the master.
//
// Raises:
//
//	An error if the list cannot be gathered or the master refuses it.
func (a *Agent) sendProcs(ctx context.Context) error {
	list, err := a.list()
	if err != nil {
		return err
	}

	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(list); err != nil {
		return fmt.Errorf("encoding program list: %w", err)
	}

	path := fmt.Sprintf("/api/v1/nodes/%s/procs", url.PathEscape(a.hostname))
	if err := a.send(ctx, http.MethodPut, path, &body, http.StatusNoContent); err != nil {
		return err
	}
	a.logger.Printf("sent %d programs to the master", len(list))
	return nil
}

// send sends a JSON body to path and checks for the expected status.
//
// Args:
//
//	ctx: cancelled to abandon the request.
//	method: HTTP method to use.
//	path: path on the master.
//	body: the JSON body.
//	wantStatus: the status code that means success.
//
// Raises:
//
//	An error if the request fails or the status is not the expected one.
func (a *Agent) send(ctx context.Context, method, path string, body io.Reader, wantStatus int) error {
	request, err := a.newRequest(ctx, method, a.endpoint(path).String(), body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := a.client.Do(request)
	if err != nil {
		return describeTransportFailure(method, request.URL.String(), err)
	}
	defer response.Body.Close()

	if response.StatusCode != wantStatus {
		return describeFailure(method, request.URL.String(), response)
	}
	return nil
}

// describeTransportFailure turns a request that never got an answer into a
// message that names the likely cause. A refused connection is nearly always a
// master that is not running, or a wrong port, and saying so saves a puzzled
// reading of the node log.
//
// Args:
//
//	method: HTTP method that was used.
//	url: full URL that was requested.
//	err: the transport error.
func describeTransportFailure(method, url string, err error) error {
	wrapped := fmt.Errorf("%s %s: %w", method, url, err)

	if !errors.Is(err, syscall.ECONNREFUSED) {
		return wrapped
	}
	return fmt.Errorf("%w\n\tnothing is listening at that address: start the master, "+
		"or check the address and port in the node config", wrapped)
}

// describeFailure turns a refused request into a message an operator can act
// on. A 404 usually means the address points at something other than a fleet
// master, or at a master older than this node, so the message says that much.
//
// Args:
//
//	method: HTTP method that was used.
//	url: full URL that was requested.
//	response: the refusal, whose body is read for the master's own message.
func describeFailure(method, url string, response *http.Response) error {
	message, _ := io.ReadAll(io.LimitReader(response.Body, 512))
	err := fmt.Errorf("%s %s: master said %s: %s",
		method, url, response.Status, bytes.TrimSpace(message))

	if response.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w\n\tno fleet master answered that path: check the master's "+
			"address in the node config, and that it is the same fleet version", err)
	}
	return err
}

// newRequest builds a request against the master with the secret header set.
//
// Raises:
//
//	An error if the request cannot be built.
func (a *Agent) newRequest(ctx context.Context, method, target string, body io.Reader) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set(protocol.HeaderSecret, a.cfg.Secret)
	return request, nil
}

// endpoint joins a path onto the master URL.
func (a *Agent) endpoint(path string) *url.URL {
	joined := *a.master
	joined.Path = path
	joined.RawQuery = ""
	return &joined
}
