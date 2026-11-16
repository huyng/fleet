package master

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/huyng/fleet/internal/config"
	"github.com/huyng/fleet/internal/procs"
	"github.com/huyng/fleet/internal/protocol"
	"github.com/huyng/fleet/internal/status"
	"github.com/huyng/fleet/internal/web"
)

// maxBodyBytes caps request bodies so a broken or hostile client cannot make
// the master allocate without limit.
const maxBodyBytes = 1 << 20

// shutdownGrace is how long running requests get to finish on shutdown.
const shutdownGrace = 5 * time.Second

// Server answers the API and serves the status page.
type Server struct {
	cfg      config.Master
	registry *Registry
	logger   *log.Logger
	// now is the clock, replaceable in tests.
	now func() time.Time
}

// NewServer builds a server around a fresh registry.
//
// Args:
//
//	cfg: validated master configuration.
//	logger: where first check-ins and queued commands are recorded.
func NewServer(cfg config.Master, logger *log.Logger) *Server {
	return &Server{
		cfg:      cfg,
		registry: NewRegistry(),
		logger:   logger,
		now:      time.Now,
	}
}

// Handler returns the routed HTTP handler, with the secret required on every
// API path.
//
// The page and the snapshot it reads are not protected by the secret: a
// browser cannot keep one, and the page is meant to be opened like a
// dashboard. Bind the master to a trusted address or put it behind a proxy.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/report", s.requireSecret(s.handleReport))
	mux.HandleFunc("GET /api/v1/nodes", s.requireSecret(s.handleNodes))
	mux.HandleFunc("GET /api/v1/nodes/{hostname}", s.requireSecret(s.handleNode))
	mux.HandleFunc("POST /api/v1/nodes/{hostname}/reboot", s.requireSecret(s.handleReboot))
	mux.HandleFunc("GET /api/v1/nodes/{hostname}/procs", s.requireSecret(s.handleGetProcs))
	mux.HandleFunc("POST /api/v1/nodes/{hostname}/procs", s.requireSecret(s.handleRequestProcs))
	mux.HandleFunc("PUT /api/v1/nodes/{hostname}/procs", s.requireSecret(s.handlePutProcs))
	mux.HandleFunc("GET /api/v1/commands", s.requireSecret(s.handleCommands))

	// What the page reads. Read only, and no secret, for the reason above.
	mux.HandleFunc("GET /ui/api/nodes", s.handleUISnapshot)

	app, err := web.Handler()
	if err != nil {
		// The bundle is embedded, so this only fails if the binary was built
		// without running the frontend build.
		panic(err)
	}
	mux.Handle("/", app)

	return s.logRequests(mux)
}

// logRequests writes one log line per request.
//
// A node pointed at the wrong address, or at a master with a different
// version, shows up here as a 404 on a path this master does not serve. That
// makes the usual "node reports 404" puzzle easy to read off the log.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := s.now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(recorder, r)

		s.logger.Printf("%s %s %d in %s", r.Method, r.URL.RequestURI(),
			recorder.status, s.now().Sub(start).Round(time.Millisecond))
	})
}

// statusRecorder remembers the status code a handler wrote so the request log
// can show it. Handlers that never call WriteHeader count as 200.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader records the status code before passing it on.
func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Serve listens and answers requests until ctx is cancelled.
//
// Args:
//
//	ctx: cancelled to start a graceful shutdown.
//	cfg: validated master configuration.
//
// Raises:
//
//	An error if the address cannot be bound or the server stops unexpectedly.
func Serve(ctx context.Context, cfg config.Master) error {
	logger := log.New(log.Writer(), "fleet master: ", log.LstdFlags)
	server := NewServer(cfg, logger)

	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", cfg.Listen, err)
	}

	httpServer := &http.Server{
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	served := make(chan error, 1)
	go func() {
		served <- httpServer.Serve(listener)
	}()
	logger.Printf("listening on http://%s", listener.Addr())

	select {
	case err := <-served:
		return err
	case <-ctx.Done():
		logger.Print("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}

// requireSecret rejects requests that do not carry the shared secret.
func (s *Server) requireSecret(next http.HandlerFunc) http.HandlerFunc {
	expected := []byte(s.cfg.Secret)
	return func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get(protocol.HeaderSecret))
		if subtle.ConstantTimeCompare(got, expected) != 1 {
			writeError(w, http.StatusUnauthorized, errors.New("bad or missing secret"))
			return
		}
		next(w, r)
	}
}

// handleReport stores a node's status sample.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	var rep status.Report
	if err := decodeJSON(w, r, &rep); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if rep.Hostname == "" {
		writeError(w, http.StatusBadRequest, errors.New("hostname must not be empty"))
		return
	}

	if first := s.registry.Upsert(rep, s.now()); first {
		s.logger.Printf("first report from %s (%s)", rep.Hostname, rep.IP)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleNodes lists every known node.
func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.registry.Snapshot(s.now(), s.cfg.StaleAfter()))
}

// handleNode returns one node.
func (s *Server) handleNode(w http.ResponseWriter, r *http.Request) {
	node, err := s.registry.Get(r.PathValue("hostname"), s.now(), s.cfg.StaleAfter())
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

// handleReboot queues a reboot for a node.
func (s *Server) handleReboot(w http.ResponseWriter, r *http.Request) {
	hostname := r.PathValue("hostname")
	if err := s.registry.Queue(hostname, protocol.CommandReboot); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	s.logger.Printf("queued reboot for %s", hostname)
	w.WriteHeader(http.StatusAccepted)
}

// handleCommands hands a node its queued commands and clears them.
//
// A node the master has not seen yet gets an empty list rather than an error.
// That is a normal state: a node asks for commands on the same tick it sends
// its first report, and that report may have failed. Answering 404 there only
// hides the real problem behind a confusing message.
func (s *Server) handleCommands(w http.ResponseWriter, r *http.Request) {
	hostname := r.URL.Query().Get("hostname")
	if hostname == "" {
		writeError(w, http.StatusBadRequest, errors.New("hostname query parameter is required"))
		return
	}

	commands := s.registry.Take(hostname)
	if commands == nil {
		commands = []string{}
	}
	writeJSON(w, http.StatusOK, struct {
		Commands []string `json:"commands"`
	}{Commands: commands})
}

// handleGetProcs returns the last running program list a node sent.
func (s *Server) handleGetProcs(w http.ResponseWriter, r *http.Request) {
	node, err := s.registry.Get(r.PathValue("hostname"), s.now(), s.cfg.StaleAfter())
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if node.ProcsAt.IsZero() {
		writeError(w, http.StatusNotFound, errors.New("no program list for this node yet"))
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Procs     []procs.Process `json:"procs"`
		Collected time.Time       `json:"collected_at"`
	}{Procs: node.Procs, Collected: node.ProcsAt})
}

// handleRequestProcs queues a program list collection for a node.
func (s *Server) handleRequestProcs(w http.ResponseWriter, r *http.Request) {
	hostname := r.PathValue("hostname")
	if err := s.registry.Queue(hostname, protocol.CommandProcs); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	s.logger.Printf("queued program list request for %s", hostname)
	w.WriteHeader(http.StatusAccepted)
}

// handlePutProcs stores a running program list from a node.
func (s *Server) handlePutProcs(w http.ResponseWriter, r *http.Request) {
	var list []procs.Process
	if err := decodeJSON(w, r, &list); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	hostname := r.PathValue("hostname")
	if err := s.registry.SetProcs(hostname, list, s.now()); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	s.logger.Printf("program list from %s: %d processes", hostname, len(list))
	w.WriteHeader(http.StatusNoContent)
}

// handleUISnapshot gives the page everything it shows in one request.
//
// The master's own clock travels with the data, so the page can work out how
// long ago a node reported without trusting the browser's clock. That keeps a
// laptop with a wrong clock from making every node look stale.
func (s *Server) handleUISnapshot(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	nodes := s.registry.Snapshot(now, s.cfg.StaleAfter())
	if nodes == nil {
		nodes = []NodeStatus{}
	}

	writeJSON(w, http.StatusOK, struct {
		ServerTime time.Time    `json:"server_time"`
		Nodes      []NodeStatus `json:"nodes"`
	}{
		ServerTime: now.UTC(),
		Nodes:      nodes,
	})
}

// decodeJSON reads a size limited JSON body into target.
//
// Args:
//
//	w: used to cap the body size.
//	r: the request to read.
//	target: the value to fill.
//
// Raises:
//
//	An error if the body is too large or is not valid JSON.
func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		return fmt.Errorf("bad JSON body: %w", err)
	}
	return nil
}

// writeJSON writes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The response has already started; log instead of replying.
		log.Printf("writing JSON response: %v", err)
	}
}

// writeError writes a JSON error body.
func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, struct {
		Error string `json:"error"`
	}{Error: err.Error()})
}
