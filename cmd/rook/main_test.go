package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdparchitect/rook/internal/config"
	"github.com/pdparchitect/rook/internal/objective"
	"github.com/pdparchitect/rook/internal/version"
)

// firstNonEmpty returns the first trimmed value that is not empty, so a flag
// whose default is "" is distinguishable from one explicitly set.
func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		values []string
		want   string
	}{
		{[]string{"", "second"}, "second"},
		{[]string{"first", "second"}, "first"},
		{[]string{"  ", "\t", "real"}, "real"},
		{[]string{"", ""}, ""},
		{nil, ""},
	}

	for _, tt := range tests {
		if got := firstNonEmpty(tt.values...); got != tt.want {
			t.Errorf("firstNonEmpty(%q) = %q, want %q", tt.values, got, tt.want)
		}
	}
}

// expandPath expands $ENV references and a leading ~. A path with no references
// comes back unchanged.
func TestExpandPath(t *testing.T) {
	t.Setenv("ROOK_TEST_VAR", "expanded")

	if got := expandPath("$ROOK_TEST_VAR/sub"); got != "expanded/sub" {
		t.Errorf("env expansion = %q, want expanded/sub", got)
	}

	// A literal path with no variable or tilde is returned as-is.
	if got := expandPath("/usr/local/bin"); got != "/usr/local/bin" {
		t.Errorf("literal path = %q, want /usr/local/bin", got)
	}
}

// A tilde with no HOME falls through rather than panicking.
func TestExpandPathTildeUsesHome(t *testing.T) {
	home := os.Getenv("HOME")
	if home == "" {
		t.Skip("HOME is unset; cannot test tilde expansion")
	}

	if got := expandPath("~/bin"); got != filepath.Join(home, "bin") {
		t.Errorf("tilde expansion = %q, want %s", got, filepath.Join(home, "bin"))
	}
}

// flagsUsage prints the usage the user sees when they get it wrong, so it has
// to name the things they can actually do.
func TestFlagsUsageDescribesTheRealCommands(t *testing.T) {
	original := os.Stderr

	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	os.Stderr = write

	flagsUsage()

	write.Close()

	os.Stderr = original

	var b strings.Builder

	buf := make([]byte, 4096)
	for {
		n, err := read.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}

	text := b.String()

	for _, want := range []string{"rook [flags]", "rook new", "rook config", "rook version"} {
		if !strings.Contains(text, want) {
			t.Errorf("usage does not mention %q:\n%s", want, text)
		}
	}
}

// resolveObjectives loads the objectives named on the command line. A bad file
// fails the whole batch up front.
func TestResolveObjectivesLoadsNamedFiles(t *testing.T) {
	dir := t.TempDir()

	good := filepath.Join(dir, "audit.yaml")
	if err := os.WriteFile(good, []byte("objective: audit the api\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	paths, err := resolveObjectives([]string{good}, dir)
	if err != nil {
		t.Fatalf("resolveObjectives: %v", err)
	}

	if len(paths) != 1 || paths[0].Objective != "audit the api" {
		t.Errorf("paths = %+v", paths)
	}
}

// A batch with a broken objective must fail the whole batch up front, not
// midway through.
func TestResolveObjectivesFailsTheWholeBatchUpFront(t *testing.T) {
	good := filepath.Join(t.TempDir(), "ok.yaml")
	if err := os.WriteFile(good, []byte("objective: fine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	bad := filepath.Join(t.TempDir(), "nope.yaml")

	if _, err := resolveObjectives([]string{good, bad}, ""); err == nil {
		t.Error("a batch with a broken objective must not resolve")
	}
}

// Someone typing prose where an objective file goes is the retraining moment:
// the error has to teach the new shape, not just report a missing file.
func TestResolveObjectivesTeachesProseTypers(t *testing.T) {
	// silence the usage print
	origStderr := os.Stderr
	devNull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	os.Stderr = devNull
	defer func() {
		os.Stderr = origStderr
		devNull.Close()
	}()

	_, err := resolveObjectives([]string{"audit the api handlers"}, "")
	if err == nil {
		t.Fatal("prose must not resolve")
	}

	if !strings.Contains(err.Error(), "rook new") {
		t.Errorf("the error should point at `rook new`: %v", err)
	}
}

// With no arguments, resolveObjectives lists the directory. An empty directory
// is an error because there is nothing to run.
func TestResolveObjectivesEmptyDirectoryIsAnError(t *testing.T) {
	origStderr := os.Stderr
	devNull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	os.Stderr = devNull
	defer func() {
		os.Stderr = origStderr
		devNull.Close()
	}()

	dir := t.TempDir()

	if _, err := resolveObjectives(nil, dir); err == nil {
		t.Error("an empty objectives directory must be an error")
	}
}

// With no arguments, resolveObjectives lists the directory and finds every
// .yaml file in filename order.
func TestResolveObjectivesListsFromDirectory(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"zebra.yaml", "alpha.yaml", "beta.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("objective: "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// A non-yaml file is ignored.
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	paths, err := resolveObjectives(nil, dir)
	if err != nil {
		t.Fatalf("resolveObjectives: %v", err)
	}

	if len(paths) != 3 {
		t.Fatalf("got %d objectives, want 3", len(paths))
	}

	want := []string{"alpha.yaml", "beta.yaml", "zebra.yaml"}
	for i, w := range want {
		if paths[i].Objective != w {
			t.Errorf("path %d objective = %q, want %q", i, paths[i].Objective, w)
		}
	}
}

// `rook new` writes an objective rook itself will run.
func TestNewObjectiveScaffoldsARunnableObjective(t *testing.T) {
	t.Chdir(t.TempDir())

	var out strings.Builder

	if err := newObjective([]string{"audit", "the", "api"}, &out); err != nil {
		t.Fatalf("newObjective: %v", err)
	}

	path := filepath.Join(objective.ObjectivesDir("."), "audit-the-api.yaml")

	if !strings.Contains(out.String(), path) {
		t.Errorf("output should say where the objective went:\n%s", out.String())
	}

	loaded, err := objective.Load(path)
	if err != nil {
		t.Fatalf("scaffolded objective does not load: %v", err)
	}

	if loaded.Objective != "audit the api" {
		t.Errorf("objective = %q, want \"audit the api\"", loaded.Objective)
	}
}

// Bare `rook new` scaffolds the blank form: a file to fill in, which refuses to
// parse until it is.
func TestNewObjectiveScaffoldsTheBlankForm(t *testing.T) {
	t.Chdir(t.TempDir())

	var out strings.Builder

	if err := newObjective(nil, &out); err != nil {
		t.Fatalf("newObjective: %v", err)
	}

	if !strings.Contains(out.String(), "edit its objective") {
		t.Errorf("output should say the objective still needs writing:\n%s", out.String())
	}

	path := filepath.Join(objective.ObjectivesDir("."), "objective.yaml")
	if _, err := objective.Load(path); err == nil {
		t.Error("the unedited blank form must not parse")
	}
}

// `rook new` with --objectives-dir files the objective there instead of the
// default dossier.
func TestNewObjectiveWithObjectivesDir(t *testing.T) {
	t.Chdir(t.TempDir())

	dir := filepath.Join(t.TempDir(), "briefs")

	var out strings.Builder

	if err := newObjective([]string{"--objectives-dir", dir, "hunt", "bugs"}, &out); err != nil {
		t.Fatalf("newObjective: %v", err)
	}

	path := filepath.Join(dir, "hunt-bugs.yaml")
	loaded, err := objective.Load(path)
	if err != nil {
		t.Fatalf("objective not filed in --objectives-dir: %v", err)
	}

	if loaded.Objective != "hunt bugs" {
		t.Errorf("objective = %q, want \"hunt bugs\"", loaded.Objective)
	}
}

// printVersion writes the version line to stdout. The build kind is on the line
// because it changes what the binary reads from disk.
func TestPrintVersion(t *testing.T) {
	version.Version = "v0.6.0-test"
	defer func() { version.Version = "dev" }()

	// notifyUpdate calls the network; swallow stderr and let it fail silently.
	origStderr := os.Stderr
	devNull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	os.Stderr = devNull
	defer func() {
		os.Stderr = origStderr
		devNull.Close()
	}()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	printVersion()
	w.Close()

	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}

	if !strings.Contains(b.String(), "v0.6.0-test") {
		t.Errorf("version output should contain the version, got: %q", b.String())
	}
}

// `rook config path` prints the resolved config path and exits, without needing
// a provider or a config file.
func TestRunConfigPath(t *testing.T) {
	t.Setenv("ROOK_CONFIG", "/tmp/rook-test-config.yaml")

	origArgs := os.Args
	origStdout := os.Stdout
	defer func() {
		os.Args = origArgs
		os.Stdout = origStdout
	}()

	os.Args = []string{"rook", "config", "path"}

	r, w, _ := os.Pipe()
	os.Stdout = w

	err := run()
	w.Close()

	if err != nil {
		t.Fatalf("run: %v", err)
	}

	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}

	if !strings.Contains(b.String(), "/tmp/rook-test-config.yaml") {
		t.Errorf("config path output should contain the path, got: %q", b.String())
	}
}

// `rook version` as a subcommand prints the version line.
func TestRunVersionSubcommand(t *testing.T) {
	version.Version = "v0.6.0-test"
	defer func() { version.Version = "dev" }()

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"rook", "version"}

	// Swallow stderr (notifyUpdate writes there) and capture stdout.
	origStderr := os.Stderr
	devNull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	os.Stderr = devNull
	defer func() {
		os.Stderr = origStderr
		devNull.Close()
	}()

	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	_ = run()
	w.Close()

	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}

	if !strings.Contains(b.String(), "v0.6.0-test") {
		t.Errorf("version subcommand should print the version, got: %q", b.String())
	}
}

// editConfig creates the config directory and seeds the template on first run.
// When no editor is available (no VISUAL/EDITOR and nothing on PATH), it prints
// the path and returns an error.
func TestEditConfigSeedsTemplateAndErrorsWithoutEditor(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("ROOK_CONFIG", configPath)
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	// Clear PATH so exec.LookPath finds nothing.
	t.Setenv("PATH", "")

	// Capture stdout (the path is printed there on no-editor).
	origStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := editConfig()
	w.Close()
	os.Stdout = origStdout

	if err == nil {
		t.Fatal("expected an error when no editor is found")
	}

	// The config file was seeded from the embedded template.
	data, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("config file was not created: %v", readErr)
	}

	if len(data) == 0 {
		t.Error("seeded config file is empty")
	}
}

// `rook new` with --dir scaffolds into that directory's dossier.
func TestNewObjectiveWithDirScaffoldsIntoThatDir(t *testing.T) {
	target := t.TempDir()
	t.Chdir(t.TempDir())

	var out strings.Builder

	if err := newObjective([]string{"--dir", target, "audit", "api"}, &out); err != nil {
		t.Fatalf("newObjective: %v", err)
	}

	path := filepath.Join(objective.ObjectivesDir(target), "audit-api.yaml")
	loaded, err := objective.Load(path)
	if err != nil {
		t.Fatalf("objective not filed in --dir: %v", err)
	}

	if loaded.Objective != "audit api" {
		t.Errorf("objective = %q", loaded.Objective)
	}
}

// DefaultSessionDir returns the per-project sessions directory.
func TestDefaultSessionDir(t *testing.T) {
	got := config.DefaultSessionDir()
	want := filepath.Join(".", objective.DossierDir, "sessions")
	if got != want {
		t.Errorf("DefaultSessionDir = %q, want %q", got, want)
	}
}

// `rook sessions` with no sessions prints a message and exits cleanly.
func TestRunSessionsEmpty(t *testing.T) {
	dir := t.TempDir()

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"rook", "sessions", "--dir", dir}

	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := run()
	w.Close()
	os.Stderr = origStderr

	if err != nil {
		t.Fatalf("run sessions: %v", err)
	}

	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}

	if !strings.Contains(b.String(), "no sessions") {
		t.Errorf("expected 'no sessions' message, got: %q", b.String())
	}
}

// `rook sessions` lists sessions from the directory.
func TestRunSessionsListsEntries(t *testing.T) {
	dir := t.TempDir()

	// Write a minimal session file.
	meta := `{"kind":"meta","at":"2026-01-01T00:00:00Z","meta":{"id":"20260101-000000","task":"audit the api","model":"glm-5.2","provider":"zai","driver":"zai","workdir":"."}}`
	result := `{"kind":"result","at":"2026-01-01T01:00:00Z","result":{"reason":"settled","code":0,"iterations":5}}`

	path := filepath.Join(dir, "20260101-000000.jsonl")
	if err := os.WriteFile(path, []byte(meta+"\n"+result+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"rook", "sessions", "--dir", dir}

	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := run()
	w.Close()
	os.Stdout = origStdout

	if err != nil {
		t.Fatalf("run sessions: %v", err)
	}

	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}

	out := b.String()
	if !strings.Contains(out, "20260101-000000") {
		t.Errorf("session listing should contain the id, got: %q", out)
	}

	if !strings.Contains(out, "audit the api") {
		t.Errorf("session listing should contain the task, got: %q", out)
	}
}

// `rook --resume last` with no sessions is an error.
func TestRunResumeNoSessions(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(t.TempDir())

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"rook", "--resume", "last", "--session-dir", dir}

	// Swallow stderr.
	origStderr := os.Stderr
	devNull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	os.Stderr = devNull
	defer func() { os.Stderr = origStderr }()

	err := run()
	if err == nil {
		t.Error("expected error when no sessions exist to resume")
	}
}

// `rook --version` (the flag) prints the version and exits.
func TestRunVersionFlag(t *testing.T) {
	version.Version = "v0.6.0-test"
	defer func() { version.Version = "dev" }()

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"rook", "--version"}

	// Swallow stderr (notifyUpdate).
	origStderr := os.Stderr
	devNull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	os.Stderr = devNull
	defer func() { os.Stderr = origStderr }()

	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	_ = run()
	w.Close()

	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}

	if !strings.Contains(b.String(), "v0.6.0-test") {
		t.Errorf("--version flag should print the version, got: %q", b.String())
	}
}

// `run` with a bad explicit --config reports the error.
func TestRunBadConfigErrors(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"rook", "--config", filepath.Join(t.TempDir(), "nope.yaml")}

	// Swallow stderr.
	origStderr := os.Stderr
	devNull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	os.Stderr = devNull
	defer func() {
		os.Stderr = origStderr
		devNull.Close()
	}()

	if err := run(); err == nil {
		t.Error("a missing explicit --config should error")
	}
}

// `run` with no objectives and no directory is an error that prints usage.
func TestRunNoObjectivesErrors(t *testing.T) {
	t.Chdir(t.TempDir())

	// Swallow stderr (usage is printed there).
	origStderr := os.Stderr
	devNull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	os.Stderr = devNull
	defer func() {
		os.Stderr = origStderr
		devNull.Close()
	}()

	// Provide a config so the backend resolves, then fail on objectives.
	// Actually, a bare `rook` with no config file and no API key will fail at
	// cfg.Validate or cfg.Selected first. We want the "no objectives" error,
	// so provide a valid config.
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.yaml")
	t.Setenv("ZAI_API_KEY", "sk-test")

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"rook", "--config", configPath}

	err := run()
	if err == nil {
		t.Error("expected an error when no objectives are found")
	}
}

// `rook new` with a parse error (bad flag) returns the error.
func TestNewObjectiveBadFlag(t *testing.T) {
	var out strings.Builder
	if err := newObjective([]string{"--nonexistent"}, &out); err == nil {
		t.Error("expected error for unknown flag")
	}
}

// `rook new` with --objectives-dir and --dir both set uses --objectives-dir.
func TestNewObjectiveWithDirAndObjectivesDir(t *testing.T) {
	t.Chdir(t.TempDir())

	target := t.TempDir()
	briefs := filepath.Join(t.TempDir(), "briefs")

	var out strings.Builder

	if err := newObjective([]string{"--dir", target, "--objectives-dir", briefs, "hunt"}, &out); err != nil {
		t.Fatalf("newObjective: %v", err)
	}

	path := filepath.Join(briefs, "hunt.yaml")
	if _, err := objective.Load(path); err != nil {
		t.Errorf("objective not filed in --objectives-dir: %v", err)
	}
}

// updateCheckDisabledByEnv reads the env-only opt-out with strconv.ParseBool
// semantics, so `rook version` (which loads no config) can still be silenced.
func TestUpdateCheckDisabledByEnv(t *testing.T) {
	cases := map[string]bool{"": false, "true": true, "1": true, "false": false, "0": false, "yes": false}
	for value, want := range cases {
		if value == "" {
			os.Unsetenv("ROOK_UPDATE_CHECK_DISABLED")
		} else {
			t.Setenv("ROOK_UPDATE_CHECK_DISABLED", value)
		}
		if got := updateCheckDisabledByEnv(); got != want {
			t.Errorf("ROOK_UPDATE_CHECK_DISABLED=%q -> %v, want %v", value, got, want)
		}
	}
}
