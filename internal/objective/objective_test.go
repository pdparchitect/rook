package objective

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadReadsAFullObjective(t *testing.T) {
	path := filepath.Join(t.TempDir(), "objective.yaml")

	write(t, path, `
objective: |-
  audit the api for injection bugs
success:
  - "every finding has a PoC"
  - "  the report is delivered  "
  - ""
rules_of_engagement:
  - read-only, no exploitation
`)

	o, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if o.Objective != "audit the api for injection bugs" {
		t.Errorf("Objective = %q", o.Objective)
	}

	// entries are trimmed and empties dropped, so a stray "- " in the YAML does
	// not become a criterion the agent is asked to satisfy
	if len(o.Success) != 2 || o.Success[1] != "the report is delivered" {
		t.Errorf("Success = %q", o.Success)
	}

	if len(o.RulesOfEngagement) != 1 {
		t.Errorf("RulesOfEngagement = %q", o.RulesOfEngagement)
	}

	if o.Path != path {
		t.Errorf("Path = %q, want the file it came from", o.Path)
	}
}

func TestLoadErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.yaml")

	empty := filepath.Join(t.TempDir(), "empty.yaml")
	write(t, empty, "objective: '  '\n")

	// a typo must fail loudly rather than silently dropping what the operator
	// thought they set
	typo := filepath.Join(t.TempDir(), "typo.yaml")
	write(t, typo, "objective: x\nsucces:\n  - y\n")

	prose := filepath.Join(t.TempDir(), "prose.yaml")
	write(t, prose, "hunt the bugs in the parser\n")

	for name, path := range map[string]string{
		"a missing file":          missing,
		"an empty objective":      empty,
		"an unknown field":        typo,
		"prose, not an objective": prose,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(path); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestFromTextWrapsProse(t *testing.T) {
	o := FromText("  audit the flaky endpoint  ")

	if o.Objective != "audit the flaky endpoint" {
		t.Errorf("Objective = %q", o.Objective)
	}

	if len(o.Success) != 0 || len(o.RulesOfEngagement) != 0 {
		t.Errorf("a wrapped mission invented criteria: %+v", o)
	}
}

func TestFromTextAcceptsAFullObjective(t *testing.T) {
	o := FromText("objective: hunt the bugs\nsuccess:\n  - it finds them\n")

	if o.Objective != "hunt the bugs" {
		t.Errorf("Objective = %q", o.Objective)
	}

	if len(o.Success) != 1 || o.Success[0] != "it finds them" {
		t.Errorf("Success = %q, want the document's own criteria", o.Success)
	}
}

// Encode must round-trip through Parse: it is how an objective travels to another
// process (a sandbox, a CI job), and an encoding Parse rejects would strand it.
func TestEncodeRoundTrips(t *testing.T) {
	original := Objective{
		Objective:         "line one\nline two: with a colon",
		Success:           []string{"a: tricky { criterion }", "plain"},
		RulesOfEngagement: []string{"# not a comment"},
	}

	decoded, err := Parse([]byte(original.Encode()))
	if err != nil {
		t.Fatalf("Parse(Encode()): %v", err)
	}

	decoded.Path = original.Path

	if decoded.Objective != original.Objective {
		t.Errorf("Objective = %q, want %q", decoded.Objective, original.Objective)
	}

	if len(decoded.Success) != 2 || decoded.Success[0] != original.Success[0] {
		t.Errorf("Success = %q", decoded.Success)
	}

	if len(decoded.RulesOfEngagement) != 1 || decoded.RulesOfEngagement[0] != original.RulesOfEngagement[0] {
		t.Errorf("RulesOfEngagement = %q", decoded.RulesOfEngagement)
	}
}

func TestTaskRendersTheWholeContract(t *testing.T) {
	o := Objective{
		Objective:         "audit the api",
		Success:           []string{"every finding has a poc", "report delivered"},
		RulesOfEngagement: []string{"no network egress"},
	}

	task := o.Task()

	for _, want := range []string{
		"audit the api",
		"Success criteria",
		"1. every finding has a poc",
		"2. report delivered",
		"Rules of engagement",
		"- no network egress",
	} {
		if !strings.Contains(task, want) {
			t.Errorf("Task() is missing %q:\n%s", want, task)
		}
	}
}

func TestTaskOfABareObjectiveIsJustTheObjective(t *testing.T) {
	task := Objective{Objective: "hunt the bugs"}.Task()

	if task != "hunt the bugs" {
		t.Errorf("Task() = %q - empty sections must not render headings", task)
	}
}

// The scaffold's whole job is to produce a file rook itself will accept.
func TestScaffoldWritesALoadableObjective(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "objectives")

	path, err := Scaffold(dir, Objective{Objective: "Audit the API for bugs!"})
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	if filepath.Base(path) != "audit-the-api-for-bugs.yaml" {
		t.Errorf("path = %q", path)
	}

	o, err := Load(path)
	if err != nil {
		t.Fatalf("the scaffold does not load: %v", err)
	}

	if o.Objective != "Audit the API for bugs!" {
		t.Errorf("Objective = %q", o.Objective)
	}

	// the stub sections are comments: present to invite editing, absent from
	// the parsed objective
	if len(o.Success) != 0 || len(o.RulesOfEngagement) != 0 {
		t.Errorf("the scaffold's commented stubs leaked into the objective: %+v", o)
	}
}

func TestScaffoldCarriesAMultilineObjective(t *testing.T) {
	dir := t.TempDir()

	path, err := Scaffold(dir, Objective{Objective: "first line\n\nthird line: with a colon"})
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	o, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if o.Objective != "first line\n\nthird line: with a colon" {
		t.Errorf("Objective = %q", o.Objective)
	}
}

// Scaffolding the same objective twice is routine, not an error.
func TestScaffoldUniquifiesRatherThanOverwrites(t *testing.T) {
	dir := t.TempDir()

	first, err := Scaffold(dir, Objective{Objective: "hunt the bugs"})
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	second, err := Scaffold(dir, Objective{Objective: "hunt the bugs"})
	if err != nil {
		t.Fatalf("Scaffold again: %v", err)
	}

	if first == second {
		t.Fatalf("both scaffolds wrote %s", first)
	}

	if filepath.Base(second) != "hunt-the-bugs-2.yaml" {
		t.Errorf("second = %q", second)
	}
}

// A filled objective - a drafted one - scaffolds with its sections as real YAML,
// not commented stubs, and the file round-trips through Load.
func TestScaffoldRendersFilledSections(t *testing.T) {
	path, err := Scaffold(t.TempDir(), Objective{
		Objective:         "audit the api",
		Success:           []string{"every finding has a poc", "the: report { delivered }"},
		RulesOfEngagement: []string{"no network egress"},
	})
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("the drafted scaffold does not load: %v", err)
	}

	if len(loaded.Success) != 2 || loaded.Success[1] != "the: report { delivered }" {
		t.Errorf("Success = %q", loaded.Success)
	}

	if len(loaded.RulesOfEngagement) != 1 || loaded.RulesOfEngagement[0] != "no network egress" {
		t.Errorf("RulesOfEngagement = %q", loaded.RulesOfEngagement)
	}
}

// An empty objective scaffolds the blank form - which must refuse to run until
// it is filled in, or a forgotten edit becomes a run with no goal.
func TestScaffoldBlankForm(t *testing.T) {
	path, err := Scaffold(t.TempDir(), Objective{})
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	if filepath.Base(path) != "objective.yaml" {
		t.Errorf("path = %q", path)
	}

	if _, err := Load(path); err == nil {
		t.Error("the unedited blank form must not load")
	}
}

func TestScaffoldErrors(t *testing.T) {
	// a file where the directory should go
	blocked := filepath.Join(t.TempDir(), "objectives")
	write(t, blocked, "not a directory")

	if _, err := Scaffold(filepath.Join(blocked, "sub"), Objective{Objective: "x"}); err == nil {
		t.Error("an uncreatable directory must be an error")
	}
}

func TestSlug(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Hunt the bugs", "hunt-the-bugs"},
		{"añadir límites!!", "a-adir-l-mites"},
		{"...", "objective"},
		{strings.Repeat("very long objective ", 10), "very-long-objective-very-long-objective-very-lon"},
	}

	for _, test := range tests {
		if got := slug(test.in); got != test.want {
			t.Errorf("slug(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A title is a label for people. A declared one wins; without one the file name
// is already a perfectly good name, because objective files are named from their
// objective. Sentence case, not Title Case - the name is a sentence.
func TestDisplayTitlePrefersTheDeclaredOneThenTheFileName(t *testing.T) {
	tests := []struct {
		name string
		obj  Objective
		want string
	}{
		{
			name: "a declared title wins",
			obj:  Objective{Title: "API audit", Path: "/dossier/.rook/objectives/audit-the-api-handlers.yaml"},
			want: "API audit",
		},
		{
			name: "the file name becomes one",
			obj:  Objective{Path: "/dossier/.rook/objectives/audit-the-flaky-endpoint.yaml"},
			want: "Audit the flaky endpoint",
		},
		{
			name: "underscores read as spaces too",
			obj:  Objective{Path: "audit_the_flaky_endpoint.yaml"},
			want: "Audit the flaky endpoint",
		},
		{
			name: "a one-word name still capitalises",
			obj:  Objective{Path: "recon.yaml"},
			want: "Recon",
		},
		{
			name: "an already-capitalised name is left alone",
			obj:  Objective{Path: "API-recon.yaml"},
			want: "API recon",
		},
		{
			name: "a name that is only separators yields nothing to show",
			obj:  Objective{Path: "---.yaml"},
			want: "",
		},
		{
			name: "an objective that was never a file has no name to show",
			obj:  Objective{Objective: "a synthesized objective with a very long objective nobody wants as a title"},
			want: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.obj.DisplayTitle(); got != test.want {
				t.Errorf("DisplayTitle = %q, want %q", got, test.want)
			}
		})
	}
}

// The title is a label for humans and never reaches the model: the objective is
// the contract, and spending context on a name for it would be paying twice for
// the same words.
func TestTheTitleStaysOutOfTheTask(t *testing.T) {
	o := Objective{
		Title:    "API audit",
		Objective: "audit the api for injection bugs",
		Success:  []string{"every finding has a poc"},
	}

	if strings.Contains(o.Task(), "API audit") {
		t.Errorf("the title must not enter the task the agent is given:\n%s", o.Task())
	}

	if !strings.Contains(o.Task(), "audit the api for injection bugs") {
		t.Errorf("the objective must still be the task:\n%s", o.Task())
	}
}

// The field is optional in both directions: an objective without one parses as it
// always did, and one with a title round-trips through the file.
func TestTitleIsOptionalAndRoundTrips(t *testing.T) {
	plain, err := Parse([]byte("objective: do the thing\n"))
	if err != nil {
		t.Fatalf("an objective without a title must parse: %v", err)
	}

	if plain.Title != "" {
		t.Errorf("Title = %q, want empty", plain.Title)
	}

	titled, err := Parse([]byte("title: The Thing\nobjective: do the thing\n"))
	if err != nil {
		t.Fatalf("an objective with a title must parse: %v", err)
	}

	if titled.Title != "The Thing" {
		t.Errorf("Title = %q, want %q", titled.Title, "The Thing")
	}

	// through a file and back
	dir := t.TempDir()

	path, err := Scaffold(dir, titled)
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("a scaffolded titled objective does not load: %v", err)
	}

	if reloaded.Title != "The Thing" {
		t.Errorf("the title did not survive the file: %q", reloaded.Title)
	}
}

// An untitled scaffold advertises the field without implying it is expected -
// a stub nobody has to fill in, because the file name already names the objective.
func TestAnUntitledScaffoldMentionsTheField(t *testing.T) {
	dir := t.TempDir()

	path, err := Scaffold(dir, Objective{Objective: "do the thing"})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), "# title:") {
		t.Errorf("the scaffold should show the optional title field:\n%s", data)
	}

	// commented out, so it stays optional and the file still loads
	if _, err := Load(path); err != nil {
		t.Errorf("the scaffold must still load: %v", err)
	}
}

// A drafted objective is named by its title, not its objective. An objective is
// however the thought arrived; slugging one gives
// the-new-command-should-have-an-interactive-versi.yaml, which is hard to tell
// from its neighbours in the directory where objectives are actually browsed.
func TestScaffoldNamesTheFileByTitleWhenThereIsOne(t *testing.T) {
	dir := t.TempDir()

	titled, err := Scaffold(dir, Objective{
		Title:     "Interactive rook new",
		Objective: "the new command should have an interactive version where I can continuously type new mission briefs that get recorded as objectives",
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := filepath.Base(titled); got != "interactive-rook-new.yaml" {
		t.Errorf("file name = %q, want it named from the title", got)
	}

	// without a title the objective still names it, exactly as before
	untitled, err := Scaffold(dir, Objective{Objective: "audit the flaky endpoint"})
	if err != nil {
		t.Fatal(err)
	}

	if got := filepath.Base(untitled); got != "audit-the-flaky-endpoint.yaml" {
		t.Errorf("file name = %q, want it named from the objective", got)
	}

	// and a name already taken is still uniquified rather than overwritten
	again, err := Scaffold(dir, Objective{Title: "Interactive rook new", Objective: "something else"})
	if err != nil {
		t.Fatal(err)
	}

	if again == titled {
		t.Error("scaffolding the same title twice must not overwrite the first")
	}

	if got := filepath.Base(again); got != "interactive-rook-new-2.yaml" {
		t.Errorf("the second file is %q, want it uniquified from the title", got)
	}
}

// List returns the objective files directly inside dir, in filename order -
// the batch a bare `rook` runs. Only the top level is listed, and a missing
// directory is an empty listing rather than an error: a project with no dossier
// yet simply has no outstanding work.
func TestListReturnsYAMLInFilenameOrder(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"b.yaml", "a.yaml", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("objective: x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	paths, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(paths) != 2 {
		t.Fatalf("got %d paths, want 2 (yaml only, sorted)", len(paths))
	}

	if filepath.Base(paths[0]) != "a.yaml" || filepath.Base(paths[1]) != "b.yaml" {
		t.Errorf("List = %v, want sorted YAML only", paths)
	}
}

// A missing directory is not an error: a project with no dossier yet simply
// has no outstanding work.
func TestListOnAMissingDirectoryIsEmpty(t *testing.T) {
	paths, err := List(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("List on missing dir: %v", err)
	}

	if len(paths) != 0 {
		t.Errorf("got %d paths, want 0", len(paths))
	}
}
