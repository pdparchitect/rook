// Package agent wires Rook's autonomous security agent on top of the zot
// engine. It loads the embedded skill catalog, layers on any skills the user
// has in ~/.config/rook/skills, registers the default file and shell tools, and
// drives the agent loop until it exits.
//
// The engine runs in this process and talks straight to a model provider, so a
// run needs nothing but a provider key - no hosted service sits between Rook
// and the model.
package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	rook "github.com/pdparchitect/rook"
	"github.com/pdparchitect/rook/internal/config"
	"github.com/pdparchitect/rook/internal/objective"

	"github.com/charmbracelet/lipgloss"
	"github.com/openzot/openzot/agent"
	"github.com/openzot/openzot/session"
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
	Accent:    lipgloss.Color("#DC2626"), // red-600
	Secondary: lipgloss.Color("#F87171"), // red-400
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
	// Objective is the mission brief the run is dispatched from. Its task text
	// (objective + success criteria + rules of engagement) is placed in the
	// system prompt.
	Objective objective.Objective
	// Verbose prints each token as it streams in addition to tool activity.
	Verbose bool
	// RunDir is the base directory under which each run writes its artifacts: a
	// per-run subdirectory holding status.json (live snapshot) and events.jsonl
	// (append-only log). Empty disables artifacts. The caller resolves the base
	// path; Run creates the per-run subdirectory.
	RunDir string
	// SessionDir is where the run's session log is written (JSON Lines), so it
	// can be inspected afterwards and resumed. Empty disables session recording.
	SessionDir string
	// Resume seeds the conversation from an earlier session, so a run that ran
	// out of budget or died overnight continues rather than starting again.
	Resume *session.Session
}

// Run loads the embedded skills, builds the agent, and streams its execution
// to stdout. It returns the agent's exit code and the run's recorded outcome
// (the stop reason and summary the terminal tool carried), so a batch can
// record the result and a draft run can collect its deliverable.
func Run(ctx context.Context, cfg Config) (int, tui.Outcome, error) {
	subFS, err := fs.Sub(rook.SkillsFS, "skills")
	if err != nil {
		return 1, tui.Outcome{}, fmt.Errorf("open embedded skills: %w", err)
	}

	skillsResult, err := agent.LoadSkillsFromFS(subFS)
	if err != nil {
		return 1, tui.Outcome{}, fmt.Errorf("load embedded skills: %w", err)
	}

	// The embedded catalog is the baseline - the one skill that tells the agent
	// where collections live and how to fetch them. Anything in the user's
	// skills directory layers on top (winning name clashes) and is rescanned
	// every iteration, so a skill placed there mid-run - by the operator, or by
	// the agent itself cloning a collection the catalog names - surfaces on the
	// agent's next turn.
	loader := agent.NewSkillLoader(skillsResult, config.SkillsDir())

	skills := loader.Skills()

	fmt.Fprintf(os.Stderr, "Loaded %d skill(s):\n", len(skills))
	for _, s := range skills {
		fmt.Fprintf(os.Stderr, "  • %s - %s\n", s.Name, s.Description)
	}
	fmt.Fprintln(os.Stderr)

	instructions := config.Backstory

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
		return 1, tui.Outcome{}, err
	}

	// The embedded catalog ships inside the binary, so it has no filesystem
	// path. The `read` tool resolves its embedded-skill:// URL against these
	// contents, so the agent reads the catalog exactly as it reads a skill it
	// cloned onto disk - one tool, one verb, the path the only difference.
	tools := agent.DefaultToolsFor(agent.ToolOptions{
		EmbeddedSkills: skillsResult.EmbeddedContents(),
	})

	// The task is the durable objective and goes into the system prompt; the
	// opening user message only has to get the agent moving. This is what keeps
	// the objective in context however long the run grows - the instructions are
	// never summarised and always ordered first.
	task := cfg.Objective.Task()

	opts := agent.ExecuteWithToolsOptions{
		Instructions:  instructions + "\n\n## Your objective\n\n" + task,
		Tools:         tools,
		Skills:        loader.Skills,
		MaxIterations: cfg.MaxIterations,

		// Settle mode: a run ends only when the agent records an outcome, never
		// because its prose happened to sound conclusive. Rook is unattended by
		// design - nobody is watching to judge whether "I have finished the
		// audit" actually means finished - so an unambiguous ending matters more
		// here than almost anywhere. A positive value enables it.
		MaxSettles: defaultMaxSettles,
	}

	// A resumed run replays the earlier conversation, so the agent picks up with
	// everything it already knew rather than rediscovering it. The objective is
	// in the system prompt; the replayed messages carry what the agent did.
	if cfg.Resume != nil {
		opts.Messages = cfg.Resume.AgentMessages()
	}

	// The opening user message gets the agent moving. The objective is already
	// in the system prompt; this only has to kick it off. A resumed run points
	// back at the gap between its plan and the tree, not at a fresh start.
	kickoff := "Begin working on your objective. Start by calling the plan tool to lay out your approach, then carry it through to completion."
	if cfg.Resume != nil {
		kickoff = "Continue your investigation from where the session left off. Reconcile your plan with the current state of the target, then carry the objective through to completion."
	}

	opts.Messages = append(opts.Messages, agent.Message{
		Type: agent.TypeUser,
		Text: kickoff,
	})

	// Every run writes artifacts - a live status snapshot and an append-only
	// event log - into its own directory, so concurrent runs never collide and
	// the desktop widget can render the active run. A composite recorder fans
	// the engine's event stream to both the artifact recorder and, when session
	// recording is enabled, the session log.
	var recorder agent.Recorder

	// Captures the run's final totals for the end-of-run digest; the viewer's
	// returned Outcome carries only the reason and message.
	summaryRec := &agent.SummaryRecorder{}

	var sessionID string

	var runDir string

	if cfg.RunDir != "" {
		runDir = filepath.Join(cfg.RunDir, NewRunID())
		if err := os.MkdirAll(runDir, 0o700); err == nil {
			rec := &artifactRecorder{
				status: newStatusWriter(filepath.Join(runDir, "status.json"), Status{
					State:         "running",
					Model:         cfg.Model,
					MaxIterations: cfg.MaxIterations,
					StartedAt:     time.Now(),
				}),
				log: newEventLogger(filepath.Join(runDir, "events.jsonl")),
			}
			defer rec.close()
			recorder = rec
			fmt.Fprintf(os.Stderr, "Run artifacts: %s\n\n", runDir)
		}
	}

	// Session recording: the conversation log is what makes a run resumable and
	// what turns "it failed overnight" into something answerable. A log that
	// cannot be opened is reported but not fatal: the run is the point.
	if cfg.SessionDir != "" {
		meta := session.Meta{
			Task:     task,
			Model:    cfg.Model,
			Provider: cfg.Backend,
			Driver:   client.Provider(),
			Workdir:  "",
		}

		if cfg.Resume != nil {
			meta.ResumedFrom = cfg.Resume.Meta.ID
		}

		writer, err := session.Start(cfg.SessionDir, time.Now(), meta)
		if err != nil {
			fmt.Fprintf(os.Stderr, "rook: session log unavailable: %v\n", err)
		} else {
			defer writer.Close()

			fmt.Fprintf(os.Stderr, "Session log: %s\n", writer.Path())

			sessionID = writer.ID()

			sesRec := session.NewRecorder(writer)
			if recorder == nil {
				recorder = sesRec
			} else {
				recorder = &multiRecorder{recorder, sesRec}
			}
		}
	}

	// The summary capture always rides along, so the digest has the run's totals
	// whether or not artifacts or a session log were enabled.
	opts.Recorder = agent.MultiRecorder(recorder, summaryRec)

	workdir, _ := os.Getwd()

	// The viewer is zot's shared TUI package, themed for Rook: a red identity, the
	// "rook" name in the badge, and the run's iteration cap shown as progress. It
	// renders the full-screen view on a terminal and streams plain text otherwise
	// - or when --verbose is set, so the reasoning tokens stay in a pipe or log.
	outcome, err := tui.Run(ctx, client, tui.Meta{
		AppName:       "rook",
		Task:          task,
		Title:         cfg.Objective.DisplayTitle(),
		Model:         cfg.Model,
		Provider:      cfg.Backend,
		Workdir:       workdir,
		Plain:         cfg.Verbose,
		Theme:         rookTheme,
		MaxIterations: cfg.MaxIterations,
	}, opts)

	// The end-of-run digest: the outcome, what the run spent, and - when a
	// session was recorded - its id and the exact command to resume it. Printed
	// after the viewer restores the terminal, so it survives on the main screen
	// where the alt-screen stats did not.
	printDigest(os.Stderr, "rook", sessionID, outcome, summaryRec.Summary)

	// tui.Run reports an agent-declared failure (a _failure outcome) as an
	// *AgentExitError: that is a completed run with a nonzero exit code, not a
	// runner error, so surface the code with no error. Any other error is a real
	// run failure (a provider or setup problem) and maps to exit 1.
	var exitErr *tui.AgentExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code, outcome, nil
	}
	if err != nil {
		return 1, outcome, err
	}

	return 0, outcome, nil
}

// printDigest writes the end-of-run digest: the outcome, what the run spent,
// and - when the run was recorded - the session id and the exact command that
// resumes it. Kept to stderr so it never mixes into a piped deliverable, and
// skipped when there is nothing to say (a setup failure before the first turn
// leaves no summary).
func printDigest(w io.Writer, app, sessionID string, outcome tui.Outcome, summary *agent.Summary) {
	if summary == nil {
		return
	}

	digest := tui.Digest{
		Status:       tui.DigestStatus(summary.Reason, summary.Code),
		Session:      sessionID,
		Iterations:   summary.Iterations,
		Calls:        summary.Calls,
		InputTokens:  summary.InputTokens,
		OutputTokens: summary.OutputTokens,
		Message:      outcome.Message,
	}

	if sessionID != "" {
		digest.Resume = app + " --resume " + sessionID
	}

	fmt.Fprintf(w, "\n%s", tui.RenderDigest(digest))
}

// multiRecorder fans every recording call to two recorders: the artifact
// recorder (status.json + events.jsonl for the live widget) and the session
// recorder (JSONL conversation log for resume and analysis). The engine takes a
// single Recorder, so a composite is how both are driven from one event
// stream. Errors from either are ignored: recording is best-effort, and a
// failure in one sink must not silence the other. A nil recorder on either side
// is safe: it is skipped rather than called.
type multiRecorder struct {
	primary   agent.Recorder
	secondary agent.Recorder
}

func (m *multiRecorder) RecordMessage(msg agent.Message) error {
	if m.primary != nil {
		_ = m.primary.RecordMessage(msg)
	}
	if m.secondary != nil {
		_ = m.secondary.RecordMessage(msg)
	}
	return nil
}

func (m *multiRecorder) RecordEvent(kind, tool, text string, iteration int) error {
	if m.primary != nil {
		_ = m.primary.RecordEvent(kind, tool, text, iteration)
	}
	if m.secondary != nil {
		_ = m.secondary.RecordEvent(kind, tool, text, iteration)
	}
	return nil
}

func (m *multiRecorder) RecordResult(summary agent.Summary) error {
	if m.primary != nil {
		_ = m.primary.RecordResult(summary)
	}
	if m.secondary != nil {
		_ = m.secondary.RecordResult(summary)
	}
	return nil
}

func (m *multiRecorder) RecordFailure(f *agent.Failure) error {
	if m.primary != nil {
		_ = m.primary.RecordFailure(f)
	}
	if m.secondary != nil {
		_ = m.secondary.RecordFailure(f)
	}
	return nil
}

func (m *multiRecorder) RecordReset() error {
	if m.primary != nil {
		_ = m.primary.RecordReset()
	}
	if m.secondary != nil {
		_ = m.secondary.RecordReset()
	}
	return nil
}
