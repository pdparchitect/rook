// Package objective defines the mission file: the document a rook run is
// dispatched from.
//
// rook deliberately takes no prose on the command line. A security engagement
// accepts a mission brief, not a conversation - a durable objective, the
// success criteria that define "done", and the rules of engagement the work
// must hold to. The objective is a file so it outlives the invocation: it can
// be edited, committed, re-run, and later judged against the evidence the run
// produced.
//
// An objective is advisory input - what to do - and may therefore live anywhere,
// including the repository being assessed. How the result is judged (evidence,
// findings) is deliberately not part of the objective schema: adjudication
// belongs to the operator's configuration, never to a document the agent can
// write.
package objective

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// The dossier's layout. A project's objectives and the ledger of what has been
// run from them live together under one dotted directory at its root, the way
// every other tool that keeps state in a repository does it:
// .rook/objectives/<slug>.yaml and .rook/records/<slug>/<run>.yaml. Two top-level
// objectives/ and records/ directories claimed generic names in the root of
// somebody else's project, which is not rook's to take.
//
// Only the defaults live here. An objective may be read from anywhere, and the
// ledger root is configurable - see Ledger - so this names the convention rather
// than enforcing it.
const (
	// DossierDir is the per-project directory holding both.
	DossierDir = ".rook"

	objectivesName = "objectives"
	recordsName    = "records"
)

// ObjectivesDir is where new objectives for the project rooted at dir are
// scaffolded.
func ObjectivesDir(dir string) string { return filepath.Join(dir, DossierDir, objectivesName) }

// RecordsDir is the default ledger root for the project rooted at dir.
func RecordsDir(dir string) string { return filepath.Join(dir, DossierDir, recordsName) }

// Objective is one mission brief: a single run's contract.
type Objective struct {
	// Title is an optional short label for the objective, for people rather than
	// for the agent. It never reaches the model - see Task - because the
	// objective is the contract and a title is only how a human recognises it
	// in a list or a viewer.
	Title string `yaml:"title,omitempty"`

	// Objective is the durable goal of the run: what the engagement is trying to
	// achieve. It goes into the system prompt and survives compaction, so the
	// agent cannot forget it on a long run.
	Objective string `yaml:"objective"`

	// Success are the criteria that define "done". They travel with the
	// objective into the system prompt, and they are the contract a future
	// verification gate judges the result against. For a security engagement
	// these are concrete findings: a vulnerability confirmed with a PoC, an
	// attack surface fully mapped, a report delivered.
	Success []string `yaml:"success,omitempty"`

	// RulesOfEngagement are the rules the work must hold to throughout - the
	// non-negotiable constraints on how the objective may be pursued.
	// "Read-only, no exploitation", "passive recon only", "no network egress"
	// - boundaries, not goals.
	RulesOfEngagement []string `yaml:"rules_of_engagement,omitempty"`

	// Path is where the objective was loaded from, for reporting. Empty for an
	// objective that never was a file (a synthesized one).
	Path string `yaml:"-"`
}

// List returns the objective files directly inside dir, in filename order - the
// batch a bare `rook` runs. Only the top level is listed, matching the shell
// glob the invocation is named after, and a missing directory is an empty
// listing rather than an error: a project with no dossier yet simply has no
// outstanding work.
func List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("read objectives: %w", err)
	}

	var paths []string

	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".yaml") {
			continue
		}

		paths = append(paths, filepath.Join(dir, entry.Name()))
	}

	sort.Strings(paths)

	return paths, nil
}

// Load reads and parses one objective file.
func Load(path string) (Objective, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Objective{}, fmt.Errorf("read objective: %w", err)
	}

	obj, err := Parse(data)
	if err != nil {
		return Objective{}, fmt.Errorf("objective %s: %w", path, err)
	}

	obj.Path = path

	return obj, nil
}

// Parse decodes objective YAML. Unknown fields are rejected - a typo like
// "succes:" must fail loudly rather than silently dropping the criteria the
// operator thought they set.
func Parse(data []byte) (Objective, error) {
	var obj Objective

	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)

	if err := decoder.Decode(&obj); err != nil {
		return Objective{}, fmt.Errorf("parse: %w", err)
	}

	obj.Title = strings.TrimSpace(obj.Title)
	obj.Objective = strings.TrimSpace(obj.Objective)
	obj.Success = cleanList(obj.Success)
	obj.RulesOfEngagement = cleanList(obj.RulesOfEngagement)

	if obj.Objective == "" {
		return Objective{}, fmt.Errorf("no objective")
	}

	return obj, nil
}

// FromText builds an objective from free text. Text that already is a valid
// objective document is used as one - this is what lets a dispatcher carry
// either a plain mission or a full objective in the same field - and anything
// else becomes the objective of a minimal objective.
func FromText(text string) Objective {
	if obj, err := Parse([]byte(text)); err == nil {
		return obj
	}

	return Objective{Objective: strings.TrimSpace(text)}
}

// Encode renders the objective back to YAML, for handing to another process.
func (o Objective) Encode() string {
	// Objective is plain strings and slices, which Marshal cannot fail on.
	data, _ := yaml.Marshal(o)

	return string(data)
}

// DisplayTitle is what to call this objective on screen.
//
// A declared title wins. Failing that the file name is one: objective files are
// named from their objective already, so audit-the-api-handlers.yaml is a
// perfectly good "Audit the api handlers" and deriving it costs the operator
// nothing. An objective that is neither titled nor a file - one synthesized in
// memory by a dispatcher - has no name to show, and gets none: inventing a
// label from the objective would put a truncated sentence where a title goes,
// which is the thing having titles is meant to stop.
func (o Objective) DisplayTitle() string {
	if o.Title != "" {
		return o.Title
	}

	if o.Path == "" {
		return ""
	}

	return titleFromFilename(o.Path)
}

// titleFromFilename turns an objective's file name into a label: dashes and
// underscores become spaces, and the first word is capitalised. Sentence case
// rather than Title Case, because an objective-derived name is a sentence -
// "Audit The Api Handlers" reads like a headline for something that is not one.
func titleFromFilename(path string) string {
	name := filepath.Base(path)
	name = strings.TrimSuffix(name, filepath.Ext(name))

	name = strings.Map(func(r rune) rune {
		if r == '-' || r == '_' {
			return ' '
		}

		return r
	}, name)

	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return ""
	}

	first, size := utf8.DecodeRuneInString(name)

	return string(unicode.ToUpper(first)) + name[size:]
}

// Task renders the objective as the durable text placed in the system prompt:
// the objective, then the success criteria and the rules of engagement as the
// terms the agent works - and is judged - against.
func (o Objective) Task() string {
	var b strings.Builder

	b.WriteString(o.Objective)

	if len(o.Success) > 0 {
		b.WriteString("\n\nSuccess criteria - the objective is not met until every one of these holds:")

		for i, criterion := range o.Success {
			fmt.Fprintf(&b, "\n%d. %s", i+1, criterion)
		}
	}

	if len(o.RulesOfEngagement) > 0 {
		b.WriteString("\n\nRules of engagement - these hold for the whole run:")

		for _, rule := range o.RulesOfEngagement {
			b.WriteString("\n- " + rule)
		}
	}

	return b.String()
}

// Scaffold writes the objective as a new file under dir, creating the directory
// if needed, and returns its path. Sections the objective does not fill are
// written as commented stubs that invite editing; an empty objective scaffolds
// the blank form, which will not run until it is filled in. An existing file is
// never overwritten; the name is uniquified instead, because scaffolding the
// same objective twice is routine, not an error.
func Scaffold(dir string, o Objective) (string, error) {
	o.Objective = strings.TrimSpace(o.Objective)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create objective directory: %w", err)
	}

	path := filepath.Join(dir, nameFor(o)+".yaml")
	for n := 2; exists(path); n++ {
		path = filepath.Join(dir, fmt.Sprintf("%s-%d.yaml", nameFor(o), n))
	}

	if err := os.WriteFile(path, []byte(template(o)), 0o644); err != nil {
		return "", fmt.Errorf("write objective: %w", err)
	}

	return path, nil
}

// nameFor picks the file name for an objective: its title when it has one, its
// objective otherwise.
//
// A title is a few deliberate words naming the engagement; an objective is
// however the thought arrived, and slugging one gives
// the-audit-should-cover-the-injection-endpoint.yaml - a name that is hard to
// tell apart from its neighbours in a directory listing, which is where
// objectives are actually browsed. The drafting survey proposes a title
// precisely so the file can be found by it later.
func nameFor(o Objective) string {
	if o.Title != "" {
		return slug(o.Title)
	}

	return slug(o.Objective)
}

// template renders the scaffold. The objective is a literal block scalar, which
// carries any text without quoting rules getting a say; filled lists are
// rendered by the YAML encoder for the same reason.
func template(o Objective) string {
	var b strings.Builder

	b.WriteString("# rook objective - what to do, what \"done\" means, and where you are allowed to do it.\n\n")

	// The title is optional and the file name stands in for it, so an untitled
	// objective gets a commented stub: discoverable without implying the field is
	// expected.
	if o.Title != "" {
		b.WriteString(encodeScalar("title", o.Title))
	} else {
		b.WriteString("# An optional short label for this objective, shown in the viewer. Without\n" +
			"# one the file name is used, with its dashes read as spaces.\n" +
			"# title:\n")
	}

	b.WriteString("\n")

	if o.Objective == "" {
		b.WriteString("# The durable goal of the engagement. The objective will not run until this is filled in.\nobjective:\n")
	} else {
		b.WriteString("objective: |-\n")

		for _, line := range strings.Split(o.Objective, "\n") {
			if line == "" {
				b.WriteString("\n")

				continue
			}

			b.WriteString("  " + line + "\n")
		}
	}

	b.WriteString("\n# The objective is not met until every one of these holds.\n")

	if len(o.Success) > 0 {
		b.WriteString(encodeList("success", o.Success))
	} else {
		b.WriteString(`# success:
#   - every finding is backed by a concrete reproduction (file:line, request/response)
#   - the engagement report is delivered as the run's outcome
`)
	}

	b.WriteString("\n# Rules that hold for the whole run - non-negotiable constraints on how the\n")
	b.WriteString("# objective may be pursued.\n")

	if len(o.RulesOfEngagement) > 0 {
		b.WriteString(encodeList("rules_of_engagement", o.RulesOfEngagement))
	} else {
		b.WriteString(`# rules_of_engagement:
#   - read-only analysis; do not exploit or exfiltrate
`)
	}

	return b.String()
}

// encodeScalar renders one named scalar as YAML, so any text is quoted
// correctly rather than by hand.
func encodeScalar(name, value string) string {
	// a map of plain strings, which Marshal cannot fail on
	data, _ := yaml.Marshal(map[string]string{name: value})

	return string(data)
}

// encodeList renders one named list as YAML.
func encodeList(name string, items []string) string {
	// a map of plain strings, which Marshal cannot fail on
	data, _ := yaml.Marshal(map[string][]string{name: items})

	return string(data)
}

// slug turns an objective into a filename: lower-case words joined by dashes,
// bounded so a paragraph-long objective still names a manageable file.
func slug(objective string) string {
	const maxLen = 48

	var b strings.Builder

	dash := false

	for _, r := range strings.ToLower(objective) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}

			dash = false

			b.WriteRune(r)
		default:
			dash = true
		}

		if b.Len() >= maxLen {
			break
		}
	}

	if b.Len() == 0 {
		return "objective"
	}

	return strings.TrimSuffix(b.String(), "-")
}

// cleanList trims entries and drops empty ones, so a stray "- " in the YAML
// does not become an empty criterion the agent is asked to satisfy.
func cleanList(items []string) []string {
	var out []string

	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}

	return out
}

func exists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}
