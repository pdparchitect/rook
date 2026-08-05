// Command rook is a standalone autonomous security agent for vulnerability
// research, bug hunting and source-code auditing. It is built on the
// ChatBotKit Go SDK and ships with an embedded library of security skills.
//
// Usage:
//
//	export RELAY_API_KEY="your-provider-key"   # default "relay" backend
//	rook "Audit the HTTP handlers in ./server for injection bugs"
//	rook --scope "repo: ./server, no network" "Hunt for auth bypasses"
//	rook --backend cbk "..."                   # ChatBotKit backend instead
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
	"os/signal"
	"strings"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/spf13/pflag"

	"github.com/chatbotkit/rook/internal/agent"
	"github.com/chatbotkit/rook/internal/config"
	"github.com/chatbotkit/rook/internal/version"
)

func main() {
	// A .env in the working directory is a convenience for populating the
	// environment (e.g. RELAY_API_KEY); the config file is the primary surface.
	godotenv.Load()

	flags := pflag.NewFlagSet("rook", pflag.ContinueOnError)
	configPath := flags.String("config", "", "path to the config file (default: $ROOK_CONFIG or ~/.config/rook/config.yaml)")
	backend := flags.String("backend", "", "backend to target: relay (default), cbk, or chatbotkit")
	model := flags.String("model", "", "model the agent reasons with (overrides config)")
	maxIter := flags.Int("max-iterations", 0, "maximum agent iterations before forced stop (overrides config)")
	scope := flags.String("scope", "", "authorization boundary (hosts, repos, paths) the agent must stay within")
	scopeFile := flags.String("scope-file", "", "read the authorization scope from a file")
	verbose := flags.BoolP("verbose", "v", false, "stream the agent's reasoning tokens to stdout")
	showVersion := flags.BoolP("version", "V", false, "print version and exit")

	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "rook - autonomous security research agent\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  rook [flags] <task>\n  rook version\n\nFlags:\n")
		flags.PrintDefaults()
	}

	// Allow `rook version` as a subcommand in addition to the --version flag.
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "version" {
		printVersion()
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

	selected, resolvedModel, resolvedMaxIter, err := cfg.Selected()
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

	// Strip backend credentials from the environment before the agent runs, so
	// the commands it executes against a target cannot read them. The resolved
	// secret is still handed to the SDK client below.
	config.ScrubBackendSecrets(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	code, err := agent.Run(ctx, agent.Config{
		APISecret:     selected.APISecret,
		BaseURL:       selected.BaseURL,
		Model:         resolvedModel,
		MaxIterations: resolvedMaxIter,
		Task:          task,
		Scope:         resolvedScope,
		Verbose:       *verbose,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}

	notifyUpdate()

	os.Exit(code)
}

func printVersion() {
	fmt.Printf("rook %s\n", version.Version)
	notifyUpdate()
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
