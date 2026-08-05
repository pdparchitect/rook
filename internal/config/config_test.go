package config

import (
	"os"
	"path/filepath"
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

// The default backend is the CBK relay. The relay authenticates the provider
// per model, inside the model string; there is no RELAY_API_KEY - a backend-level
// authorization set in config (pointing at the user's own provider key) is
// composed onto the default model.
func TestDefaultsToRelayBackend(t *testing.T) {
	t.Setenv("MY_PROVIDER_KEY", "sk-provider")
	path := writeConfig(t, `
default_backend: relay
backends:
  relay:
    authorization: $MY_PROVIDER_KEY
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultBackend != "relay" {
		t.Errorf("default backend = %q, want relay", cfg.DefaultBackend)
	}

	backend, model, maxIter, err := cfg.Selected()
	if err != nil {
		t.Fatalf("Selected: %v", err)
	}
	if backend.BaseURL != "https://relay.cbk.ai" {
		t.Errorf("relay base_url = %q, want https://relay.cbk.ai", backend.BaseURL)
	}
	// Relay uses no Bearer secret; the credential rides in the model string.
	if backend.APISecret != "" {
		t.Errorf("relay APISecret = %q, want empty", backend.APISecret)
	}
	want := DefaultModel + "/authorization=sk-provider"
	if model != want {
		t.Errorf("model = %q, want %q", model, want)
	}
	if maxIter != DefaultMaxIterations {
		t.Errorf("max iterations = %d, want %d", maxIter, DefaultMaxIterations)
	}
}

// Per-model authorization: each model carries its own provider key, composed
// into the model string. This is the case a single backend key cannot express.
func TestRelayPerModelAuthorization(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-openai")
	t.Setenv("MISTRAL_API_KEY", "sk-mistral")
	path := writeConfig(t, `
agent:
  model: gpt-4
default_backend: relay
backends:
  relay:
    models:
      gpt-4:
        authorization: $OPENAI_API_KEY
      mistral-large:
        authorization: $MISTRAL_API_KEY
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, model, _, err := cfg.Selected()
	if err != nil {
		t.Fatalf("Selected: %v", err)
	}
	if model != "gpt-4/authorization=sk-openai" {
		t.Errorf("model = %q, want gpt-4/authorization=sk-openai", model)
	}
}

// A key already inlined into the model name is respected, not double-composed.
func TestRelayInlineAuthorizationRespected(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := writeConfig(t, `
agent:
  model: 'gpt-4/authorization=sk-inline'
default_backend: relay
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, model, _, err := cfg.Selected()
	if err != nil {
		t.Fatalf("Selected: %v", err)
	}
	if model != "gpt-4/authorization=sk-inline" {
		t.Errorf("model = %q, want it unchanged", model)
	}
}

// No provider key anywhere for the relay is a clear, actionable error.
func TestRelayErrorsWithoutAuthorization(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, _, _, err := cfg.Selected(); err == nil {
		t.Fatal("expected an error when the relay model has no authorization")
	}
}

// The Bearer backends resolve to their endpoints and read their own credential.
func TestBearerBackendEndpoints(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CBK_API_SECRET", "cbk-key")
	t.Setenv("CHATBOTKIT_API_SECRET", "chatbotkit-key")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cases := map[string]struct{ url, secret string }{
		"cbk":        {"https://api.cbk.ai", "cbk-key"},
		"chatbotkit": {"https://api.chatbotkit.com", "chatbotkit-key"},
	}
	for name, want := range cases {
		cfg.DefaultBackend = name
		backend, model, _, err := cfg.Selected()
		if err != nil {
			t.Fatalf("Selected(%s): %v", name, err)
		}
		if backend.BaseURL != want.url {
			t.Errorf("%s base_url = %q, want %q", name, backend.BaseURL, want.url)
		}
		if backend.APISecret != want.secret {
			t.Errorf("%s secret = %q, want %q", name, backend.APISecret, want.secret)
		}
		// Bearer backends do not touch the model string.
		if model != DefaultModel {
			t.Errorf("%s model = %q, want %q unchanged", name, model, DefaultModel)
		}
	}
}

// A missing Bearer credential for a ChatBotKit backend is a clear error.
func TestBearerErrorsWithoutSecret(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CBK_API_SECRET", "")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.DefaultBackend = "cbk"
	if _, _, _, err := cfg.Selected(); err == nil {
		t.Fatal("expected an error when the cbk backend has no secret")
	}
}

// api_secret / authorization in the file may name an env var with $VAR.
func TestSecretEnvReference(t *testing.T) {
	t.Setenv("MY_CBK_KEY", "sk-from-env")
	path := writeConfig(t, `
default_backend: cbk
backends:
  cbk:
    api_secret: '$MY_CBK_KEY'
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Backends["cbk"].APISecret; got != "sk-from-env" {
		t.Errorf("resolved secret = %q, want sk-from-env", got)
	}
}

// Env vars override the file (defaults < file < env). CLI flags override env,
// but that layer lives in main.
func TestEnvOverridesFile(t *testing.T) {
	t.Setenv("CBK_API_SECRET", "k")
	path := writeConfig(t, `
agent:
  model: from-file
default_backend: relay
`)
	t.Setenv("ROOK_AGENT_MODEL", "from-env")
	t.Setenv("ROOK_DEFAULT_BACKEND", "cbk")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Agent.Model != "from-env" {
		t.Errorf("model = %q, want from-env (env overrides file)", cfg.Agent.Model)
	}
	if cfg.DefaultBackend != "cbk" {
		t.Errorf("default backend = %q, want cbk (env overrides file)", cfg.DefaultBackend)
	}
}

// A custom model entry aliases a real id, caps iterations, and carries auth.
func TestCustomModelAlias(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-openai")
	path := writeConfig(t, `
agent:
  model: fast
default_backend: relay
backends:
  relay:
    models:
      fast:
        model: gpt-5
        max_iterations: 50
        authorization: $OPENAI_API_KEY
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, model, maxIter, err := cfg.Selected()
	if err != nil {
		t.Fatalf("Selected: %v", err)
	}
	if model != "gpt-5/authorization=sk-openai" {
		t.Errorf("model = %q, want gpt-5/authorization=sk-openai", model)
	}
	if maxIter != 50 {
		t.Errorf("max iterations = %d, want 50 (from custom model)", maxIter)
	}
}

// Scrubbing removes every resolved credential - Bearer secrets and provider
// authorizations, backend-level and per-model - from the environment.
func TestScrubBackendSecrets(t *testing.T) {
	t.Setenv("ZAI_API_KEY", "sk-zai")
	t.Setenv("OPENAI_API_KEY", "sk-openai")
	path := writeConfig(t, `
default_backend: relay
backends:
  relay:
    authorization: $ZAI_API_KEY
    models:
      gpt-4:
        authorization: $OPENAI_API_KEY
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ScrubBackendSecrets(cfg)

	if v := os.Getenv("ZAI_API_KEY"); v != "" {
		t.Errorf("ZAI_API_KEY still present after scrub: %q", v)
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		t.Errorf("OPENAI_API_KEY still present after scrub: %q", v)
	}
}
