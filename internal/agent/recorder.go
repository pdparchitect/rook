package agent

import (
	"encoding/json"
	"strings"

	"github.com/openzot/openzot/agent"
)

// artifactRecorder persists a run's activity to the two on-disk artifacts Rook
// keeps per run: a live status.json snapshot and an append-only events.jsonl
// log. It implements agent.Recorder, so the engine drives it from the same event
// stream that feeds the viewer - rendering (the TUI) and recording (these files)
// no longer share a hand-rolled switch statement.
//
// Every method is best-effort: statusWriter and eventLogger are nil-safe (a run
// with no run directory records nothing), and the Recorder contract already
// ignores returned errors, so a logging failure never ends a run.
type artifactRecorder struct {
	status *statusWriter
	log    *eventLogger
}

// RecordMessage is unused: Rook's artifacts are an event log and a status
// snapshot, not a message transcript.
func (r *artifactRecorder) RecordMessage(agent.Message) error { return nil }

// RecordEvent maps the engine's event kinds onto the status snapshot and the
// JSONL log. The kinds are the loop's own (camelCase) names; token and message
// events carry nothing an artifact consumer needs, so they fall through.
func (r *artifactRecorder) RecordEvent(kind, tool, text string, iteration int) error {
	switch kind {
	case "iteration":
		r.status.update(func(s *Status) { s.Iteration = iteration })
		r.log.log("iteration", map[string]interface{}{"iteration": iteration})
	case "toolCallStart":
		// text is the tool's raw JSON argument string (see the loop's
		// EventToolCallStart); summarize it for the one-line status action and log
		// the parsed arguments when they parse as an object.
		action := actionFromArgs(text)
		r.status.update(func(s *Status) {
			s.Tool = tool
			s.Action = action
		})
		r.log.log("tool_start", map[string]interface{}{"tool": tool, "args": rawArgs(text)})
	case "toolCallEnd":
		r.log.log("tool_end", map[string]interface{}{"tool": tool})
	case "toolCallError":
		r.log.log("tool_error", map[string]interface{}{"tool": tool, "error": text})
	}
	return nil
}

// RecordResult records how the run ended: the terminal status state and exit
// code, and a final "exit" log line.
func (r *artifactRecorder) RecordResult(summary agent.Summary) error {
	code := summary.Code
	r.status.update(func(s *Status) {
		s.Tool = ""
		s.Action = ""
		s.ExitCode = &code
		if code == 0 {
			s.State = "done"
		} else {
			s.State = "error"
		}
	})
	r.log.log("exit", map[string]interface{}{
		"code":    summary.Code,
		"reason":  summary.Reason,
		"message": summary.Message,
	})
	return nil
}

// RecordReset is a no-op: the artifacts are an append-only record of what
// actually happened during the run, so a history rewrite (compaction) inside the
// engine leaves them untouched.
func (r *artifactRecorder) RecordReset() error { return nil }

// close releases the underlying log file. Safe on a zero-value recorder.
func (r *artifactRecorder) close() {
	if r != nil {
		r.log.close()
	}
}

// actionFromArgs renders a tool call's raw JSON argument string as the short,
// one-line status action, reusing summarizeArgs when the arguments parse as an
// object and clipping the raw text otherwise.
func actionFromArgs(raw string) string {
	if m, ok := parseArgs(raw); ok {
		return summarizeArgs(m)
	}
	return summarizeArgs(raw)
}

// rawArgs returns the parsed argument object for the JSONL log, or the raw string
// when it does not parse as one.
func rawArgs(raw string) interface{} {
	if m, ok := parseArgs(raw); ok {
		return m
	}
	return raw
}

func parseArgs(raw string) (map[string]interface{}, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, false
	}
	return m, true
}
