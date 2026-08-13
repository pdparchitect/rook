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

	for name := range builtinProviders {
		if env := builtinProviders[name].secretEnv; env != "" {
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

	if selection.Driver != "zai" || selection.APIKey != "sk-zai" {
		t.Errorf("selection = %+v", selection)
	}

	if selection.Model != DefaultModel {
		t.Errorf("model = %q, want %q", selection.Model, DefaultModel)
	}
}

// The default model must be one the default provider actually serves. A pair
// that cannot talk to each other fails as a provider error mid-run rather than
// as a configuration error before it starts.
func TestTheDefaultPairAgrees(t *testing.T) {
	if _, ok := builtinProviders[DefaultProvider]; !ok {
		t.Fatalf("the default provider %q is not built in", DefaultProvider)
	}

	// glm-5.2 is Z.AI's model; if either default moves, the other has to follow
	if DefaultProvider != "zai" || DefaultModel != "glm-5.2" {
		t.Errorf("defaults are %s/%s - check they still serve each other",
			DefaultProvider, DefaultModel)
	}
}

// The built-in providers are exactly the providers Rook speaks to. Pinning the
// whole set catches an accidental addition and an accidental removal with one
// assertion.
func TestBuiltinProvidersAreExactlyTheDrivers(t *testing.T) {
	isolate(t)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	seeded := make([]string, 0, len(cfg.Providers))

	for name := range cfg.Providers {
		seeded = append(seeded, name)
	}

	sort.Strings(seeded)

	want := []string{
		"abliteration", "anthropic", "cerebras", "deepseek", "groq", "mistral",
		"moonshot", "ollama", "openai", "openrouter", "qwen", "together", "xai",
		"zai",
	}

	if !reflect.DeepEqual(seeded, want) {
		t.Errorf("built-in providers:\n got %v\nwant %v", seeded, want)
	}

	for _, name := range seeded {
		if ProviderDriver(name, cfg.Providers[name]) == "" {
			t.Errorf("provider %q resolves to no driver", name)
		}
	}
}

// A provider connection named after a driver needs no further configuration -
// the name is the driver.
func TestTheProviderNameInfersTheDriver(t *testing.T) {
	tests := []struct {
		name     string
		provider ProviderConfig
		want     string
	}{
		{name: "groq", want: "groq"},
		{name: "openai", want: "openai"},
		{name: "mygateway", provider: ProviderConfig{Driver: "custom"}, want: "custom"},
		{name: "mygateway", want: ""},
		{name: "openai", provider: ProviderConfig{Driver: "anthropic"}, want: "anthropic"},
	}

	for _, test := range tests {
		if got := ProviderDriver(test.name, test.provider); got != test.want {
			t.Errorf("ProviderDriver(%q, %+v) = %q, want %q",
				test.name, test.provider, got, test.want)
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
		t.Fatal("a provider with no credential must be rejected")
	}

	// the message has to name the variable to export
	if !strings.Contains(err.Error(), "ZAI_API_KEY") {
		t.Errorf("error = %q, want it to name the variable", err)
	}
}

// Ollama is local and unauthenticated. Demanding a key would make the one
// provider that never sends data off the machine the hardest to use - which is
// backwards for security work on sensitive material.
func TestOllamaNeedsNoKey(t *testing.T) {
	isolate(t)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cfg.DefaultProvider = "ollama"

	selection, err := cfg.Selected()
	if err != nil {
		t.Fatalf("Selected: %v", err)
	}

	if selection.Driver != "ollama" {
		t.Errorf("provider = %q, want ollama", selection.Driver)
	}
}

// The engine does not know Abliteration.ai, so the built-in is the one
// built-in that carries its endpoint: the provider has to resolve to the
// provider's URL and credential with nothing but ABLIT_KEY exported.
func TestAbliterationSeedsItsEndpoint(t *testing.T) {
	isolate(t)
	t.Setenv("ABLIT_KEY", "sk-abliteration")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cfg.DefaultProvider = "abliteration"

	selection, err := cfg.Selected()
	if err != nil {
		t.Fatalf("Selected: %v", err)
	}

	if selection.Driver != "abliteration" {
		t.Errorf("driver = %q, want abliteration", selection.Driver)
	}

	if selection.BaseURL != "https://api.abliteration.ai/v1" {
		t.Errorf("base URL = %q, want the provider's endpoint", selection.BaseURL)
	}

	if selection.APIKey != "sk-abliteration" {
		t.Errorf("key = %q, want the exported one", selection.APIKey)
	}

	// an explicit base_url still wins, as it does for every provider
	path := writeConfig(t, `
providers:
  abliteration:
    base_url: 'https://staging.abliteration.ai/v1'
`)

	cfg, err = Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := cfg.Providers["abliteration"].BaseURL; got != "https://staging.abliteration.ai/v1" {
		t.Errorf("base URL = %q, want the configured override", got)
	}
}

// A provider connection that names no driver cannot resolve, and says so.
func TestAnUnknownProviderIsRejected(t *testing.T) {
	isolate(t)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cfg.DefaultProvider = "nowhere"

	if _, err := cfg.Selected(); err == nil {
		t.Fatal("an unconfigured provider must be rejected")
	}

	cfg.Providers["nowhere"] = ProviderConfig{APIKey: "sk-test"}

	_, err = cfg.Selected()
	if err == nil {
		t.Fatal("a provider naming no driver must be rejected")
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
default_provider: openai
providers:
  openai:
    api_key: '$MY_PROVIDER_KEY'
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := cfg.Providers["openai"].APIKey; got != "sk-from-env" {
		t.Errorf("key = %q, want the expanded value", got)
	}

	path = writeConfig(t, `
providers:
  openai:
    api_key: '${MY_PROVIDER_KEY}'
`)

	cfg, err = Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := cfg.Providers["openai"].APIKey; got != "sk-from-env" {
		t.Errorf("braced key = %q", got)
	}
}

// An unset variable resolves to nothing, so the run fails with "no API key"
// rather than sending the literal text "$MY_KEY" to the provider.
func TestAnUnsetEnvReferenceResolvesToNothing(t *testing.T) {
	isolate(t)
	t.Setenv("ROOK_TEST_UNSET", "")

	path := writeConfig(t, `
default_provider: openai
providers:
  openai:
    api_key: '$ROOK_TEST_UNSET'
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := cfg.Providers["openai"].APIKey; got != "" {
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
default_provider: openai
`)

	t.Setenv("ROOK_AGENT_MODEL", "from-env")
	t.Setenv("ROOK_DEFAULT_PROVIDER", "groq")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Agent.Model != "from-env" {
		t.Errorf("model = %q, want from-env", cfg.Agent.Model)
	}

	if cfg.DefaultProvider != "groq" {
		t.Errorf("default provider = %q, want groq", cfg.DefaultProvider)
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
default_provider: mygateway
providers:
  mygateway:
    driver: custom
    base_url: 'https://gateway.example.com/v1'
    api_key: sk-gateway
    models:
      fast:
        model: gpt-5
        max_iterations: 50
        driver: openai
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

	if selection.Driver != "openai" || selection.APIKey != "sk-openai" {
		t.Errorf("the model's own provider and key must win: %+v", selection)
	}

	if selection.BaseURL != "https://gateway.example.com/v1" {
		t.Errorf("base URL = %q, want the provider's", selection.BaseURL)
	}
}

// Scrubbing removes every resolved credential from the environment. An
// offensive-security agent runs commands against targets, and a provider key in
// one of those commands' environment is a key that can leave with it.
func TestScrubProviderSecrets(t *testing.T) {
	isolate(t)
	t.Setenv("ZAI_API_KEY", "sk-zai")
	t.Setenv("OPENAI_API_KEY", "sk-openai")
	t.Setenv("ROOK_TEST_UNRELATED", "keep-me")

	path := writeConfig(t, `
default_provider: zai
providers:
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

	ScrubProviderSecrets(cfg)

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
	if cfg.Providers["zai"].APIKey != "sk-zai" {
		t.Error("scrubbing must not empty the resolved config")
	}
}

// Validate catches a missing model, a non-positive iteration cap, and a default
// provider the config does not define - before any request reaches a provider.
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

	// Unknown default provider.
	goodProvider := cfg.DefaultProvider
	cfg.DefaultProvider = "nowhere"
	if err := cfg.Validate(); err == nil {
		t.Error("an unknown default provider must be rejected")
	}
	cfg.DefaultProvider = goodProvider
}

// secretEnvName returns the conventional variable for a built-in provider, and a
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

	path := writeConfig(t, "default_provider: zai\n")
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

// A built-in provider's conventional env key is withheld once base_url is
// overridden: forwarding OPENAI_API_KEY to a URL someone typed is exactly the
// leak an offensive-security tool must not create.
func TestOverriddenBaseURLWithholdsTheAmbientKey(t *testing.T) {
	isolate(t)
	t.Setenv("OPENAI_API_KEY", "sk-openai")

	// No base_url: the conventional key seeds the connection.
	cfg, err := Load(writeConfig(t, "default_provider: openai\nproviders:\n  openai:\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Providers["openai"].APIKey; got != "sk-openai" {
		t.Fatalf("without base_url the ambient key should seed the connection, got %q", got)
	}

	// base_url set: the ambient key is withheld, so the connection has no key.
	cfg, err = Load(writeConfig(t, "default_provider: openai\nproviders:\n  openai:\n    base_url: https://gw.example.com/v1\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Providers["openai"].APIKey; got != "" {
		t.Errorf("an overridden base_url must not inherit OPENAI_API_KEY, got %q", got)
	}
}

// MaxDuration parses a duration string, treats empty as unbounded, and rejects
// a malformed or negative value.
func TestMaxDuration(t *testing.T) {
	if d, err := (Agent{MaxTime: ""}).MaxDuration(); err != nil || d != 0 {
		t.Errorf("empty = (%v, %v), want (0, nil)", d, err)
	}
	if d, err := (Agent{MaxTime: "30m"}).MaxDuration(); err != nil || d.Minutes() != 30 {
		t.Errorf("30m = (%v, %v)", d, err)
	}
	if _, err := (Agent{MaxTime: "soon"}).MaxDuration(); err == nil {
		t.Error("a malformed duration must be rejected")
	}
	if _, err := (Agent{MaxTime: "-5m"}).MaxDuration(); err == nil {
		t.Error("a negative duration must be rejected")
	}
}

// Validate catches the new tuning knobs' bad values before a run starts.
func TestValidateRejectsBadTuning(t *testing.T) {
	isolate(t)
	t.Setenv("ZAI_API_KEY", "sk-zai")

	base := func(t *testing.T) Config {
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		return cfg
	}

	cfg := base(t)
	cfg.Agent.ContextStrategy = "sideways"
	if err := cfg.Validate(); err == nil {
		t.Error("an unknown context_strategy must be rejected")
	}

	cfg = base(t)
	cfg.Agent.CompactTriggerRatio = 1.5
	if err := cfg.Validate(); err == nil {
		t.Error("a compact_trigger_ratio above 1 must be rejected")
	}

	cfg = base(t)
	cfg.Agent.LimitCheckpoints = []int{50, 120}
	if err := cfg.Validate(); err == nil {
		t.Error("an out-of-range limit checkpoint must be rejected")
	}

	cfg = base(t)
	cfg.Agent.MaxTime = "nope"
	if err := cfg.Validate(); err == nil {
		t.Error("a malformed max_time must be rejected")
	}

	// A run with valid tuning still passes.
	cfg = base(t)
	cfg.Agent.ContextStrategy = "truncate"
	cfg.Agent.CompactTriggerRatio = 0.9
	cfg.Agent.LimitCheckpoints = []int{50, 90}
	cfg.Agent.MaxTime = "2h"
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid tuning must pass: %v", err)
	}
}

// A model entry's context and vision overrides resolve into the Selection, so a
// custom endpoint can state a ceiling and that it can be shown images.
func TestModelCapabilityOverridesResolve(t *testing.T) {
	isolate(t)
	t.Setenv("OPENAI_API_KEY", "sk-openai")

	yes := true
	cfg, err := Load(writeConfig(t, `
agent:
  model: fast
default_provider: openai
providers:
  openai:
    models:
      fast: {}
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// default: no override -> zero/false
	if sel, _ := cfg.Selected(); sel.ContextWindow != 0 || sel.Vision {
		t.Errorf("unset overrides should be zero/false, got %+v", sel)
	}

	m := cfg.Providers["openai"].Models["fast"]
	m.Context = 8000
	m.Vision = &yes
	cfg.Providers["openai"].Models["fast"] = m

	sel, err := cfg.Selected()
	if err != nil {
		t.Fatalf("Selected: %v", err)
	}
	if sel.ContextWindow != 8000 || !sel.Vision {
		t.Errorf("overrides did not resolve: %+v", sel)
	}
}

// A provider key must be scrubbed from the agent's environment even when that
// provider has an overridden base_url (so its ambient key was never adopted
// into config). Regression: the base_url scoping fix must not leave the key
// readable to the commands the agent runs against a target.
func TestScrubRemovesKeysOfOverriddenProviders(t *testing.T) {
	isolate(t)
	t.Setenv("ZAI_API_KEY", "sk-zai")
	t.Setenv("OPENAI_API_KEY", "sk-openai")

	// Run zai, but the openai connection has a custom base_url (so it does not
	// adopt OPENAI_API_KEY as its own credential).
	cfg, err := Load(writeConfig(t, `
default_provider: zai
providers:
  openai:
    base_url: https://gw.example.com/v1
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	ScrubProviderSecrets(cfg)

	if got := os.Getenv("OPENAI_API_KEY"); got != "" {
		t.Errorf("OPENAI_API_KEY survived scrubbing despite base_url override: %q", got)
	}
	if got := os.Getenv("ZAI_API_KEY"); got != "" {
		t.Errorf("the active provider key survived scrubbing: %q", got)
	}
}
