package agent

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/openzot/openzot/agent"
)

// readJSONL returns every record in a JSONL file, keyed for lookup by event.
func readJSONL(t *testing.T, path string) []map[string]interface{} {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()

	var recs []map[string]interface{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var rec map[string]interface{}
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("parse log line: %v", err)
		}
		recs = append(recs, rec)
	}
	return recs
}

// The recorder maps the engine's event stream onto the two run artifacts: the
// status snapshot tracks the current tool and the terminal outcome, and the log
// carries one line per event with a tool call's arguments parsed to an object.
func TestArtifactRecorderWritesStatusAndLog(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	logPath := filepath.Join(dir, "events.jsonl")

	rec := &artifactRecorder{
		status: newStatusWriter(statusPath, Status{State: "running", Model: "glm-5.2", MaxIterations: 42}),
		log:    newEventLogger(logPath),
	}

	_ = rec.RecordEvent("iteration", "", "", 3)
	_ = rec.RecordEvent("toolCallStart", "exec", `{"command":"ls -la /etc"}`, 3)
	_ = rec.RecordEvent("toolCallEnd", "exec", "", 3)
	_ = rec.RecordEvent("toolCallError", "read", "no such file", 3)
	_ = rec.RecordEvent("token", "", "ignored", 3) // must not produce a line
	_ = rec.RecordResult(agent.Summary{Reason: "success", Code: 0})
	rec.close()

	// status.json reflects the terminal state, and the current tool/action are
	// cleared once the run exits.
	var s Status
	data, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parse status: %v", err)
	}
	if s.State != "done" {
		t.Errorf("state = %q, want done", s.State)
	}
	if s.Iteration != 3 {
		t.Errorf("iteration = %d, want 3", s.Iteration)
	}
	if s.ExitCode == nil || *s.ExitCode != 0 {
		t.Errorf("exit code = %v, want 0", s.ExitCode)
	}
	if s.Tool != "" || s.Action != "" {
		t.Errorf("tool/action must be cleared at exit, got %q/%q", s.Tool, s.Action)
	}

	// The log carries iteration, tool_start, tool_end, tool_error, exit - and
	// nothing for the token event.
	recs := readJSONL(t, logPath)
	if len(recs) != 5 {
		t.Fatalf("expected 5 log lines (token ignored), got %d", len(recs))
	}

	// The tool_start line's arguments are a parsed object, and the status action
	// was summarized from them.
	var toolStart map[string]interface{}
	for _, r := range recs {
		if r["event"] == "tool_start" {
			toolStart = r
		}
	}
	if toolStart == nil {
		t.Fatal("no tool_start line")
	}
	args, ok := toolStart["args"].(map[string]interface{})
	if !ok || args["command"] != "ls -la /etc" {
		t.Errorf("tool_start args not a parsed object: %#v", toolStart["args"])
	}
}

// A run with no run directory records nothing: the writers are nil, and every
// method must stay safe rather than panic.
func TestArtifactRecorderNilWritersAreSafe(t *testing.T) {
	rec := &artifactRecorder{} // no status, no log
	_ = rec.RecordMessage(agent.Message{})
	_ = rec.RecordEvent("iteration", "", "", 1)
	_ = rec.RecordEvent("toolCallStart", "exec", `{"command":"id"}`, 1)
	_ = rec.RecordReset()
	_ = rec.RecordResult(agent.Summary{Code: 1})
	rec.close()
}

// A tool call whose arguments are not a JSON object still yields a usable action
// (the clipped raw text) and a raw-string args field in the log.
func TestActionFromArgsHandlesNonObject(t *testing.T) {
	if got := actionFromArgs("not json"); got != "not json" {
		t.Errorf("actionFromArgs(non-json) = %q, want the clipped raw text", got)
	}
	if got := rawArgs(`{"path":"x"}`); got.(map[string]interface{})["path"] != "x" {
		t.Errorf("rawArgs must parse an object, got %#v", got)
	}
	if got := rawArgs("plain"); got != "plain" {
		t.Errorf("rawArgs(non-json) = %v, want the raw string", got)
	}
}

// RecordFailure persists a provider failure as it happens, so a run killed
// mid-retry still leaves the failing exchange behind in the event log.
func TestRecordFailureWritesToLog(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "events.jsonl")

	rec := &artifactRecorder{
		log: newEventLogger(logPath),
	}
	defer rec.close()

	// A nil failure is a safe no-op.
	if err := rec.RecordFailure(nil); err != nil {
		t.Errorf("RecordFailure(nil) = %v", err)
	}

	// A real failure writes a provider_failure line.
	if err := rec.RecordFailure(&agent.Failure{
		Status:        429,
		RequestBytes:  1024,
		ResponseBody:   "rate limited",
	}); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	rec.close()

	recs := readJSONL(t, logPath)
	var found bool
	for _, r := range recs {
		if r["event"] == "provider_failure" {
			found = true
			if r["status"].(float64) != 429 {
				t.Errorf("status = %v, want 429", r["status"])
			}
			if r["response"] != "rate limited" {
				t.Errorf("response = %v", r["response"])
			}
		}
	}
	if !found {
		t.Error("no provider_failure line in the log")
	}
}

// RecordFailure on a nil-writer recorder is safe.
func TestRecordFailureNilWriterIsSafe(t *testing.T) {
	rec := &artifactRecorder{}
	if err := rec.RecordFailure(&agent.Failure{Status: 500}); err != nil {
		t.Errorf("RecordFailure on nil writers = %v", err)
	}
}

// multiRecorder fans every call to two recorders. Neither failure silences
// the other: recording is best-effort.
func TestMultiRecorderFansOut(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	logPath := filepath.Join(dir, "events.jsonl")
	sesPath := filepath.Join(dir, "session.jsonl")

	artifact := &artifactRecorder{
		status: newStatusWriter(statusPath, Status{State: "running"}),
		log:    newEventLogger(logPath),
	}
	defer artifact.close()

	// A second artifact recorder as a stand-in for the session recorder.
	secondary := &artifactRecorder{
		status: newStatusWriter(sesPath+".status", Status{State: "running"}),
		log:    newEventLogger(sesPath),
	}
	defer secondary.close()

	multi := &multiRecorder{artifact, secondary}

	_ = multi.RecordEvent("iteration", "", "", 1)
	_ = multi.RecordEvent("toolCallStart", "exec", `{"command":"ls"}`, 1)
	_ = multi.RecordMessage(agent.Message{Type: agent.TypeUser, Text: "hello"})
	_ = multi.RecordResult(agent.Summary{Reason: "success", Code: 0})

	// Both recorders received every call.
	primaryLog := readJSONL(t, logPath)
	secondaryLog := readJSONL(t, sesPath)

	if len(primaryLog) == 0 {
		t.Error("primary recorder received nothing")
	}

	if len(secondaryLog) == 0 {
		t.Error("secondary recorder received nothing")
	}

	// Both have the same event count (iteration + tool_start + exit = 3;
	// RecordMessage produces no event-log line).
	if len(primaryLog) != len(secondaryLog) {
		t.Errorf("log line count mismatch: primary=%d secondary=%d",
			len(primaryLog), len(secondaryLog))
	}
}

// multiRecorder is safe when either recorder is nil (as when RunDir is empty
// and only session recording is active).
func TestMultiRecorderNilSafe(t *testing.T) {
	secondary := &artifactRecorder{}
	defer secondary.close()

	// A nil interface, not a typed nil pointer.
	multi := &multiRecorder{nil, secondary}

	// None of these should panic.
	_ = multi.RecordEvent("iteration", "", "", 1)
	_ = multi.RecordMessage(agent.Message{})
	_ = multi.RecordResult(agent.Summary{})
	_ = multi.RecordFailure(nil)
	_ = multi.RecordReset()
}

// flushLocked writes the status atomically. A writer whose directory has been
// removed fails silently rather than panicking - status is best-effort.
func TestStatusWriterFlushSurvivesMissingDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gone", "status.json")
	w := newStatusWriter(path, Status{State: "running"})
	if w == nil {
		t.Skip("could not create writer (parent dir was already gone)")
	}

	// Remove the parent directory, then update: the rename in flushLocked
	// fails, but the call must not panic.
	_ = os.RemoveAll(filepath.Dir(path))
	w.update(func(s *Status) { s.Iteration = 1 })
}

// newEventLogger returns nil when the path is empty, and also nil when the
// file cannot be created (e.g. the directory does not exist).
func TestNewEventLoggerReturnsNilForUncreatablePath(t *testing.T) {
	l := newEventLogger(filepath.Join("/nonexistent-dir", "events.jsonl"))
	if l != nil {
		t.Error("expected nil logger for an uncreatable path")
		defer l.close()
	}
}

// parseArgs returns false for an empty string or non-object JSON.
func TestParseArgsEdgeCases(t *testing.T) {
	if _, ok := parseArgs(""); ok {
		t.Error("empty string should not parse")
	}

	if _, ok := parseArgs("   "); ok {
		t.Error("whitespace should not parse")
	}

	// A JSON array is valid JSON but not a map.
	if _, ok := parseArgs("[1,2,3]"); ok {
		t.Error("a JSON array should not parse as a map")
	}

	// A valid object.
	m, ok := parseArgs(`{"key":"value"}`)
	if !ok || m["key"] != "value" {
		t.Errorf("valid object parse failed: %v, %v", m, ok)
	}
}
