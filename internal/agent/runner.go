// Package agent wires Rook's autonomous security agent on top of the zot
// engine. It loads the embedded skill library, registers the default file and
// shell tools, and drives the agent loop until it exits.
//
// The engine runs in this process and talks straight to a model provider, so a
// run needs nothing but a provider key - no hosted service sits between Rook
// and the model.
package agent

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	rook "github.com/pdparchitect/rook"
	"github.com/pdparchitect/rook/internal/config"

	"github.com/openzot/openzot/agent"
)

// defaultMaxSettles bounds how many times the engine nudges the agent to record
// an outcome before surfacing the run as unsettled. Positive so settle mode is
// on; the value is generous because a security run legitimately produces a long
// final report before it calls a terminal tool.
const defaultMaxSettles = 20

// Config controls a single autonomous run.
type Config struct {
	// Provider names the model provider to call: "openai", "anthropic", "zai"
	// and so on, or "custom" with a BaseURL for anything else that speaks the
	// OpenAI-compatible API.
	Provider string
	// APIKey is the provider credential.
	APIKey string
	// BaseURL overrides the provider's default endpoint. Required for a custom
	// provider, ignored otherwise unless a gateway needs it.
	BaseURL string
	// Model is the model the agent reasons with, as the provider names it.
	Model string
	// MaxIterations bounds how many tool-using turns the agent may take
	// before it is forced to stop.
	MaxIterations int
	// Task is the objective the operator hands to the agent.
	Task string
	// Scope is the explicit authorization boundary (hosts, repos, paths) the
	// agent must stay within. It is injected into the backstory.
	Scope string
	// Verbose prints each token as it streams in addition to tool activity.
	Verbose bool
	// RunDir is the base directory under which each run writes its artifacts: a
	// per-run subdirectory holding status.json (live snapshot) and events.jsonl
	// (append-only log). Empty disables artifacts. The caller resolves the base
	// path; Run creates the per-run subdirectory.
	RunDir string
}

// Run loads the embedded skills, builds the agent, and streams its execution
// to stdout. It returns the agent's exit code.
func Run(ctx context.Context, cfg Config) (int, error) {
	subFS, err := fs.Sub(rook.SkillsFS, "skills")
	if err != nil {
		return 1, fmt.Errorf("open embedded skills: %w", err)
	}

	skillsResult, err := agent.LoadSkillsFromFS(subFS)
	if err != nil {
		return 1, fmt.Errorf("load embedded skills: %w", err)
	}

	skills := skillsResult.Skills

	fmt.Fprintf(os.Stderr, "Loaded %d embedded skill(s):\n", len(skills))
	for _, s := range skills {
		fmt.Fprintf(os.Stderr, "  • %s - %s\n", s.Name, s.Description)
	}
	fmt.Fprintln(os.Stderr)

	scope := strings.TrimSpace(cfg.Scope)
	if scope == "" {
		scope = "Authorized scope: not specified. Treat the current working " +
			"directory as the only target and do not reach out to remote systems."
	} else {
		scope = "Authorized scope:\n" + scope
	}

	backstory := fmt.Sprintf(config.Backstory, scope)

	// The model is a property of the client rather than of a run: it decides
	// which endpoint and which tokenizer the engine uses, so it has to be known
	// before the conversation starts.
	client, err := agent.NewClient(agent.ClientOptions{
		Provider: cfg.Provider,
		Model:    cfg.Model,
		APIKey:   cfg.APIKey,
		BaseURL:  cfg.BaseURL,
	})
	if err != nil {
		return 1, err
	}

	tools := agent.DefaultTools()

	// Embedded skills are served by a tool rather than read off disk, because
	// Rook ships its skill library inside the binary - there is no path for the
	// agent to read. The engine describes them in the backstory either way.
	if skillTool := skillsResult.Tool(); skillTool != nil {
		tools["skill"] = *skillTool
	}

	events, errs := agent.ExecuteWithTools(ctx, client, agent.ExecuteWithToolsOptions{
		Messages:      []agent.Message{{Type: agent.TypeUser, Text: cfg.Task}},
		Backstory:     backstory,
		Tools:         tools,
		Skills:        skills,
		MaxIterations: cfg.MaxIterations,

		// Settle mode: a run ends only when the agent records an outcome, never
		// because its prose happened to sound conclusive. Rook is unattended by
		// design - nobody is watching to judge whether "I have finished the
		// audit" actually means finished - so an unambiguous ending matters more
		// here than almost anywhere. A positive value enables it.
		MaxSettles: defaultMaxSettles,
	})

	// Every run writes artifacts - a live status snapshot and an append-only
	// event log - into its own directory, so concurrent runs never collide and
	// the desktop widget can render the active run.
	var (
		status *statusWriter
		logf   *eventLogger
	)
	if cfg.RunDir != "" {
		runDir := filepath.Join(cfg.RunDir, NewRunID())
		if err := os.MkdirAll(runDir, 0o700); err == nil {
			status = newStatusWriter(filepath.Join(runDir, "status.json"), Status{
				State:         "running",
				Model:         cfg.Model,
				Scope:         strings.TrimSpace(cfg.Scope),
				MaxIterations: cfg.MaxIterations,
				StartedAt:     time.Now(),
			})
			logf = newEventLogger(filepath.Join(runDir, "events.jsonl"))
			fmt.Fprintf(os.Stderr, "Run artifacts: %s\n\n", runDir)
		}
	}
	defer logf.close()

	exitCode := 0

	for event := range events {
		switch e := event.(type) {
		case agent.TokenAgentEvent:
			if cfg.Verbose {
				fmt.Print(e.Token)
			}
		case agent.IterationEvent:
			fmt.Fprintf(os.Stderr, "\n--- Iteration %d ---\n", e.Iteration)
			status.update(func(s *Status) { s.Iteration = e.Iteration })
			logf.log("iteration", map[string]interface{}{"iteration": e.Iteration})
		case agent.ToolCallStartEvent:
			fmt.Fprintf(os.Stderr, "\n[%s] %v\n", e.Name, e.Args)
			status.update(func(s *Status) {
				s.Tool = e.Name
				s.Action = summarizeArgs(e.Args)
			})
			logf.log("tool_start", map[string]interface{}{"tool": e.Name, "args": e.Args})
		case agent.ToolCallEndEvent:
			fmt.Fprintf(os.Stderr, "[%s] → %v\n", e.Name, truncate(e.Result))
			logf.log("tool_end", map[string]interface{}{"tool": e.Name})
		case agent.ToolCallErrorEvent:
			fmt.Fprintf(os.Stderr, "[%s] error: %s\n", e.Name, e.Error)
			logf.log("tool_error", map[string]interface{}{"tool": e.Name, "error": e.Error})
		case agent.AgentExitEvent:
			fmt.Fprintf(os.Stderr, "\n\n=== Agent exited with code %d ===\n", e.Code)
			if e.Message != "" {
				fmt.Fprintf(os.Stderr, "Message: %s\n", e.Message)
			}
			exitCode = e.Code
			code := e.Code
			status.update(func(s *Status) {
				s.Tool = ""
				s.Action = ""
				s.ExitCode = &code
				if code == 0 {
					s.State = "done"
				} else {
					s.State = "error"
				}
			})
			logf.log("exit", map[string]interface{}{"code": e.Code, "message": e.Message})
		}
	}

	if err := <-errs; err != nil {
		status.update(func(s *Status) { s.State = "error" })
		logf.log("error", map[string]interface{}{"error": err.Error()})
		return 1, err
	}

	return exitCode, nil
}

// truncate shortens long string fields in a tool result for terminal display.
func truncate(result interface{}) interface{} {
	m, ok := result.(map[string]interface{})
	if !ok {
		return result
	}
	const limit = 200
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok && len(s) > limit {
			out[k] = s[:limit] + "… (truncated)"
			continue
		}
		out[k] = v
	}
	return out
}
