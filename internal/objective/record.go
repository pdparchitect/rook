package objective

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Record is one completed run of an objective, kept in the dossier's ledger.
//
// Objectives are never deleted or marked up when they finish - the file is the
// contract, and mutating it to track state would destroy the thing being
// tracked. Doneness is derived instead: an objective is satisfied when the
// ledger holds a record of a successful run of this exact content. Editing the
// objective changes its hash, so an edited objective stops being satisfied and
// runs again - re-queueing is a diff, exactly like everything else in the
// dossier.
type Record struct {
	// Run identifies the run - the run id, which names its artifacts directory.
	Run string `yaml:"run"`

	// Reason is the run's stop reason. Only a settled run satisfies an objective.
	Reason string `yaml:"reason"`

	// At is when the run concluded.
	At time.Time `yaml:"at"`

	// ObjectiveSHA256 is the hash of the objective file the run executed. A
	// record only counts toward the objective content it actually ran.
	ObjectiveSHA256 string `yaml:"objective_sha256"`

	// Evidence is what the run actually did, read back from its status and event
	// logs.
	Evidence Evidence `yaml:"evidence"`
}

// Evidence is the proof half of a receipt: enough of what the run did that
// someone can judge the claim without opening the run's artifact directory.
//
// A record that says only "settled" asks to be trusted. The run already wrote
// everything needed to check it - its own closing summary, how many rounds and
// tool calls it took - so a receipt that omits them is throwing away proof it
// was handed. None of it is computed here: every field is copied from what the
// run recorded, which is what keeps a receipt evidence rather than a claim.
type Evidence struct {
	// Run is the id of the run this was read from - the pointer to the full
	// artifacts directory, for when the summary is not enough.
	Run string `yaml:"run,omitempty"`

	// Reason is how the run itself said it ended, which is not the same claim
	// as the ledger's: the ledger records that the run returned without error,
	// the agent records what it decided. They should agree, and a receipt is
	// worth little if it cannot show that they did.
	Reason string `yaml:"reason,omitempty"`

	// Summary is the run's own closing words - what it says it accomplished.
	Summary string `yaml:"summary,omitempty"`

	// Iterations and Calls are the shape of the work: an objective settled in two
	// rounds and one settled in ninety are different stories, and the
	// difference is usually the interesting part of a receipt.
	Iterations int `yaml:"iterations,omitempty"`
	Calls      int `yaml:"calls,omitempty"`

	// Missing says why there is no proof, when there is none. A receipt with
	// nothing to show has to say so: silence reads identically to a run that
	// did nothing, and a ledger that quietly implies work it cannot evidence is
	// worse than one that admits the gap.
	Missing string `yaml:"missing,omitempty"`
}

// Proven reports whether this evidence shows anything.
func (e Evidence) Proven() bool { return e.Missing == "" && e.Run != "" }

// ArtifactSummary is the shape of what rook's status.json records at the end of
// a run, read back to build evidence without coupling the objective package to
// the agent package's Status type.
type ArtifactSummary struct {
	// Reason is the agent's stop reason.
	Reason string
	// Message is the run's closing summary or failure reason.
	Message string
	// Iterations is the number of plan/act/observe cycles the run took.
	Iterations int
}

// EvidenceFromSummary builds evidence from a run's recorded summary. Nothing is
// invented: a summary that is absent, or one that recorded no outcome, yields
// evidence that says exactly that.
func EvidenceFromSummary(runID string, summary *ArtifactSummary, err error) Evidence {
	switch {
	case err != nil:
		return Evidence{Run: runID, Missing: "the run artifacts could not be read back: " + err.Error()}

	case summary == nil:
		return Evidence{Missing: "this run was not recorded to artifacts, so nothing here proves what it did"}

	case summary.Reason == "":
		return Evidence{Run: runID, Missing: "the run recorded no outcome in its artifacts"}
	}

	return Evidence{
		Run:        runID,
		Reason:     summary.Reason,
		Summary:    strings.TrimSpace(summary.Message),
		Iterations: summary.Iterations,
	}
}

// Ledger is where finished runs are recorded: one directory per objective slug
// under Root, one append-only file per run inside it.
//
// The root is the caller's to choose, because an objective is advisory input
// that may live anywhere - in the repository being assessed, in a shared folder
// of briefs, in a temp directory a dispatcher wrote it to - while the ledger is
// the operator's own record of what their tool has done. Deriving one from the
// other tied the two together: running an objective from somewhere else wrote
// the receipt somewhere else too, and an objective run from a read-only or
// throwaway location had nowhere to record at all.
type Ledger struct {
	// Root is the directory the per-objective record folders live under. Empty
	// disables the ledger: nothing is recorded, and nothing is ever satisfied,
	// which is what an embedding caller that keeps its own history wants.
	Root string
}

// dir is where this objective's records live: one folder per objective slug,
// named for the objective file rather than its content, so a run's receipts
// stay together across the edits that re-queue it.
func (l Ledger) dir(objectivePath string) string {
	slug := strings.TrimSuffix(filepath.Base(objectivePath), filepath.Ext(objectivePath))

	return filepath.Join(l.Root, slug)
}

// hashFile hashes the objective's current content.
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]), nil
}

// Record appends a run's outcome, and the evidence for it, to the ledger. One
// append-only file per run, so concurrent runs cannot conflict and history
// accumulates.
func (l Ledger) Record(o Objective, runID, reason string, at time.Time, proof Evidence) error {
	if l.Root == "" {
		return fmt.Errorf("record: no records directory configured")
	}

	if o.Path == "" {
		return fmt.Errorf("record: the objective has no path")
	}

	hash, err := hashFile(o.Path)
	if err != nil {
		return fmt.Errorf("record: %w", err)
	}

	dir := l.dir(o.Path)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("record: %w", err)
	}

	data, err := yaml.Marshal(Record{
		Run:             runID,
		Reason:          reason,
		At:              at.UTC(),
		ObjectiveSHA256: hash,
		Evidence:        proof,
	})
	if err != nil {
		return fmt.Errorf("record: %w", err)
	}

	return os.WriteFile(filepath.Join(dir, runID+".yaml"), data, 0o644)
}

// Satisfied reports whether the ledger holds a successful run of this exact
// objective content, returning the newest such record.
func (l Ledger) Satisfied(o Objective) (Record, bool) {
	if l.Root == "" || o.Path == "" {
		return Record{}, false
	}

	hash, err := hashFile(o.Path)
	if err != nil {
		return Record{}, false
	}

	dir := l.dir(o.Path)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return Record{}, false
	}

	var newest Record

	var found bool

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		var record Record

		if yaml.Unmarshal(data, &record) != nil {
			continue
		}

		if record.Reason != "settled" || record.ObjectiveSHA256 != hash {
			continue
		}

		if !found || record.At.After(newest.At) {
			newest = record
			found = true
		}
	}

	return newest, found
}

// ReadArtifactSummary reads back what a run recorded about itself, so the
// receipt carries proof rather than only a claim. The status.json the run wrote
// is the only honest source: anything reconstructed here would be rook vouching
// for rook. A run with no status file yields evidence that says so.
//
// The status file is the run's own JSON shape (the agent package's Status), read
// here through a minimal local struct so this package does not depend on the
// agent package - the objective layer is below the agent layer, not above it.
func ReadArtifactSummary(statusPath string) (*ArtifactSummary, error) {
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return nil, err
	}

	// The status file's shape is the agent.Status struct; only the fields
	// evidence needs are read, and only as untyped JSON so a field added to
	// Status later cannot break a receipt.
	var raw struct {
		State     string `json:"state"`
		ExitCode  *int   `json:"exit_code"`
		Iteration int    `json:"iteration"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	reason := "settled"
	if raw.ExitCode != nil && *raw.ExitCode != 0 {
		reason = "failed"
	}

	return &ArtifactSummary{
		Reason:     reason,
		Iterations: raw.Iteration,
	}, nil
}

// SortRecords returns records ordered newest-first by At, for listing. The
// ledger on disk is already append-only; this only orders what was read back.
func SortRecords(records []Record) []Record {
	out := make([]Record, 0, len(records))
	for _, r := range records {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}
