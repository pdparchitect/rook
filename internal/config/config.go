// Package config holds Rook's central configuration. It layers built-in
// defaults, an optional YAML file, and environment variables (defaults < file <
// env), the same model as the sibling incubator tools (pantalk, zot), so a Rook
// run is configured the same way everywhere rather than through an ad-hoc .env.
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultModel is the model the agent reasons with when nothing overrides it.
//
// glm-5.2 is a strong open model well suited to autonomous security work: large
// context for reading codebases during source audits, solid tool use, and it is
// open/permissive for offensive-security tasks. The model must be one the
// selected backend actually serves, which is why the default backend below is
// the provider that serves this one.
const DefaultModel = "glm-5.2"

// DefaultMaxIterations bounds how many tool-using turns the agent may take
// before it is forced to stop.
const DefaultMaxIterations = 10000

// DefaultBackend is the backend a run targets when --backend and config do not
// select one.
//
// @note it has to serve DefaultModel. A default pair that cannot talk to each
// other is worse than no default, because the failure arrives as a provider
// error rather than as a configuration one.
const DefaultBackend = "zai"

// Config is the fully-resolved Rook configuration.
type Config struct {
	Agent Agent `yaml:"agent"`
	// RunDir is the base directory under which each run writes its artifacts (a
	// per-run subdirectory with status.json and events.jsonl). Empty uses the
	// built-in default, $XDG_STATE_HOME/rook/runs.
	RunDir string `yaml:"run_dir"`
	// DefaultBackend is the backend used when --backend is not given.
	DefaultBackend string `yaml:"default_backend"`
	// Backends are the named providers a run can target. Rook ships with one per
	// provider it knows; a config file can override their credential or
	// endpoint, or add custom model entries.
	Backends map[string]Backend `yaml:"backends"`
}

// Agent holds the knobs that shape an autonomous run.
type Agent struct {
	// Model is the model name driving the agent, resolved against the backend.
	Model string `yaml:"model"`
	// MaxIterations caps how many plan/act/observe cycles the agent may run
	// before it is forced to stop.
	MaxIterations int `yaml:"max_iterations"`
}

// Backend is a provider Rook can run against. Every provider authenticates with
// a Bearer credential.
type Backend struct {
	// Provider names the model provider this backend talks to: "openai",
	// "anthropic", "zai" and so on. Empty infers it from the backend's own name,
	// so a backend called "groq" needs no further configuration.
	Provider string `yaml:"provider"`
	// BaseURL overrides the API endpoint. Empty uses the built-in default.
	// Required for a "custom" provider.
	BaseURL string `yaml:"base_url"`
	// APIKey is the provider credential. Supports "$ENV_VAR" references, so no
	// secret need be written to disk; for a built-in it defaults from the
	// provider's conventional variable.
	APIKey string `yaml:"api_key"`
	// Models holds custom, named model configurations for this backend. When a
	// run's model name matches a key here, that entry's settings take priority.
	Models map[string]ModelConfig `yaml:"models"`
}

// ModelConfig is a custom model definition under a backend. Any field set here
// overrides the run's defaults when the model is selected.
type ModelConfig struct {
	// Model is the underlying model id to send. Lets a custom name alias a real
	// model; leave empty to use the selected name as-is.
	Model string `yaml:"model"`
	// MaxIterations overrides the global iteration cap for this model.
	MaxIterations int `yaml:"max_iterations"`
	// Provider overrides the backend's provider for this model, so one gateway
	// entry can front several.
	Provider string `yaml:"provider"`
	// APIKey is this model's own credential, overriding the backend's. Supports
	// "$ENV_VAR".
	APIKey string `yaml:"api_key"`
}

// builtinBackends are the providers Rook ships with, each seeded from its
// provider's conventional environment variable so exporting that one variable is
// all a run needs.
//
// The endpoints themselves live in the engine, which is what actually calls
// them; duplicating the URLs here would give two places for them to drift.
// Ollama is deliberately included: a local model is the right default for
// security work on material that must not leave the machine.
var builtinBackends = map[string]struct {
	secretEnv string // the provider's conventional credential variable
}{
	"openai":     {secretEnv: "OPENAI_API_KEY"},
	"anthropic":  {secretEnv: "ANTHROPIC_API_KEY"},
	"groq":       {secretEnv: "GROQ_API_KEY"},
	"mistral":    {secretEnv: "MISTRAL_API_KEY"},
	"deepseek":   {secretEnv: "DEEPSEEK_API_KEY"},
	"openrouter": {secretEnv: "OPENROUTER_API_KEY"},
	"together":   {secretEnv: "TOGETHER_API_KEY"},
	"cerebras":   {secretEnv: "CEREBRAS_API_KEY"},
	"xai":        {secretEnv: "XAI_API_KEY"},
	"moonshot":   {secretEnv: "MOONSHOT_API_KEY"},
	"zai":        {secretEnv: "ZAI_API_KEY"},
	"qwen":       {secretEnv: "DASHSCOPE_API_KEY"},
	"ollama":     {},
}

// BackendProvider returns the provider a backend talks to, inferring it from the
// backend's own name when nothing says otherwise.
func BackendProvider(name string, backend Backend) string {
	if p := strings.TrimSpace(backend.Provider); p != "" {
		return p
	}

	if _, ok := builtinBackends[name]; ok {
		return name
	}

	return ""
}

// Defaults returns the built-in configuration used when nothing else is set.
func Defaults() Config {
	return Config{
		Agent: Agent{
			Model:         DefaultModel,
			MaxIterations: DefaultMaxIterations,
		},
		DefaultBackend: DefaultBackend,
	}
}

// Load resolves the configuration: defaults, then the YAML file (if present),
// then environment overrides. A missing file at the default path is fine - env
// vars alone can configure Rook; a bad explicit --config file is an error.
func Load(path string) (Config, error) {
	cfg := Defaults()

	explicit := path != ""
	if path == "" {
		path = DefaultConfigPath()
	}

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse %s: %w", path, err)
		}
	case os.IsNotExist(err) && !explicit:
		// No default config file: rely on defaults + env.
	default:
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}

	if err := applyEnv(&cfg); err != nil {
		return cfg, err
	}

	resolveBackends(&cfg)

	if cfg.DefaultBackend == "" {
		cfg.DefaultBackend = DefaultBackend
	}

	return cfg, nil
}

// resolveBackends ensures the built-in backends exist and resolves every
// credential: a config "$ENV_VAR" reference first, then the provider's
// conventional environment variable as a fallback.
//
// The endpoint is left empty for a built-in. The engine knows each provider's
// URL, so filling one in here would create a second copy to drift.
func resolveBackends(cfg *Config) {
	if cfg.Backends == nil {
		cfg.Backends = map[string]Backend{}
	}

	for name := range builtinBackends {
		if _, ok := cfg.Backends[name]; !ok {
			cfg.Backends[name] = Backend{}
		}
	}

	for name, b := range cfg.Backends {
		builtin, isBuiltin := builtinBackends[name]

		b.APIKey = resolveSecret(b.APIKey)

		// Exporting the provider's own variable is enough on its own, which is
		// what makes a run possible with no config file at all.
		if b.APIKey == "" && isBuiltin && builtin.secretEnv != "" {
			b.APIKey = strings.TrimSpace(os.Getenv(builtin.secretEnv))
		}

		for mName, mc := range b.Models {
			if mc.APIKey != "" {
				mc.APIKey = resolveSecret(mc.APIKey)
				b.Models[mName] = mc
			}
		}

		cfg.Backends[name] = b
	}
}

// resolveSecret expands a "$ENV_VAR" / "${ENV_VAR}" reference; a literal value
// is returned unchanged.
func resolveSecret(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "$") {
		name := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(v, "$"), "{"), "}")
		return strings.TrimSpace(os.Getenv(strings.TrimSpace(name)))
	}
	return v
}

// Selected resolves the default backend into the provider, endpoint, credential,
// model and iteration cap a run uses, applying any custom model definition.
//
// It is the one place a backend choice turns into concrete client settings, so
// a misconfiguration is reported here - before a request is made - rather than
// as a provider error mid-run.
func (c Config) Selected() (Selection, error) {
	b, ok := c.Backends[c.DefaultBackend]
	if !ok {
		return Selection{}, fmt.Errorf("backend %q is not configured", c.DefaultBackend)
	}

	selection := Selection{
		Provider:      BackendProvider(c.DefaultBackend, b),
		BaseURL:       b.BaseURL,
		APIKey:        b.APIKey,
		Model:         c.Agent.Model,
		MaxIterations: c.Agent.MaxIterations,
	}

	if mc, ok := b.Models[selection.Model]; ok {
		if mc.Model != "" {
			selection.Model = mc.Model
		}
		if mc.MaxIterations > 0 {
			selection.MaxIterations = mc.MaxIterations
		}
		if mc.Provider != "" {
			selection.Provider = mc.Provider
		}
		if mc.APIKey != "" {
			selection.APIKey = mc.APIKey
		}
	}

	if selection.Provider == "" {
		return Selection{}, fmt.Errorf(
			"backend %q does not name a model provider (set provider: on the backend or the model)",
			c.DefaultBackend)
	}

	// Ollama is local and unauthenticated; everything else needs a key, and
	// saying so now beats a 401 halfway through a run.
	if selection.APIKey == "" && selection.Provider != "ollama" {
		return Selection{}, fmt.Errorf(
			"no API key for backend %q (set %s in the environment, or api_key in config)",
			c.DefaultBackend, secretEnvName(c.DefaultBackend))
	}

	return selection, nil
}

// Selection is a resolved backend choice: everything a run needs to build its
// client.
type Selection struct {
	Provider      string
	BaseURL       string
	APIKey        string
	Model         string
	MaxIterations int
}

func secretEnvName(backend string) string {
	if b, ok := builtinBackends[backend]; ok && b.secretEnv != "" {
		return b.secretEnv
	}

	return "its credential"
}

// ScrubBackendSecrets removes every resolved backend credential, backend-level
// and per-model, from the process environment.
//
// Config keeps the resolved values for the client, while shell commands the
// agent runs no longer inherit them. That matters more for Rook than for most
// tools: an offensive-security agent runs commands against targets, and a
// provider key in the environment of one of those commands is a key that can
// leave with it.
func ScrubBackendSecrets(cfg Config) {
	secrets := map[string]bool{}
	add := func(v string) {
		if v != "" {
			secrets[v] = true
		}
	}
	for _, backend := range cfg.Backends {
		add(backend.APIKey)
		for _, mc := range backend.Models {
			add(mc.APIKey)
		}
	}
	if len(secrets) == 0 {
		return
	}

	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if ok && secrets[value] {
			_ = os.Unsetenv(name)
		}
	}
}

// Validate checks the fully-merged configuration.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Agent.Model) == "" {
		return fmt.Errorf("agent.model must be set")
	}
	if c.Agent.MaxIterations <= 0 {
		return fmt.Errorf("agent.max_iterations must be a positive number")
	}
	if _, ok := c.Backends[c.DefaultBackend]; !ok {
		return fmt.Errorf("default backend %q is not configured", c.DefaultBackend)
	}
	return nil
}

// Backstory is Rook's system prompt. It is the single source of truth for the
// agent's persona, operating rules and safety constraints. The %s verb is
// replaced at runtime with the resolved authorization scope.
//
// Edit this string to change how the agent behaves across the whole tool.
const Backstory = `You are Rook, an autonomous offensive-security agent specialised in
vulnerability research, bug hunting, source-code auditing and exploit
development. You operate as a careful, methodical researcher.

You have four tools: "read" and "list" to inspect files and directories,
"write" to create them, and "shell" to run commands.

Operating rules:
- Stay strictly within the authorized scope. Never touch systems, hosts,
  repositories or paths outside it.
- Work in phases: reconnaissance, analysis, hypothesis, verification,
  reporting. Narrate each phase in your reasoning so the run is followable.
- Prefer reading and static analysis before any active testing. Use "shell"
  only for safe, non-destructive, non-interactive commands.
- Every claimed vulnerability must be backed by concrete evidence (a file
  and line, a request/response, a reproduction). Do not speculate without
  marking it clearly as a hypothesis.
- Do not create files on your own. Deliver your findings as your response;
  only "write" a file if the task explicitly asks for it.
- When the investigation is complete, record the outcome: call "_success"
  with a summary of what you found, or "_failure" with the reason if you
  cannot proceed. Do not simply stop - the run is not over until an outcome
  is recorded.

You have a built-in library of security skills. Read the relevant skill with
the "skill" tool before starting each phase.

%s`
