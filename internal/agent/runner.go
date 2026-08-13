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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	rook "github.com/pdparchitect/rook"
	"github.com/pdparchitect/rook/internal/config"

	"github.com/openzot/openzot/agent"
	"github.com/openzot/openzot/tui"
)

// defaultMaxSettles bounds how many times the engine nudges the agent to record
// an outcome before surfacing the run as unsettled. Positive so settle mode is
// on; the value is generous because a security run legitimately produces a long
// final report before it calls a terminal tool.
const defaultMaxSettles = 20

// rookTheme is Rook's identity in the shared viewer: red, distinct from zot's
// neutral slate and pion's blue. Only the accent is themed - the semantic status
// colours (running / done / failed) stay fixed across every embedding app.
var rookTheme = tui.Theme{
	Accent:    "#DC2626", // red-600
	Secondary: "#F87171", // red-400
}

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
	// Backend is the name of the backend the run targets (zai, openai, …), shown
	// in the viewer's header. Presentation only; the client is already resolved.
	Backend string
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

	instructions := fmt.Sprintf(config.Backstory, scope)

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

	opts := agent.ExecuteWithToolsOptions{
		Messages:      []agent.Message{{Type: agent.TypeUser, Text: cfg.Task}},
		Instructions:  instructions,
		Tools:         tools,
		Skills:        skills,
		MaxIterations: cfg.MaxIterations,

		// Settle mode: a run ends only when the agent records an outcome, never
		// because its prose happened to sound conclusive. Rook is unattended by
		// design - nobody is watching to judge whether "I have finished the
		// audit" actually means finished - so an unambiguous ending matters more
		// here than almost anywhere. A positive value enables it.
		MaxSettles: defaultMaxSettles,
	}

	// Every run writes artifacts - a live status snapshot and an append-only
	// event log - into its own directory, so concurrent runs never collide and
	// the desktop widget can render the active run. The recorder is handed to the
	// engine, which drives it from the same event stream that feeds the viewer:
	// rendering and recording no longer share a hand-rolled loop here.
	if cfg.RunDir != "" {
		runDir := filepath.Join(cfg.RunDir, NewRunID())
		if err := os.MkdirAll(runDir, 0o700); err == nil {
			rec := &artifactRecorder{
				status: newStatusWriter(filepath.Join(runDir, "status.json"), Status{
					State:         "running",
					Model:         cfg.Model,
					Scope:         strings.TrimSpace(cfg.Scope),
					MaxIterations: cfg.MaxIterations,
					StartedAt:     time.Now(),
				}),
				log: newEventLogger(filepath.Join(runDir, "events.jsonl")),
			}
			defer rec.close()
			opts.Recorder = rec
			fmt.Fprintf(os.Stderr, "Run artifacts: %s\n\n", runDir)
		}
	}

	workdir, _ := os.Getwd()

	// The viewer is zot's shared TUI package, themed for Rook: a red identity, the
	// "rook" name in the badge, and the run's iteration cap shown as progress. It
	// renders the full-screen view on a terminal and streams plain text otherwise
	// - or when --verbose is set, so the reasoning tokens stay in a pipe or log.
	err = tui.Run(ctx, client, tui.Meta{
		AppName:       "rook",
		Task:          cfg.Task,
		Model:         cfg.Model,
		Backend:       cfg.Backend,
		Workdir:       workdir,
		Plain:         cfg.Verbose,
		Theme:         rookTheme,
		MaxIterations: cfg.MaxIterations,
	}, opts)

	// tui.Run reports an agent-declared failure (a _failure outcome) as an
	// *AgentExitError: that is a completed run with a nonzero exit code, not a
	// runner error, so surface the code with no error. Any other error is a real
	// run failure (a provider or setup problem) and maps to exit 1.
	var exitErr *tui.AgentExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code, nil
	}
	if err != nil {
		return 1, err
	}
	return 0, nil
}
