package agent

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusWriterWritesAndUpdates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.json")

	w := newStatusWriter(path, Status{State: "running", Model: "glm-5.2", MaxIterations: 42})
	if w == nil {
		t.Fatal("expected a writer for a non-empty path")
	}

	read := func() Status {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read status: %v", err)
		}
		var s Status
		if err := json.Unmarshal(data, &s); err != nil {
			t.Fatalf("parse status: %v", err)
		}
		return s
	}

	if s := read(); s.State != "running" || s.Model != "glm-5.2" || s.MaxIterations != 42 {
		t.Fatalf("initial status wrong: %+v", s)
	}

	w.update(func(s *Status) { s.Iteration = 7; s.Tool = "exec" })
	s := read()
	if s.Iteration != 7 || s.Tool != "exec" {
		t.Errorf("after update: %+v", s)
	}
	if s.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be stamped on update")
	}
}

func TestStatusWriterEmptyPathIsNoop(t *testing.T) {
	w := newStatusWriter("", Status{State: "running"})
	if w != nil {
		t.Fatal("empty path should yield a nil writer")
	}
	// A nil writer must be safe to use.
	w.update(func(s *Status) { s.Iteration = 1 })
}

func TestEventLoggerAppendsJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	l := newEventLogger(path)
	if l == nil {
		t.Fatal("expected a logger")
	}
	l.log("iteration", map[string]interface{}{"iteration": 1})
	l.log("tool_start", map[string]interface{}{"tool": "exec"})
	l.close()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()

	var lines int
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines++
		var rec map[string]interface{}
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("line %d not JSON: %v", lines, err)
		}
		if rec["event"] == nil || rec["ts"] == nil {
			t.Errorf("line %d missing event/ts: %v", lines, rec)
		}
	}
	if lines != 2 {
		t.Errorf("got %d lines, want 2", lines)
	}
}

func TestEventLoggerNilIsNoop(t *testing.T) {
	var l *eventLogger
	l.log("x", nil) // must not panic
	l.close()
}

func TestNewRunIDHasPID(t *testing.T) {
	id := NewRunID()
	if !strings.Contains(id, "-") || strings.TrimSpace(id) == "" {
		t.Errorf("unexpected run id %q", id)
	}
}

func TestSummarizeArgs(t *testing.T) {
	if got := summarizeArgs(map[string]interface{}{"command": "nmap -sV host"}); got != "nmap -sV host" {
		t.Errorf("command summary = %q", got)
	}
	if got := summarizeArgs(nil); got != "" {
		t.Errorf("nil args = %q, want empty", got)
	}
	long := strings.Repeat("a", 300)
	if got := summarizeArgs(map[string]interface{}{"path": long}); len(got) > 130 || !strings.HasSuffix(got, "…") {
		t.Errorf("long arg not truncated: len=%d", len(got))
	}
}
