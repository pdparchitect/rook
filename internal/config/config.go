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
// selected backend serves (a ChatBotKit catalogue name).
const DefaultModel = "glm-5.2"

// DefaultMaxIterations bounds how many tool-using turns the agent may take
// before it is forced to stop.
const DefaultMaxIterations = 10000

// DefaultBackend is the backend a run targets when --backend and config do not
// select one. Rook defaults to the CBK Relay: a run brings its own provider
// key and reaches models through relay.cbk.ai.
const DefaultBackend = "relay"

// Config is the fully-resolved Rook configuration.
type Config struct {
	Agent Agent `yaml:"agent"`
	// DefaultBackend is the backend used when --backend is not given.
	DefaultBackend string `yaml:"default_backend"`
	// Backends are the named providers a run can target. Rook ships with three -
	// "relay" (CBK Relay, the default), "cbk" (ChatBotKit at api.cbk.ai) and
	// "chatbotkit" (ChatBotKit at api.chatbotkit.com) - and a config file can
	// override their credentials/endpoint or add custom model entries.
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

// Backend is a provider Rook can run against. How a run authenticates depends on
// the backend's style: the ChatBotKit backends send a Bearer credential
// (APISecret); the relay carries the provider credential per model, inside the
// model string (Authorization), because on the relay each model is a different
// provider with its own key.
type Backend struct {
	// BaseURL overrides the API endpoint. Empty uses the built-in default.
	BaseURL string `yaml:"base_url"`
	// APISecret is the Bearer credential for the ChatBotKit backends. Supports
	// "$ENV_VAR" references; for the built-ins it defaults from the environment.
	APISecret string `yaml:"api_secret"`
	// Authorization is the relay's default model credential, applied to every
	// model on this backend that does not set its own. Supports "$ENV_VAR" (point
	// it at your own provider key). It has no built-in environment default.
	// Ignored by the Bearer backends.
	Authorization string `yaml:"authorization"`
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
	// Authorization is this model's provider credential on the relay, composed
	// into the model string as "<model>/authorization=<value>". Supports
	// "$ENV_VAR". Overrides the backend-level Authorization. Ignored by the
	// Bearer backends.
	Authorization string `yaml:"authorization"`
}

// authStyle is how a backend authenticates a run.
type authStyle int

const (
	// authBearer sends the credential as an Authorization: Bearer header.
	authBearer authStyle = iota
	// authModelParam carries the credential inside the model string, as
	// "<model>/authorization=<value>" - the CBK Relay's convention, where each
	// model is a distinct provider authenticated with its own key.
	authModelParam
)

// builtinBackends are the providers Rook ships with: their default endpoint and
// how they authenticate. The Bearer backends fall back to a brand-named
// environment variable for their credential; the relay has no such default -
// its per-model provider credential comes from config (or is inlined into the
// model). "cbk" and "chatbotkit" are the same platform on its two hosts and
// take the same credential value under their own variable.
var builtinBackends = map[string]struct {
	baseURL   string
	style     authStyle
	secretEnv string // Bearer credential fallback (authBearer)
}{
	"relay":      {baseURL: "https://relay.cbk.ai", style: authModelParam},
	"cbk":        {baseURL: "https://api.cbk.ai", style: authBearer, secretEnv: "CBK_API_SECRET"},
	"chatbotkit": {baseURL: "https://api.chatbotkit.com", style: authBearer, secretEnv: "CHATBOTKIT_API_SECRET"},
}

func backendStyle(name string) authStyle {
	if b, ok := builtinBackends[name]; ok {
		return b.style
	}
	return authBearer
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

// resolveBackends ensures the built-in backends exist, fills their default
// endpoint, and resolves every credential (config "$ENV" reference first, then
// the built-in environment fallback) - the Bearer secret, the backend-level
// model authorization, and each model's own authorization.
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
		if b.BaseURL == "" && isBuiltin {
			b.BaseURL = builtin.baseURL
		}

		// Bearer credential (authBearer backends).
		if s := strings.TrimSpace(b.APISecret); s != "" {
			b.APISecret = resolveSecret(s)
		} else if isBuiltin && builtin.secretEnv != "" {
			b.APISecret = strings.TrimSpace(os.Getenv(builtin.secretEnv))
		}

		// Backend-level model authorization (authModelParam backends). No
		// environment default - the relay's provider credential comes from config
		// (or is inlined into the model).
		if a := strings.TrimSpace(b.Authorization); a != "" {
			b.Authorization = resolveSecret(a)
		}

		// Per-model authorization.
		for mName, mc := range b.Models {
			if mc.Authorization != "" {
				mc.Authorization = resolveSecret(mc.Authorization)
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

// Selected resolves the default backend into the endpoint, credential, model,
// and iteration cap a run uses, applying any custom model definition. It is the
// one place the backend choice turns into concrete client settings - including
// composing the relay's "<model>/authorization=<key>" model string.
func (c Config) Selected() (backend Backend, model string, maxIterations int, err error) {
	b, ok := c.Backends[c.DefaultBackend]
	if !ok {
		return Backend{}, "", 0, fmt.Errorf("backend %q is not configured", c.DefaultBackend)
	}

	model = c.Agent.Model
	maxIterations = c.Agent.MaxIterations
	auth := b.Authorization
	if mc, ok := b.Models[model]; ok {
		if mc.Model != "" {
			model = mc.Model
		}
		if mc.MaxIterations > 0 {
			maxIterations = mc.MaxIterations
		}
		if mc.Authorization != "" {
			auth = mc.Authorization
		}
	}

	switch backendStyle(c.DefaultBackend) {
	case authModelParam:
		// The relay authenticates the provider per model, inside the model
		// string. Respect a key the caller already inlined into --model.
		if !strings.Contains(model, "/authorization=") {
			if auth == "" {
				return Backend{}, "", 0, fmt.Errorf(
					"no authorization for model %q on backend %q (set authorization on the model or the backend in config, or inline it as %s/authorization=KEY)",
					model, c.DefaultBackend, model)
			}
			model = model + "/authorization=" + auth
		}
	default: // authBearer
		if b.APISecret == "" {
			return Backend{}, "", 0, fmt.Errorf(
				"no API secret for backend %q (set %s in the environment or api_secret in config)",
				c.DefaultBackend, secretEnvName(c.DefaultBackend))
		}
	}

	return b, model, maxIterations, nil
}

func secretEnvName(backend string) string {
	if b, ok := builtinBackends[backend]; ok && b.secretEnv != "" {
		return b.secretEnv
	}
	return "its credential"
}

// ScrubBackendSecrets removes every resolved backend credential - Bearer
// secrets and provider authorizations, backend-level and per-model - from the
// process environment. Config retains the resolved values used by the SDK
// client, while shell commands the agent executes no longer inherit those
// credentials, which matters for an offensive-security agent that runs commands
// against targets.
func ScrubBackendSecrets(cfg Config) {
	secrets := map[string]bool{}
	add := func(v string) {
		if v != "" {
			secrets[v] = true
		}
	}
	for _, backend := range cfg.Backends {
		add(backend.APISecret)
		add(backend.Authorization)
		for _, mc := range backend.Models {
			add(mc.Authorization)
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

Operating rules:
- Stay strictly within the authorized scope. Never touch systems, hosts,
  repositories or paths outside it.
- Work in phases: reconnaissance, analysis, hypothesis, verification,
  reporting. Use the "plan" tool to lay out your approach and "progress" to
  record findings as you go.
- Prefer reading and static analysis before any active testing. Use the
  "exec" tool only for safe, non-destructive, non-interactive commands.
- Every claimed vulnerability must be backed by concrete evidence (a file
  and line, a request/response, a reproduction). Do not speculate without
  marking it clearly as a hypothesis.
- Do not create files on your own. Deliver your output as your response;
  only write files if the task explicitly asks for it.
- When the investigation is complete, produce a structured report and call
  the "exit" tool with code 0. Use a non-zero exit code if you cannot
  proceed.

You have a built-in library of security skills. Consult the relevant skill
before starting each phase.

%s`
