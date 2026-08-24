package config

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return path
}

// isolate stops a developer's real environment leaking into a test: the config
// seeds credentials from provider variables, and one exported in the shell
// would silently satisfy a case meant to fail.
func isolate(t *testing.T) {
	t.Helper()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("ROOK_CONFIG", "")

	for name := range builtinBackends {
		if env := builtinBackends[name].secretEnv; env != "" {
			t.Setenv(env, "")
		}
	}
}

// Exporting one provider variable is the whole setup. A run with no config file
// at all has to work, or the tool is unusable in a container.
func TestAProviderKeyAloneIsEnough(t *testing.T) {
	isolate(t)
	t.Setenv("ZAI_API_KEY", "sk-zai")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	selection, err := cfg.Selected()
	if err != nil {
		t.Fatalf("Selected: %v", err)
	}

	if selection.Provider != "zai" || selection.APIKey != "sk-zai" {
		t.Errorf("selection = %+v", selection)
	}

	if selection.Model != DefaultModel {
		t.Errorf("model = %q, want %q", selection.Model, DefaultModel)
	}
}

// The default model must be one the default backend actually serves. A pair
// that cannot talk to each other fails as a provider error mid-run rather than
// as a configuration error before it starts.
func TestTheDefaultPairAgrees(t *testing.T) {
	if _, ok := builtinBackends[DefaultBackend]; !ok {
		t.Fatalf("the default backend %q is not built in", DefaultBackend)
	}

	// glm-5.2 is Z.AI's model; if either default moves, the other has to follow
	if DefaultBackend != "zai" || DefaultModel != "glm-5.2" {
		t.Errorf("defaults are %s/%s - check they still serve each other",
			DefaultBackend, DefaultModel)
	}
}

// The built-in backends are exactly the providers Rook speaks to. Pinning the
// whole set catches an accidental addition and an accidental removal with one
// assertion.
func TestBuiltinBackendsAreExactlyTheProviders(t *testing.T) {
	isolate(t)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	seeded := make([]string, 0, len(cfg.Backends))

	for name := range cfg.Backends {
		seeded = append(seeded, name)
	}

	sort.Strings(seeded)

	want := []string{
		"anthropic", "cerebras", "deepseek", "groq", "mistral", "moonshot",
		"ollama", "openai", "openrouter", "qwen", "together", "xai", "zai",
	}

	if !reflect.DeepEqual(seeded, want) {
		t.Errorf("built-in backends:\n got %v\nwant %v", seeded, want)
	}

	for _, name := range seeded {
		if BackendProvider(name, cfg.Backends[name]) == "" {
			t.Errorf("backend %q names no provider", name)
		}
	}
}

// A backend named after a provider needs no further configuration - the name is
// the provider.
func TestTheBackendNameInfersTheProvider(t *testing.T) {
	tests := []struct {
		name    string
		backend Backend
		want    string
	}{
		{name: "groq", want: "groq"},
		{name: "openai", want: "openai"},
		{name: "mygateway", backend: Backend{Provider: "custom"}, want: "custom"},
		{name: "mygateway", want: ""},
		{name: "openai", backend: Backend{Provider: "anthropic"}, want: "anthropic"},
	}

	for _, test := range tests {
		if got := BackendProvider(test.name, test.backend); got != test.want {
			t.Errorf("BackendProvider(%q, %+v) = %q, want %q",
				test.name, test.backend, got, test.want)
		}
	}
}

// A missing credential is reported before any request, so the operator sees a
// configuration error rather than a 401 halfway through a run.
func TestAMissingKeyIsReportedUpFront(t *testing.T) {
	isolate(t)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	_, err = cfg.Selected()
	if err == nil {
		t.Fatal("a backend with no credential must be rejected")
	}

	// the message has to name the variable to export
	if !strings.Contains(err.Error(), "ZAI_API_KEY") {
		t.Errorf("error = %q, want it to name the variable", err)
	}
}

// Ollama is local and unauthenticated. Demanding a key would make the one
// backend that never sends data off the machine the hardest to use - which is
// backwards for security work on sensitive material.
func TestOllamaNeedsNoKey(t *testing.T) {
	isolate(t)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cfg.DefaultBackend = "ollama"

	selection, err := cfg.Selected()
	if err != nil {
		t.Fatalf("Selected: %v", err)
	}

	if selection.Provider != "ollama" {
		t.Errorf("provider = %q, want ollama", selection.Provider)
	}
}

// A backend that names no provider cannot resolve, and says so.
func TestAnUnknownBackendIsRejected(t *testing.T) {
	isolate(t)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cfg.DefaultBackend = "nowhere"

	if _, err := cfg.Selected(); err == nil {
		t.Fatal("an unconfigured backend must be rejected")
	}

	cfg.Backends["nowhere"] = Backend{APIKey: "sk-test"}

	_, err = cfg.Selected()
	if err == nil {
		t.Fatal("a backend naming no provider must be rejected")
	}

	if !strings.Contains(err.Error(), "provider") {
		t.Errorf("error = %q, want it to name what is missing", err)
	}
}

// A `$VAR` reference is how a key stays out of the config file.
func TestAnEnvReferenceIsExpanded(t *testing.T) {
	isolate(t)
	t.Setenv("MY_PROVIDER_KEY", "sk-from-env")

	path := writeConfig(t, `
default_backend: openai
backends:
  openai:
    api_key: '$MY_PROVIDER_KEY'
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := cfg.Backends["openai"].APIKey; got != "sk-from-env" {
		t.Errorf("key = %q, want the expanded value", got)
	}

	path = writeConfig(t, `
backends:
  openai:
    api_key: '${MY_PROVIDER_KEY}'
`)

	cfg, err = Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := cfg.Backends["openai"].APIKey; got != "sk-from-env" {
		t.Errorf("braced key = %q", got)
	}
}

// An unset variable resolves to nothing, so the run fails with "no API key"
// rather than sending the literal text "$MY_KEY" to the provider.
func TestAnUnsetEnvReferenceResolvesToNothing(t *testing.T) {
	isolate(t)
	t.Setenv("ROOK_TEST_UNSET", "")

	path := writeConfig(t, `
default_backend: openai
backends:
  openai:
    api_key: '$ROOK_TEST_UNSET'
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := cfg.Backends["openai"].APIKey; got != "" {
		t.Errorf("key = %q, want nothing", got)
	}
}

// Env vars override the file (defaults < file < env). CLI flags override env,
// but that layer lives in main.
func TestEnvOverridesFile(t *testing.T) {
	isolate(t)
	t.Setenv("GROQ_API_KEY", "sk-groq")

	path := writeConfig(t, `
agent:
  model: from-file
default_backend: openai
`)

	t.Setenv("ROOK_AGENT_MODEL", "from-env")
	t.Setenv("ROOK_DEFAULT_BACKEND", "groq")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Agent.Model != "from-env" {
		t.Errorf("model = %q, want from-env", cfg.Agent.Model)
	}

	if cfg.DefaultBackend != "groq" {
		t.Errorf("default backend = %q, want groq", cfg.DefaultBackend)
	}
}

// A custom model entry aliases a real id, caps iterations, and may carry its own
// provider and credential - which is what lets one gateway front several.
func TestCustomModelEntry(t *testing.T) {
	isolate(t)
	t.Setenv("OPENAI_API_KEY", "sk-openai")

	path := writeConfig(t, `
agent:
  model: fast
default_backend: mygateway
backends:
  mygateway:
    provider: custom
    base_url: 'https://gateway.example.com/v1'
    api_key: sk-gateway
    models:
      fast:
        model: gpt-5
        max_iterations: 50
        provider: openai
        api_key: $OPENAI_API_KEY
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	selection, err := cfg.Selected()
	if err != nil {
		t.Fatalf("Selected: %v", err)
	}

	if selection.Model != "gpt-5" {
		t.Errorf("model = %q, want the aliased id", selection.Model)
	}

	if selection.MaxIterations != 50 {
		t.Errorf("max iterations = %d, want 50", selection.MaxIterations)
	}

	if selection.Provider != "openai" || selection.APIKey != "sk-openai" {
		t.Errorf("the model's own provider and key must win: %+v", selection)
	}

	if selection.BaseURL != "https://gateway.example.com/v1" {
		t.Errorf("base URL = %q, want the backend's", selection.BaseURL)
	}
}

// Scrubbing removes every resolved credential from the environment. An
// offensive-security agent runs commands against targets, and a provider key in
// one of those commands' environment is a key that can leave with it.
func TestScrubBackendSecrets(t *testing.T) {
	isolate(t)
	t.Setenv("ZAI_API_KEY", "sk-zai")
	t.Setenv("OPENAI_API_KEY", "sk-openai")
	t.Setenv("ROOK_TEST_UNRELATED", "keep-me")

	path := writeConfig(t, `
default_backend: zai
backends:
  zai:
    api_key: $ZAI_API_KEY
    models:
      gpt-4:
        api_key: $OPENAI_API_KEY
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	ScrubBackendSecrets(cfg)

	if got := os.Getenv("ZAI_API_KEY"); got != "" {
		t.Errorf("ZAI_API_KEY survived scrubbing: %q", got)
	}

	if got := os.Getenv("OPENAI_API_KEY"); got != "" {
		t.Errorf("the per-model key survived scrubbing: %q", got)
	}

	if got := os.Getenv("ROOK_TEST_UNRELATED"); got != "keep-me" {
		t.Errorf("an unrelated variable was scrubbed: %q", got)
	}

	// the config keeps what the client needs
	if cfg.Backends["zai"].APIKey != "sk-zai" {
		t.Error("scrubbing must not empty the resolved config")
	}
}

// Validate catches a missing model, a non-positive iteration cap, and a default
// backend the config does not define - before any request reaches a provider.
func TestValidateRejectsBadConfigs(t *testing.T) {
	isolate(t)
	t.Setenv("ZAI_API_KEY", "sk-zai")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// A valid config passes.
	if err := cfg.Validate(); err != nil {
		t.Fatalf("a valid config should pass: %v", err)
	}

	// Missing model.
	good := cfg.Agent.Model
	cfg.Agent.Model = ""
	if err := cfg.Validate(); err == nil {
		t.Error("missing model must be rejected")
	}
	cfg.Agent.Model = good

	// Non-positive iterations.
	goodIter := cfg.Agent.MaxIterations
	cfg.Agent.MaxIterations = 0
	if err := cfg.Validate(); err == nil {
		t.Error("zero max_iterations must be rejected")
	}
	cfg.Agent.MaxIterations = -1
	if err := cfg.Validate(); err == nil {
		t.Error("negative max_iterations must be rejected")
	}
	cfg.Agent.MaxIterations = goodIter

	// Unknown default backend.
	goodBackend := cfg.DefaultBackend
	cfg.DefaultBackend = "nowhere"
	if err := cfg.Validate(); err == nil {
		t.Error("an unknown default backend must be rejected")
	}
	cfg.DefaultBackend = goodBackend
}

// secretEnvName returns the conventional variable for a built-in backend, and a
// generic fallback for one the tool does not know.
func TestSecretEnvName(t *testing.T) {
	if got := secretEnvName("zai"); got != "ZAI_API_KEY" {
		t.Errorf("secretEnvName(zai) = %q, want ZAI_API_KEY", got)
	}

	if got := secretEnvName("ollama"); got != "its credential" {
		t.Errorf("secretEnvName(ollama) = %q, want the generic fallback", got)
	}

	if got := secretEnvName("custom"); got != "its credential" {
		t.Errorf("secretEnvName(custom) = %q, want the generic fallback", got)
	}
}

// DefaultConfigPath honours $ROOK_CONFIG, then XDG_CONFIG_HOME, then falls back
// to ~/.config/rook/config.yaml.
func TestDefaultConfigPath(t *testing.T) {
	t.Setenv("ROOK_CONFIG", "/custom/path/config.yaml")
	if got := DefaultConfigPath(); got != "/custom/path/config.yaml" {
		t.Errorf("ROOK_CONFIG override = %q", got)
	}

	t.Setenv("ROOK_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got := DefaultConfigPath(); got != "/xdg/rook/config.yaml" {
		t.Errorf("XDG_CONFIG_HOME path = %q", got)
	}

	t.Setenv("XDG_CONFIG_HOME", "")
	home := os.Getenv("HOME")
	if home != "" {
		want := filepath.Join(home, ".config", "rook", "config.yaml")
		if got := DefaultConfigPath(); got != want {
			t.Errorf("default path = %q, want %q", got, want)
		}
	}
}

// DefaultRunDir honours $XDG_STATE_HOME, then falls back to
// ~/.local/state/rook/runs.
func TestDefaultRunDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/xdg-state")
	if got := DefaultRunDir(); got != "/xdg-state/rook/runs" {
		t.Errorf("XDG_STATE_HOME path = %q", got)
	}

	t.Setenv("XDG_STATE_HOME", "")
	home := os.Getenv("HOME")
	if home != "" {
		want := filepath.Join(home, ".local", "state", "rook", "runs")
		if got := DefaultRunDir(); got != want {
			t.Errorf("default run dir = %q, want %q", got, want)
		}
	}
}

// A bad integer in a ROOK_* env var is reported with the variable name, so the
// operator can find the misconfiguration.
func TestApplyEnvRejectsBadIntegers(t *testing.T) {
	isolate(t)

	path := writeConfig(t, "default_backend: zai\n")
	t.Setenv("ZAI_API_KEY", "sk-zai")
	t.Setenv("ROOK_AGENT_MAX_ITERATIONS", "not-a-number")

	if _, err := Load(path); err == nil {
		t.Error("a non-integer ROOK_AGENT_MAX_ITERATIONS must be rejected")
	}
}

// resolveSecret expands $VAR and ${VAR}, and returns a literal unchanged.
func TestResolveSecret(t *testing.T) {
	t.Setenv("ROOK_TEST_SECRET", "expanded")

	if got := resolveSecret("$ROOK_TEST_SECRET"); got != "expanded" {
		t.Errorf("dollar reference = %q", got)
	}

	if got := resolveSecret("${ROOK_TEST_SECRET}"); got != "expanded" {
		t.Errorf("braced reference = %q", got)
	}

	if got := resolveSecret("sk-literal"); got != "sk-literal" {
		t.Errorf("literal = %q", got)
	}

	if got := resolveSecret("  $ROOK_TEST_SECRET  "); got != "expanded" {
		t.Errorf("trimmed reference = %q", got)
	}

	t.Setenv("ROOK_TEST_SECRET", "")
	if got := resolveSecret("$ROOK_TEST_SECRET"); got != "" {
		t.Errorf("unset reference = %q, want empty", got)
	}
}

// setScalar handles strings, integers and booleans. A bad integer or boolean
// is an error rather than a silent drop.
func TestSetScalar(t *testing.T) {
	var s struct {
		Str string `yaml:"str"`
		Num int    `yaml:"num"`
		Flg bool   `yaml:"flg"`
	}

	v := reflect.ValueOf(&s).Elem()

	if err := setScalar(v.FieldByName("Str"), "hello"); err != nil {
		t.Fatalf("setScalar(string): %v", err)
	}
	if s.Str != "hello" {
		t.Errorf("string = %q", s.Str)
	}

	if err := setScalar(v.FieldByName("Num"), "42"); err != nil {
		t.Fatalf("setScalar(int): %v", err)
	}
	if s.Num != 42 {
		t.Errorf("int = %d", s.Num)
	}

	if err := setScalar(v.FieldByName("Flg"), "true"); err != nil {
		t.Fatalf("setScalar(bool): %v", err)
	}
	if !s.Flg {
		t.Error("bool = false, want true")
	}

	// Bad integer.
	if err := setScalar(v.FieldByName("Num"), "abc"); err == nil {
		t.Error("bad integer must be an error")
	}

	// Bad boolean.
	if err := setScalar(v.FieldByName("Flg"), "maybe"); err == nil {
		t.Error("bad boolean must be an error")
	}
}

// ParseDraft extracts the objective and proposed title from a model's free-text
// response. Covered here to lock the contract.
func TestConfigPathHomeFallback(t *testing.T) {
	t.Setenv("ROOK_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/tmp/rook-test-home")

	if got := DefaultConfigPath(); got != "/tmp/rook-test-home/.config/rook/config.yaml" {
		t.Errorf("home fallback = %q", got)
	}
}
