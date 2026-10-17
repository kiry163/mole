package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kiry163/mole/pkg/approval"
	"github.com/kiry163/mole/pkg/config"
)

// Runtime describes the gateway the console is served by. Both run in the same
// process, so the console never starts, stops or restarts anything: it reports
// what the host attached here and asks the host to reload configuration in
// place.
type Runtime struct {
	// Address is the host:port the gateway and the console listen on.
	Address string
	// Version is the build version of the running process.
	Version string
	// PID is the process id of the running gateway.
	PID int
	// Channels lists the enabled channel names.
	Channels []string
	// Approvals returns the process-wide approval store, or nil when the console
	// runs without one.
	Approvals func() *approval.Store
	// Reload asks the host for an in-place configuration reload. It may be nil
	// when the host has no reload plumbing, in which case reload requests are
	// dropped.
	Reload func()
}

// SetRuntime attaches the running gateway to the handler so the runtime
// endpoints can report it. Hosts call it before the handlers serve traffic;
// a later call replaces the previous runtime.
func (h *Handler) SetRuntime(runtime *Runtime) {
	h.runtimeMu.Lock()
	defer h.runtimeMu.Unlock()
	h.runtime = runtime
}

// currentRuntime returns the runtime the host attached, or nil when the console
// runs on its own (tests and non-gateway hosts).
func (h *Handler) currentRuntime() *Runtime {
	h.runtimeMu.RLock()
	defer h.runtimeMu.RUnlock()
	return h.runtime
}

// approvalStore returns the process-wide approval store, or nil when the console
// runs without one, so the approval endpoints can answer 503.
func (h *Handler) approvalStore() *approval.Store {
	runtime := h.currentRuntime()
	if runtime == nil || runtime.Approvals == nil {
		return nil
	}
	return runtime.Approvals()
}

// requestReload asks the host to reload configuration in place. Handlers call it
// after they change configuration the running gateway has already read.
func (h *Handler) requestReload() {
	if runtime := h.currentRuntime(); runtime != nil && runtime.Reload != nil {
		runtime.Reload()
	}
}

// runtimeStatusUnknown is reported while no runtime is attached, mirroring the
// SPA's "unknown" gateway state.
const runtimeStatusUnknown = "unknown"

// runtimeStatusRunning is reported for an attached runtime: the console is only
// reachable while the gateway that serves it is running.
const runtimeStatusRunning = "running"

// runtimeStatusResponse is the in-process gateway status the console polls.
//
// The field names are the console contract the SPA reads.
type runtimeStatusResponse struct {
	Status   string   `json:"status"`
	PID      int      `json:"pid"`
	Version  string   `json:"version"`
	Address  string   `json:"address"`
	Channels []string `json:"channels"`
}

// registerRuntimeRoutes binds the in-process runtime endpoints to the ServeMux.
func (h *Handler) registerRuntimeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/gateway/status", h.handleRuntimeStatus)
	mux.HandleFunc("GET /api/gateway/logs", h.handleRuntimeLogs)
}

// handleRuntimeStatus reports the gateway this console is running inside.
//
//	GET /api/gateway/status
func (h *Handler) handleRuntimeStatus(w http.ResponseWriter, _ *http.Request) {
	response := runtimeStatusResponse{Status: runtimeStatusUnknown, Channels: []string{}}

	if runtime := h.currentRuntime(); runtime != nil {
		response.Status = runtimeStatusRunning
		response.PID = runtime.PID
		response.Version = runtime.Version
		response.Address = runtime.Address
		if len(runtime.Channels) > 0 {
			response.Channels = append([]string(nil), runtime.Channels...)
		}
	}

	writeJSONResponse(w, response)
}

// runtimeLogFilePath is the log file the running gateway appends to, relative to
// the mole home directory. pkg/gateway enables file logging on that path, so
// tailing it reports exactly what the gateway logged.
const runtimeLogFilePath = "logs/gateway.log"

const (
	// defaultRuntimeLogLimit is the tail window returned when the client does
	// not ask for a size.
	defaultRuntimeLogLimit = 200
	// maxRuntimeLogLimit caps one window, so a single request cannot buffer the
	// whole log file.
	maxRuntimeLogLimit = 5000
	// maxRuntimeLogOffset caps how far back the window may start.
	maxRuntimeLogOffset = 50000
	// maxRuntimeLogLineBytes bounds one log line. The structured logger emits
	// far shorter records, and the bound keeps a corrupt file from buffering
	// unbounded memory.
	maxRuntimeLogLineBytes = 1 << 20
)

// runtimeLogsResponse is the console log page's payload: the requested window,
// oldest line first, plus the total line count of the file.
type runtimeLogsResponse struct {
	Logs     []string `json:"logs"`
	LogTotal int      `json:"log_total"`
}

// handleRuntimeLogs tails the gateway log file.
//
//	GET /api/gateway/logs?offset=<lines>&limit=<lines>
//
// offset counts back from the end of the file (0, the default, ends at the
// newest line) and limit is the window size in lines (200 by default).
func (h *Handler) handleRuntimeLogs(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit := logLineCountParam(query.Get("limit"), defaultRuntimeLogLimit, maxRuntimeLogLimit)
	offset := logLineCountParam(query.Get("offset"), 0, maxRuntimeLogOffset)

	lines, total, err := tailLogLines(runtimeLogPath(), offset, limit)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read gateway log: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSONResponse(w, runtimeLogsResponse{Logs: lines, LogTotal: total})
}

// runtimeLogPath returns the absolute path of the gateway log file.
func runtimeLogPath() string {
	return filepath.Join(config.GetHome(), runtimeLogFilePath)
}

// logLineCountParam parses a non-negative line count and clamps it to max.
// Absent or malformed values fall back to fallback.
func logLineCountParam(raw string, fallback, max int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

// tailLogLines returns up to limit lines ending offset lines before the end of
// the file at path, together with the number of lines the file holds.
//
// The file is read once and only the requested window stays in memory, so a long
// log costs a bounded amount of memory. A missing file is not an error; it means
// the gateway has not written a log yet.
func tailLogLines(path string, offset, limit int) ([]string, int, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, 0, nil
		}
		return nil, 0, err
	}
	defer file.Close()

	// The window holds the requested page plus the newer lines offset skips.
	// It is a ring so that scanning a long file never costs a per-line copy.
	window := offset + limit
	ring := make([]string, 0, window)
	next, total := 0, 0

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxRuntimeLogLineBytes)
	for scanner.Scan() {
		total++
		if window == 0 {
			continue
		}
		if len(ring) < window {
			ring = append(ring, scanner.Text())
			continue
		}
		ring[next] = scanner.Text()
		next = (next + 1) % window
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}

	lines := rotateLogRing(ring, next)
	if len(lines) > offset {
		lines = lines[:len(lines)-offset]
	} else {
		lines = lines[:0]
	}

	return lines, total, nil
}

// rotateLogRing returns the ring's lines in insertion order, oldest first. next
// is the slot the next insertion would overwrite; it stays 0 while the ring has
// not wrapped.
func rotateLogRing(ring []string, next int) []string {
	if next == 0 {
		return ring
	}

	ordered := make([]string, 0, len(ring))
	ordered = append(ordered, ring[next:]...)
	ordered = append(ordered, ring[:next]...)
	return ordered
}

// writeJSONResponse encodes payload as the response body.
func writeJSONResponse(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
