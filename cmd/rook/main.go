// Command rook is an AI bug-hunting harness for vulnerability research, bug
// hunting and source-code auditing. It runs the zot autonomous engine
// in-process and ships lean: the only embedded skill is a catalog that points
// the agent at external skill collections it fetches on demand.
//
// rook takes objectives, not prompts. An objective is a small YAML file - the
// durable goal, the success criteria that define "done", and the rules of
// engagement the work must hold to - and each objective becomes one autonomous
// run: the agent reads files, runs commands and investigates on its own while
// the terminal streams a live, read-only view of everything it does.
//
// Usage:
//
//	export ZAI_API_KEY="[redacted]"
//
//	# write an objective, then run it - objectives and their records live under .rook/
//	rook new "Audit the HTTP handlers in ./server for injection bugs"
//	rook
//
//	# a bare rook runs every outstanding objective in the dossier, in filename
//	# order, skipping what the ledger already records as done; naming objectives
//	# runs exactly those
//	rook .rook/objectives/audit-the-http-handlers-in-server-for-injec.yaml
//
//	# every run writes artifacts (status + events) and a ledger receipt
//	rook .rook/objectives/hunt.yaml
//
//	rook config          # edit the config (backend, model, key)
//	rook version
//
// Configuration is layered: built-in defaults < config file < ROOK_* env vars <
// CLI flags. The config file is optional and lives at ~/.config/rook/config.yaml
// (override with $ROOK_CONFIG or --config). See configs/rook.example.yaml.
// rook is intended for authorized security testing only. Only run it against
// systems, code and services you own or are explicitly authorized to assess.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/pflag"

	rook "github.com/pdparchitect/rook"
	"github.com/pdparchitect/rook/internal/agent"
	"github.com/pdparchitect/rook/internal/buildinfo"
	"github.com/pdparchitect/rook/internal/config"
	"github.com/pdparchitect/rook/internal/objective"
	"github.com/pdparchitect/rook/internal/version"

	"github.com/openzot/openzot/session"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "rook: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	// `rook config` opens the config file in $EDITOR, seeding it from the
	// embedded template on first run. `rook config path` prints its location.
	if len(os.Args) > 1 && os.Args[1] == "config" {
		if len(os.Args) > 2 && os.Args[2] == "path" {
			fmt.Println(config.DefaultConfigPath())
			return nil
		}
		return editConfig()
	}

	// `rook new` scaffolds an objective. The two-step shape is deliberate: the
	// pause between writing the objective and running it is where success
	// criteria get written, and it is what keeps rook from feeling like a
	// prompt box.
	if len(os.Args) > 1 && os.Args[1] == "new" {
		return newObjective(os.Args[2:], os.Stdout)
	}

	flags := pflag.NewFlagSet("rook", pflag.ContinueOnError)
	configPath := flags.String("config", "", "path to the config file (default: $ROOK_CONFIG or ~/.config/rook/config.yaml)")
	backend := flags.String("backend", "", "backend to target: a provider such as zai (default), openai, anthropic, groq, ollama, or a backend named in the config")
	model := flags.String("model", "", "model the agent reasons with (overrides config)")
	dir := flags.String("dir", ".", "working directory the agent investigates: the objective runs against this tree")
	maxIter := flags.Int("max-iterations", 0, "maximum agent iterations before forced stop (overrides config)")
	runDir := flags.String("run-dir", "", "base directory for per-run artifacts (default: $ROOK_RUN_DIR or ~/.local/state/rook/runs)")
	sessionDir := flags.String("session-dir", "", "where session logs are written (default: ./.rook/sessions)")
	noSession := flags.Bool("no-session", false, "do not record a session log for this run")
	resume := flags.String("resume", "", "continue an earlier session: an id, a path, or \"last\"")
	objectivesFlag := flags.String("objectives-dir", "", "where this project's objectives live, run by a bare `rook` (default: ./.rook/objectives)")
	recordsDir := flags.String("records-dir", "", "where run records are written (default: ./.rook/records)")
	watchFlag := flags.Bool("watch", false, "stay up and run objectives as they arrive, instead of running once and exiting: bare --watch watches this project's objectives directory, or name a folder to watch instead")
	rerun := flags.Bool("rerun", false, "run objectives even when the ledger already records a successful run of the same content")
	verbose := flags.BoolP("verbose", "v", false, "stream plain output (with reasoning tokens) instead of the full-screen viewer")
	showVersion := flags.BoolP("version", "V", false, "print version and exit")

	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "rook - AI bug-hunting harness\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  rook [flags] [objective...]\n  rook new \"the objective\"\n  rook sessions\n  rook config\n  rook version\n\n")
		fmt.Fprintf(os.Stderr, "A bare `rook` runs every outstanding objective in .rook/objectives/.\n\nFlags:\n")
		flags.PrintDefaults()
	}

	// Allow `rook version` as a subcommand in addition to the --version flag.
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "version" {
		printVersion()
		return nil
	}

	// `rook sessions` lists what previous runs left behind. Its own subcommand
	// because it takes no objective and produces no run.
	if len(args) > 0 && args[0] == "sessions" {
		return listSessions(args[1:])
	}

	if err := flags.Parse(args); err != nil {
		os.Exit(2)
	}

	if *showVersion {
		printVersion()
		return nil
	}

	// A .env in the working directory is read only on a developer build. Rook
	// runs shell commands against targets with a provider key in the process, so
	// a released binary must not take credentials from whatever directory it was
	// pointed at - a stray committed .env in the code under review would
	// otherwise reach the process about to run commands against it. Released
	// builds take credentials from the config file and the real environment.
	if buildinfo.Dev {
		godotenv.Load()
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	// CLI flags win over file and env.
	if *backend != "" {
		cfg.DefaultBackend = *backend
	}
	if *model != "" {
		cfg.Agent.Model = *model
	}
	if *maxIter > 0 {
		cfg.Agent.MaxIterations = *maxIter
	}

	if err := cfg.Validate(); err != nil {
		return err
	}

	selected, err := cfg.Selected()
	if err != nil {
		return err
	}

	// Every run writes artifacts (status + log) under a base run directory:
	// flag, then config, then the built-in XDG default.
	resolvedRunDir := firstNonEmpty(*runDir, cfg.RunDir, config.DefaultRunDir())
	resolvedRunDir = expandPath(resolvedRunDir)

	// The objectives directory: what a bare `rook` runs and what a bare
	// --watch watches. Defaults to ./.rook/objectives. Resolved against the
	// invoking directory (before the chdir into --dir) so a relative path means
	// what was typed.
	objectivesRoot := *objectivesFlag
	if objectivesRoot == "" {
		objectivesRoot = objective.ObjectivesDir(".")
	}
	if abs, err := filepath.Abs(objectivesRoot); err == nil {
		objectivesRoot = abs
	}

	// The ledger: where run records are written. Defaults to ./.rook/records.
	records := *recordsDir
	if records == "" {
		records = objective.RecordsDir(".")
	}
	if abs, err := filepath.Abs(records); err == nil {
		records = abs
	}
	ledger := objective.Ledger{Root: records}

	// Session logs: where the run's conversation is recorded for inspection and
	// resume. Defaults to ./.rook/sessions, resolved before the chdir.
	sessions := *sessionDir
	if sessions == "" {
		sessions = config.DefaultSessionDir()
	}
	if abs, err := filepath.Abs(sessions); err == nil {
		sessions = abs
	}

	// A resumed run continues the session its reference names, rather than
	// starting from scratch. --watch runs objectives as they arrive, so a resume
	// (which continues one specific session) is a different shape.
	var resumed *session.Session

	if *resume != "" {
		if *watchFlag {
			return fmt.Errorf("--watch runs objectives as they arrive; --resume continues one specific session - use one, not both")
		}

		path, err := session.Resolve(sessions, *resume)
		if err != nil {
			return err
		}

		resumed, err = session.Load(path)
		if err != nil {
			return fmt.Errorf("read session: %w", err)
		}

		fmt.Fprintf(os.Stderr, "rook: resuming %s (%d messages)\n", resumed.Meta.ID, len(resumed.Messages))
	}

	if *noSession {
		sessions = ""
	}

	// Set the working directory before the agent starts. This is not a filesystem
	// sandbox: absolute paths and shell commands retain the process's host
	// permissions, so --dir is convenience, not containment.
	if err := os.Chdir(*dir); err != nil {
		return fmt.Errorf("cannot enter --dir %q: %w", *dir, err)
	}

	// Strip backend credentials from the environment before the agent runs, so
	// the commands it executes against a target cannot read them. The resolved
	// key is still handed to the client below.
	config.ScrubBackendSecrets(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Watch mode: stay up and run every objective the target yields, as it
	// arrives. One failed objective is one objective's story, not the watch's.
	if *watchFlag {
		return startWatch(ctx, objectivesRoot, runRunner{
			ctx:        ctx,
			cfg:        cfg,
			selected:   selected,
			runDir:     resolvedRunDir,
			sessionDir: sessions,
			ledger:     ledger,
			rerun:      *rerun,
			verbose:    *verbose,
		})
	}

	// A resumed run inherits its objective from the session it continues; it is
	// "keep going", never "start over", so no objective is named.
	if resumed != nil {
		runner := runRunner{
			ctx:        ctx,
			cfg:        cfg,
			selected:   selected,
			runDir:      resolvedRunDir,
			sessionDir:  sessions,
			ledger:      ledger,
			rerun:       *rerun,
			verbose:     *verbose,
			resume:      resumed,
		}

		return runner.execute(resumeObjective(resumed), false)
	}

	// Resolve the objectives to run: named on the command line, or - when none
	// are - everything in the project's objectives directory.
	paths, err := resolveObjectives(flags.Args(), objectivesRoot)
	if err != nil {
		return err
	}

	runner := runRunner{
		ctx:        ctx,
		cfg:        cfg,
		selected:   selected,
		runDir:     resolvedRunDir,
		sessionDir: sessions,
		ledger:     ledger,
		rerun:      *rerun,
		verbose:    *verbose,
	}

	for i, o := range paths {
		if len(paths) > 1 {
			fmt.Fprintf(os.Stderr, "rook: objective %d/%d: %s\n", i+1, len(paths), o.Path)
		}

		if err := runner.execute(o, i < len(paths)-1); err != nil {
			return err
		}
	}

	notifyUpdate(cfg.UpdateCheck.Disabled)
	return nil
}

// resumeObjective rebuilds an Objective from a resumed session's task text, so
// the run continues under the same contract it started with - the session
// carries the conversation, and the task text carries the objective.
func resumeObjective(s *session.Session) objective.Objective {
	return objective.FromText(s.Meta.Task)
}

// runRunner holds everything a single objective's run needs. The batch loop and
// watch mode both go through execute, so an objective runs identically however
// it was named: a fresh conversation, its own artifacts, its own recorded
// outcome.
type runRunner struct {
	ctx      context.Context
	cfg      config.Config
	selected config.Selection

	runDir     string
	sessionDir string
	verbose    bool

	ledger objective.Ledger
	rerun  bool

	// resume seeds the conversation from an earlier session, so a run that ran
	// out of budget or died overnight continues rather than starting again.
	resume *session.Session
}

// execute runs one objective as its own run and records how it ended.
//
// A satisfied objective is skipped, so restarting a batch - or pointing a watch
// at a folder whose work already happened - picks up where things left off
// instead of re-executing finished work; editing the objective changes its hash
// and re-queues it.
func (r runRunner) execute(o objective.Objective, quitOnDone bool) error {
	if record, done := r.ledger.Satisfied(o); done && !r.rerun {
		fmt.Fprintf(os.Stderr,
			"rook: objective %s already satisfied by run %s (%s); edit the objective or pass --rerun to run it again\n",
			o.Path, record.Run, record.At.Format("2006-01-02 15:04"))

		return nil
	}

	code, outcome, err := agent.Run(r.ctx, agent.Config{
		Provider:      r.selected.Provider,
		APIKey:        r.selected.APIKey,
		BaseURL:        r.selected.BaseURL,
		Model:          r.selected.Model,
		Backend:        r.cfg.DefaultBackend,
		MaxIterations:  r.selected.MaxIterations,
		Objective:      o,
		Verbose:        r.verbose,
		RunDir:         r.runDir,
		SessionDir:      r.sessionDir,
		Resume:          r.resume,
	})

	if err != nil {
		return err
	}

	// A successful run enters the ledger; a failed or aborted one does not, so
	// it runs again next time - failure is not doneness.
	if code == 0 {
		runID := time.Now().UTC().Format("20060102-150405") + "-" + fmt.Sprintf("%d", os.Getpid())
		// Evidence is read back from the run's status artifact if one was
		// written; otherwise the outcome summary from the viewer is the proof.
		proof := objective.EvidenceFromSummary(runID, &objective.ArtifactSummary{
			Reason:  outcome.Reason,
			Message: outcome.Message,
		}, nil)

		if err := r.ledger.Record(o, runID, "settled", time.Now(), proof); err != nil {
			fmt.Fprintf(os.Stderr, "rook: could not record the outcome for %s: %v\n", o.Path, err)
		}
	}

	return nil
}

// resolveObjectives loads the objectives this invocation is about: the ones
// named on the command line, or - when none are - everything in the project's
// own objectives directory.
func resolveObjectives(args []string, objectivesRoot string) ([]objective.Objective, error) {
	if len(args) == 0 {
		found, err := objective.List(objectivesRoot)
		if err != nil {
			return nil, err
		}

		if len(found) == 0 {
			flagsUsage()
			where := objectivesRoot + " holds none"
			return nil, fmt.Errorf("no objective given, and %s (write one with `rook new \"the objective\"`)", where)
		}

		fmt.Fprintf(os.Stderr, "rook: running %d objective(s) from %s\n", len(found), objectivesRoot)
		args = found
	}

	objectives := make([]objective.Objective, 0, len(args))

	for _, path := range args {
		loaded, err := objective.Load(path)
		if err != nil {
			// The retraining moment: someone typed prose where an objective file
			// goes. The error has to teach the new shape, not just report a
			// missing file.
			if _, statErr := os.Stat(path); statErr != nil && strings.ContainsAny(path, " \t") {
				return nil, fmt.Errorf("objectives are files, not prose - write the objective first:\n\n  rook new %q", path)
			}

			return nil, err
		}

		if abs, absErr := filepath.Abs(loaded.Path); absErr == nil {
			loaded.Path = abs
		}

		objectives = append(objectives, loaded)
	}

	return objectives, nil
}

// startWatch watches a directory for objectives and runs them as they arrive.
// It polls the directory at an interval rather than using inotify, because a
// cross-platform poll is simpler and the arrival rate of objectives is low.
func startWatch(ctx context.Context, target string, runner runRunner) error {
	fmt.Fprintf(os.Stderr, "rook: watching %s for objectives (Ctrl-C to stop)\n\n", target)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Track which objectives have already run in this watch session, by path.
	// The ledger handles cross-session doneness; this only prevents re-running
	// an objective that arrived and was already processed in this session.
	seen := map[string]bool{}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			paths, err := objective.List(target)
			if err != nil {
				continue
			}

			for _, path := range paths {
				if seen[path] {
					continue
				}
				seen[path] = true

				o, err := objective.Load(path)
				if err != nil {
					fmt.Fprintf(os.Stderr, "rook: skipping %s: %v\n", path, err)
					continue
				}

				if err := runner.execute(o, false); err != nil {
					fmt.Fprintf(os.Stderr, "rook: %s: %v\n", path, err)
				}
			}
		}
	}
}

// newObjective scaffolds an objective under ./.rook/objectives and says how to
// run it.
//
// Two levels of help, both landing in the same reviewable file: bare `rook new`
// writes the blank form, and `rook new "objective"` fills the objective in. The
// objective will not run until its objective and success criteria are filled in.
func newObjective(args []string, out io.Writer) error {
	set := pflag.NewFlagSet("new", pflag.ContinueOnError)
	dir := set.String("dir", ".", "working directory the objective is for: the scaffold lands under <dir>/.rook/objectives")
	objectivesFlag := set.String("objectives-dir", "", "where to write the objective (default: <dir>/.rook/objectives)")

	if err := set.Parse(args); err != nil {
		return err
	}

	objectiveText := strings.TrimSpace(strings.Join(set.Args(), " "))

	// The command is anchored at --dir: the scaffold lands in that project's
	// dossier. A relative path means what was typed here, in the invoking
	// directory, so it is joined before any chdir below.
	objectivesDir := *objectivesFlag
	if objectivesDir == "" {
		objectivesDir = objective.ObjectivesDir(*dir)
	}

	o := objective.Objective{Objective: objectiveText}

	edit := "edit its success criteria"
	if objectiveText == "" {
		edit = "edit its objective and success criteria"
	}

	path, err := objective.Scaffold(objectivesDir, o)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Scaffolded %s\n", path)
	fmt.Fprintf(out, "%s, then run:\n  rook %s\n", edit, path)

	return nil
}

func printVersion() {
	// the build kind is on the version line because it changes what the binary
	// reads from disk: "why is my .env ignored" should be answerable from here.
	fmt.Printf("rook %s (%s)\n", version.Version, buildinfo.Kind)
	// No config is loaded for `rook version`; the env switch still applies.
	notifyUpdate(false)
}

// editConfig ensures the config file exists - seeding it from the embedded
// template on first run - and opens it in the user's editor. This is the setup
// path: configure the backend, model and provider key by editing the file.
func editConfig() error {
	path := config.DefaultConfigPath()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, rook.ExampleConfigYAML, 0o600); err != nil {
			return fmt.Errorf("write config template: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Created %s from the template.\n", path)
	}

	editor := firstNonEmpty(os.Getenv("VISUAL"), os.Getenv("EDITOR"))
	if editor == "" {
		for _, candidate := range []string{"nano", "vi", "vim"} {
			if _, err := exec.LookPath(candidate); err == nil {
				editor = candidate
				break
			}
		}
	}
	if editor == "" {
		fmt.Println(path)
		return fmt.Errorf("no editor found; set $EDITOR and re-run (the config is at the path above)")
	}

	// Editors expect the real terminal; wire the standard streams through.
	cmd := exec.Command(editor, path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// expandPath expands a leading ~ and any $ENV references in a path.
func expandPath(p string) string {
	p = os.ExpandEnv(p)
	switch {
	case p == "~":
		if h, err := os.UserHomeDir(); err == nil {
			return h
		}
	case strings.HasPrefix(p, "~/"):
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}

// notifyUpdate prints a one-line notice to stderr when a newer release exists.
// It is silently skipped for dev builds and on any network error, and turned
// off entirely by update_check.disabled in the config or the
// ROOK_UPDATE_CHECK_DISABLED environment variable - the latter needing no
// config file, so it works on a locked-down host. The check is the one call a
// run makes that is not to the model provider.
func notifyUpdate(disabled bool) {
	if disabled || updateCheckDisabledByEnv() {
		return
	}
	result, err := version.Check()
	if err != nil {
		return
	}
	if notice := version.FormatUpdateNotice(result); notice != "" {
		fmt.Fprintf(os.Stderr, "\n%s\n", notice)
	}
}

// updateCheckDisabledByEnv reports whether ROOK_UPDATE_CHECK_DISABLED is set to
// a truthy value. The config loader already folds this variable into
// UpdateCheck.Disabled for any command that loads config; this covers the one
// that does not - `rook version` - so the switch works without a config file,
// which is the case that matters on an air-gapped or CI box. Same truthiness as
// the config loader (strconv.ParseBool) so both agree.
func updateCheckDisabledByEnv() bool {
	v, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("ROOK_UPDATE_CHECK_DISABLED")))
	return err == nil && v
}

// flagsUsage prints the flag set's usage to stderr. A helper because the flag set
// is local to run(), and resolveObjectives needs to print the same usage when the
// objectives directory is empty.
func flagsUsage() {
	fmt.Fprintf(os.Stderr, "rook - AI bug-hunting harness\n\n")
	fmt.Fprintf(os.Stderr, "Usage:\n  rook [flags] [objective...]\n  rook new \"the objective\"\n  rook sessions\n  rook config\n  rook version\n\n")
	fmt.Fprintf(os.Stderr, "A bare `rook` runs every outstanding objective in .rook/objectives/.\n\nRun `rook --help` for flags.\n")
}

// listSessions prints the session logs from the sessions directory, newest
// first: the id, the task, and whether the run finished. Its own subcommand
// because it takes no objective and produces no run - it is how someone finds
// what to resume.
func listSessions(args []string) error {
	set := pflag.NewFlagSet("sessions", pflag.ContinueOnError)
	dir := set.String("dir", config.DefaultSessionDir(), "session directory to list")

	if err := set.Parse(args); err != nil {
		return err
	}

	entries, err := session.List(*dir)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Fprintf(os.Stderr, "no sessions in %s\n", *dir)
		return nil
	}

	fmt.Fprintf(os.Stderr, "%-20s %-10s %s\n", "SESSION", "STATUS", "TASK")
	for _, e := range entries {
		status := "unfinished"
		if e.Complete {
			status = e.Reason
		}

		task := e.Task
		if len(task) > 60 {
			task = task[:57] + "..."
		}

		fmt.Printf("%-20s %-10s %s\n", e.ID, status, task)
	}

	fmt.Fprintf(os.Stderr, "\nresume with: rook --resume <id>\n")

	return nil
}
