// Command rook is an AI bug-hunting harness for vulnerability research, bug
// hunting and source-code auditing. It runs the zot autonomous engine in-process
// and ships with an embedded library of security skills.
//
// Usage:
//
//	rook config                                 # edit the config (backend, model, key)
//	export ZAI_API_KEY="..."; rook "Audit the HTTP handlers in ./server for injection bugs"
//	rook --scope "repo: ./server, no network" "Hunt for auth bypasses"
//	rook --backend openai "..."                 # any built-in provider
//	rook version
//
// Configuration is layered: built-in defaults < config file < ROOK_* env vars <
// CLI flags. The config file is optional and lives at ~/.config/rook/config.yaml
// (override with $ROOK_CONFIG or --config). See configs/rook.example.yaml.
//
// Rook is intended for authorized security testing only. Always pass an
// explicit --scope describing the systems you are permitted to assess.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/pdparchitect/rook/internal/buildinfo"
	"github.com/spf13/pflag"

	rook "github.com/pdparchitect/rook"
	"github.com/pdparchitect/rook/internal/agent"
	"github.com/pdparchitect/rook/internal/config"
	"github.com/pdparchitect/rook/internal/version"
)

func main() {
	// A .env in the working directory is read only on a developer build. Rook
	// runs shell commands against targets with a provider key in the process, so
	// a released binary must not take credentials from whatever directory it was
	// pointed at - a stray committed .env in the code under review would
	// otherwise reach the process about to run commands against it. Released
	// builds take credentials from the config file and the real environment.
	if buildinfo.Dev {
		godotenv.Load()
	}

	flags := pflag.NewFlagSet("rook", pflag.ContinueOnError)
	configPath := flags.String("config", "", "path to the config file (default: $ROOK_CONFIG or ~/.config/rook/config.yaml)")
	backend := flags.String("backend", "", "backend to target: a provider such as zai (default), openai, anthropic, groq, ollama, or a backend named in the config")
	model := flags.String("model", "", "model the agent reasons with (overrides config)")
	maxIter := flags.Int("max-iterations", 0, "maximum agent iterations before forced stop (overrides config)")
	scope := flags.String("scope", "", "authorization boundary (hosts, repos, paths) the agent must stay within")
	scopeFile := flags.String("scope-file", "", "read the authorization scope from a file")
	runDir := flags.String("run-dir", "", "base directory for per-run artifacts (default: $ROOK_RUN_DIR or ~/.local/state/rook/runs)")
	verbose := flags.BoolP("verbose", "v", false, "stream the agent's reasoning tokens to stdout")
	showVersion := flags.BoolP("version", "V", false, "print version and exit")

	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "rook - AI bug-hunting harness\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  rook [flags] <task>\n  rook config          edit the config file in $EDITOR (creates it on first run)\n  rook version\n\nFlags:\n")
		flags.PrintDefaults()
	}

	// Allow `rook version` as a subcommand in addition to the --version flag.
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "version" {
		printVersion()
		return
	}

	// `rook config` opens the config file in $EDITOR, seeding it from the
	// embedded template on first run. `rook config path` prints its location.
	if len(args) > 0 && args[0] == "config" {
		if len(args) > 1 && args[1] == "path" {
			fmt.Println(config.DefaultConfigPath())
			return
		}
		if err := editConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := flags.Parse(args); err != nil {
		os.Exit(2)
	}

	if *showVersion {
		printVersion()
		return
	}

	task := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if task == "" {
		flags.Usage()
		os.Exit(2)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
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
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	selected, err := cfg.Selected()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	resolvedScope := *scope
	if *scopeFile != "" {
		data, err := os.ReadFile(*scopeFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: read scope file: %v\n", err)
			os.Exit(1)
		}
		resolvedScope = string(data)
	}

	// Every run writes artifacts (status + log) under a base run directory:
	// flag, then config, then the built-in XDG default.
	resolvedRunDir := firstNonEmpty(*runDir, cfg.RunDir, config.DefaultRunDir())
	resolvedRunDir = expandPath(resolvedRunDir)

	// Strip backend credentials from the environment before the agent runs, so
	// the commands it executes against a target cannot read them. The resolved
	// key is still handed to the client below.
	config.ScrubBackendSecrets(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	code, err := agent.Run(ctx, agent.Config{
		Provider:      selected.Provider,
		APIKey:        selected.APIKey,
		BaseURL:       selected.BaseURL,
		Model:         selected.Model,
		MaxIterations: selected.MaxIterations,
		Task:          task,
		Scope:         resolvedScope,
		Verbose:       *verbose,
		RunDir:        resolvedRunDir,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}

	notifyUpdate()

	os.Exit(code)
}

func printVersion() {
	// the build kind is on the version line because it changes what the binary
	// reads from disk: "why is my .env ignored" should be answerable from here.
	fmt.Printf("rook %s (%s)\n", version.Version, buildinfo.Kind)
	notifyUpdate()
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
// It is silently skipped for dev builds and on any network error.
func notifyUpdate() {
	result, err := version.Check()
	if err != nil {
		return
	}
	if notice := version.FormatUpdateNotice(result); notice != "" {
		fmt.Fprintf(os.Stderr, "\n%s\n", notice)
	}
}
