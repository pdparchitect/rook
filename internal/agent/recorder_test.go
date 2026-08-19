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
