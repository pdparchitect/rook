package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// NewRunID returns a sortable, per-process run identifier - a UTC timestamp plus
// the PID - so concurrent Rook runs each get their own run directory and never
// clobber one another's artifacts.
func NewRunID() string {
	return time.Now().UTC().Format("20060102-150405") + "-" + strconv.Itoa(os.Getpid())
}

// eventLogger appends structured run events to a JSONL file - the durable log of
// a run, alongside the status snapshot. A nil *eventLogger is a no-op.
type eventLogger struct {
	mu sync.Mutex
	f  *os.File
}

func newEventLogger(path string) *eventLogger {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil
	}
	return &eventLogger{f: f}
}

// log appends one event record ({"ts", "event", ...fields}) as a JSON line.
// Best-effort: errors never fail a run.
func (l *eventLogger) log(kind string, fields map[string]interface{}) {
	if l == nil {
		return
	}
	rec := map[string]interface{}{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"event": kind,
	}
	for k, v := range fields {
		rec[k] = v
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.f.Write(append(data, '\n'))
}

func (l *eventLogger) close() {
	if l == nil {
		return
	}
	_ = l.f.Close()
}

// Status is the machine-readable snapshot of a run. When a status file is
// configured, Rook keeps it updated as the run progresses so external consumers
// - the desktop widget in particular - can render live activity without parsing
// the human-readable event stream.
type Status struct {
	// State is the coarse run state: "running", "done", or "error".
	State string `json:"state"`
	// Model is the model driving the run.
	Model string `json:"model"`
	// Scope is the authorization boundary the run was given.
	Scope string `json:"scope,omitempty"`
	// Iteration is the current plan/act/observe cycle.
	Iteration int `json:"iteration"`
	// MaxIterations is the safety cap.
	MaxIterations int `json:"max_iterations"`
	// Tool is the tool the agent is currently running (read/write/edit/exec/…).
	Tool string `json:"tool,omitempty"`
	// Action is a short, one-line summary of the current tool's arguments.
	Action string `json:"action,omitempty"`
	// StartedAt / UpdatedAt bound the run in time.
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// ExitCode is the agent's exit code once it has exited.
	ExitCode *int `json:"exit_code,omitempty"`
}

// statusWriter persists a Status to a file, atomically, as a run progresses. A
// nil *statusWriter is a no-op, so callers never branch on whether a path was
// configured.
type statusWriter struct {
	path string
	mu   sync.Mutex
	s    Status
}

// newStatusWriter returns a writer for path seeded with s, or nil if path is
// empty. It creates the parent directory and writes the initial snapshot.
func newStatusWriter(path string, s Status) *statusWriter {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	w := &statusWriter{path: path, s: s}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	w.mu.Lock()
	w.flushLocked()
	w.mu.Unlock()
	return w
}

// update applies fn to the status under the lock, stamps UpdatedAt, and rewrites
// the file. Safe (and cheap) to call on a nil writer.
func (w *statusWriter) update(fn func(*Status)) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	fn(&w.s)
	w.s.UpdatedAt = time.Now()
	w.flushLocked()
}

// flushLocked writes the current status atomically (temp file + rename). The
// caller must hold w.mu. Errors are ignored: a status file is best-effort
// telemetry and must never fail a run.
func (w *statusWriter) flushLocked() {
	data, err := json.MarshalIndent(w.s, "", "  ")
	if err != nil {
		return
	}
	tmp := w.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, w.path)
}

// summarizeArgs renders a tool's arguments as a short, single-line action for
// the status file - the command for exec, the path for file tools, otherwise a
// compact form - truncated so the widget stays tidy.
func summarizeArgs(args interface{}) string {
	const limit = 120

	clip := func(s string) string {
		s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
		if len(s) > limit {
			s = s[:limit] + "…"
		}
		return s
	}

	if m, ok := args.(map[string]interface{}); ok {
		for _, key := range []string{"command", "cmd", "path", "file", "url", "query"} {
			if v, ok := m[key]; ok {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					return clip(s)
				}
			}
		}
	}
	if args == nil {
		return ""
	}
	return clip(fmt.Sprintf("%v", args))
}
