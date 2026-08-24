package objective

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// writeObjectiveFile writes an objective file and loads it.
func writeObjectiveFile(t *testing.T, path, body string) Objective {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	o, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	return o
}

// Doneness is derived, never stored on the objective: a settled record of this
// exact content satisfies it, and editing the objective re-queues it because the
// hash no longer matches - re-queueing is a diff, like everything in the dossier.
func TestALedgerRecordSatisfiesTheExactObjectiveContent(t *testing.T) {
	dossier := t.TempDir()

	path := filepath.Join(dossier, DossierDir, objectivesName, "hunt-the-bugs.yaml")

	o := writeObjectiveFile(t, path, "objective: hunt the bugs\n")

	ledger := Ledger{Root: RecordsDir(dossier)}

	if _, done := ledger.Satisfied(o); done {
		t.Fatal("an objective with no records must not be satisfied")
	}

	if err := ledger.Record(o, "20260822-010101", "settled", time.Now(), Evidence{}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	record, done := ledger.Satisfied(o)

	if !done || record.Run != "20260822-010101" {
		t.Fatalf("Satisfied = %+v, %v - a settled record of this content must satisfy", record, done)
	}

	// the receipt lands in the dossier, under the objective's slug
	if _, err := os.Stat(filepath.Join(dossier, DossierDir, recordsName, "hunt-the-bugs", "20260822-010101.yaml")); err != nil {
		t.Errorf("record file not where the dossier expects it: %v", err)
	}

	// editing the objective changes its hash: no longer satisfied, runs again
	if err := os.WriteFile(path, []byte("objective: hunt the bugs properly\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, done := ledger.Satisfied(o); done {
		t.Error("an edited objective must stop being satisfied - the record is for content that no longer exists")
	}
}

// The ledger no longer derives its location from the objective's. An objective is
// advisory input and may be read from anywhere - a shared folder of briefs, a
// checkout that is not the target, a temp file a dispatcher wrote - and the
// receipt still belongs in the ledger the operator configured, which may share
// no ancestor with it at all.
func TestTheLedgerIsWhereTheCallerSaysNotBesideTheObjective(t *testing.T) {
	briefs := t.TempDir()
	elsewhere := t.TempDir()

	path := filepath.Join(briefs, "somebody-elses-tree", "the-hunt.yaml")

	o := writeObjectiveFile(t, path, "objective: the hunt\n")

	ledger := Ledger{Root: filepath.Join(elsewhere, "central-ledger")}

	if err := ledger.Record(o, "20260822-030303", "settled", time.Now(), Evidence{}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	if _, err := os.Stat(filepath.Join(elsewhere, "central-ledger", "the-hunt", "20260822-030303.yaml")); err != nil {
		t.Errorf("the record did not land in the configured ledger: %v", err)
	}

	// nothing was written anywhere near the objective
	if entries, err := os.ReadDir(briefs); err == nil {
		for _, entry := range entries {
			if entry.Name() != "somebody-elses-tree" {
				t.Errorf("the ledger wrote %q beside the objective; its location is the caller's", entry.Name())
			}
		}
	}

	// and the objective counts as done only against that same ledger
	if _, done := ledger.Satisfied(o); !done {
		t.Error("the configured ledger must satisfy the objective it recorded")
	}

	if _, done := (Ledger{Root: filepath.Join(elsewhere, "another-ledger")}).Satisfied(o); done {
		t.Error("a different ledger must not see another ledger's records")
	}
}

// Two objectives of the same name from different trees are different work. They
// share a slug, so they share a records folder - and the content hash is what
// keeps one from satisfying the other.
func TestSameNamedObjectivesDoNotSatisfyEachOther(t *testing.T) {
	root := t.TempDir()

	ledger := Ledger{Root: filepath.Join(root, "ledger")}

	mine := writeObjectiveFile(t, filepath.Join(root, "mine", "scan.yaml"), "objective: scan the api\n")
	theirs := writeObjectiveFile(t, filepath.Join(root, "theirs", "scan.yaml"), "objective: scan the docs\n")

	if err := ledger.Record(mine, "20260822-040404", "settled", time.Now(), Evidence{}); err != nil {
		t.Fatal(err)
	}

	if _, done := ledger.Satisfied(theirs); done {
		t.Error("a different objective's record must not satisfy this one, however it is named")
	}
}

// A failed run never satisfies: failure is not doneness.
func TestAFailedRecordDoesNotSatisfy(t *testing.T) {
	dossier := t.TempDir()

	o := writeObjectiveFile(t, filepath.Join(dossier, DossierDir, objectivesName, "x.yaml"), "objective: x\n")

	ledger := Ledger{Root: RecordsDir(dossier)}

	if err := ledger.Record(o, "20260822-020202", "error", time.Now(), Evidence{}); err != nil {
		t.Fatal(err)
	}

	if _, done := ledger.Satisfied(o); done {
		t.Error("a failed run must leave the objective runnable")
	}
}

// A ledger with no root keeps no history: it records nothing and satisfies
// nothing, which is what an embedding caller tracking its own runs wants. It
// must say so rather than write to a relative path in whatever directory the
// process happens to be standing in.
func TestALedgerWithNoRootRecordsNothing(t *testing.T) {
	o := writeObjectiveFile(t, filepath.Join(t.TempDir(), "y.yaml"), "objective: y\n")

	var ledger Ledger

	if err := ledger.Record(o, "20260822-050505", "settled", time.Now(), Evidence{}); err == nil {
		t.Error("recording without a records directory must be refused, not written somewhere arbitrary")
	}

	if _, done := ledger.Satisfied(o); done {
		t.Error("a ledger with no root cannot satisfy anything")
	}
}

// An objective that never was a file has no content to hash and no slug to file
// under, so it cannot enter the ledger.
func TestAnObjectiveWithNoPathCannotBeRecorded(t *testing.T) {
	ledger := Ledger{Root: t.TempDir()}

	if err := ledger.Record(Objective{Objective: "synthesized"}, "20260822-060606", "settled", time.Now(), Evidence{}); err == nil {
		t.Error("an objective with no path must be refused")
	}

	if _, done := ledger.Satisfied(Objective{Objective: "synthesized"}); done {
		t.Error("an objective with no path cannot be satisfied")
	}
}

// The dossier is one directory, so a project picks up one dotted folder rather
// than two generic top-level ones.
func TestTheDossierLayoutKeepsObjectivesAndRecordsTogether(t *testing.T) {
	project := "/srv/target"

	if got, want := ObjectivesDir(project), filepath.Join(project, ".rook", "objectives"); got != want {
		t.Errorf("ObjectivesDir = %q, want %q", got, want)
	}

	if got, want := RecordsDir(project), filepath.Join(project, ".rook", "records"); got != want {
		t.Errorf("RecordsDir = %q, want %q", got, want)
	}
}

// A receipt has to carry proof, not just a claim. "settled" alone asks to be
// trusted; the run already wrote what it did, so the record shows it - and
// shows it on disk, because a reviewer reads the file, not the struct.
func TestAReceiptCarriesTheRunsOwnEvidence(t *testing.T) {
	dossier := t.TempDir()

	o := writeObjectiveFile(t, filepath.Join(dossier, DossierDir, objectivesName, "the-hunt.yaml"), "objective: the hunt\n")

	ledger := Ledger{Root: RecordsDir(dossier)}

	proof := Evidence{
		Run:        "20260822-101010",
		Reason:     "settled",
		Summary:    "found an SSRF and a blind injection; report written to findings.md",
		Iterations: 12,
	}

	if err := ledger.Record(o, "20260822-101010", "settled", time.Now(), proof); err != nil {
		t.Fatalf("Record: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(RecordsDir(dossier), "the-hunt", "20260822-101010.yaml"))
	if err != nil {
		t.Fatalf("read the receipt: %v", err)
	}

	var written Record

	if err := yaml.Unmarshal(data, &written); err != nil {
		t.Fatalf("the receipt is not readable YAML: %v\n%s", err, data)
	}

	if written.Evidence != proof {
		t.Errorf("evidence on disk = %+v, want %+v\n%s", written.Evidence, proof, data)
	}

	// the claim and its proof travel together
	if written.Run != "20260822-101010" || written.Reason != "settled" || written.ObjectiveSHA256 == "" {
		t.Errorf("the receipt lost part of the claim: %+v", written)
	}

	// a reviewer reads the file: the run's own words have to be legible in it
	if !strings.Contains(string(data), "found an SSRF") {
		t.Errorf("the run's summary is not in the receipt:\n%s", data)
	}
}

// A receipt with nothing to show must say so. Silence reads exactly like a run
// that did nothing, and a ledger that implies work it cannot evidence is worse
// than one that admits the gap - so nothing here is ever invented.
func TestAReceiptWithNoProofSaysSo(t *testing.T) {
	tests := []struct {
		name    string
		runID   string
		summary *ArtifactSummary
		err     error
		want    string // what the receipt must own up to
	}{
		{
			name: "no artifacts at all",
			want: "not recorded to artifacts",
		},
		{
			name:  "the artifacts could not be read back",
			runID: "20260822-111111",
			err:   os.ErrNotExist,
			want:  "could not be read back",
		},
		{
			name:    "the run ended without recording an outcome",
			runID:   "20260822-111111",
			summary: &ArtifactSummary{},
			want:    "recorded no outcome",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proof := EvidenceFromSummary(test.runID, test.summary, test.err)

			if proof.Proven() {
				t.Errorf("evidence with nothing behind it claims to be proof: %+v", proof)
			}

			if !strings.Contains(proof.Missing, test.want) {
				t.Errorf("Missing = %q, want it to admit %q", proof.Missing, test.want)
			}

			// nothing invented to fill the gap
			if proof.Summary != "" || proof.Iterations != 0 {
				t.Errorf("evidence was fabricated where there was none: %+v", proof)
			}
		})
	}
}

// The evidence is copied from the run's own record, never computed here - that
// is what makes it evidence rather than rook vouching for rook.
func TestEvidenceIsCopiedFromTheRunSummary(t *testing.T) {
	proof := EvidenceFromSummary("20260822-121212", &ArtifactSummary{
		Reason:     "settled",
		Message:    "  found the injection  ",
		Iterations: 7,
	}, nil)

	want := Evidence{
		Run:        "20260822-121212",
		Reason:     "settled",
		Summary:    "found the injection",
		Iterations: 7,
	}

	if proof != want {
		t.Errorf("EvidenceFromSummary = %+v, want %+v", proof, want)
	}

	if !proof.Proven() {
		t.Error("a run that recorded its outcome has proof to show")
	}
}

// A record is settled or it is not, and evidence does not change that: an
// unsettled run's receipt still carries its proof, and Satisfied still counts
// only settled records of the exact objective content.
func TestEvidenceDoesNotMakeAnUnsettledRunCount(t *testing.T) {
	dossier := t.TempDir()

	o := writeObjectiveFile(t, filepath.Join(dossier, DossierDir, objectivesName, "the-hunt.yaml"), "objective: the hunt\n")

	ledger := Ledger{Root: RecordsDir(dossier)}

	proof := Evidence{
		Run:        "20260822-131313",
		Reason:     "max_iterations",
		Summary:    "ran out of rounds partway through the scan",
		Iterations: 40,
	}

	if err := ledger.Record(o, "20260822-131313", "max_iterations", time.Now(), proof); err != nil {
		t.Fatal(err)
	}

	if _, done := ledger.Satisfied(o); done {
		t.Error("a run that did not settle must not satisfy the objective, however much it did")
	}

	// but the receipt still says what happened - the ledger is a history, not
	// only a list of successes
	data, err := os.ReadFile(filepath.Join(RecordsDir(dossier), "the-hunt", "20260822-131313.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), "ran out of rounds") {
		t.Errorf("an unsettled run's receipt lost its evidence:\n%s", data)
	}
}

// SortRecords orders newest-first, for listing. The ledger on disk is
// append-only and unordered; listing the receipts for an objective needs them in
// the order the runs concluded, not the order the filesystem returns them.
func TestSortRecordsNewestFirst(t *testing.T) {
	now := time.Now()

	records := []Record{
		{Run: "old", At: now.Add(-2 * time.Hour)},
		{Run: "new", At: now},
		{Run: "mid", At: now.Add(-1 * time.Hour)},
	}

	sorted := SortRecords(records)

	if sorted[0].Run != "new" || sorted[1].Run != "mid" || sorted[2].Run != "old" {
		t.Errorf("SortRecords = %v, want new, mid, old", runNames(sorted))
	}
}

func runNames(records []Record) []string {
	names := make([]string, len(records))
	for i, r := range records {
		names[i] = r.Run
	}
	return names
}

// ReadArtifactSummary reads what the run recorded about itself: a done state
// with exit code 0 is settled, a nonzero exit code is failed, and the iteration
// count is carried as the shape of the work.
func TestReadArtifactSummaryReadsASettledRun(t *testing.T) {
	statusPath := filepath.Join(t.TempDir(), "status.json")

	status := map[string]interface{}{
		"state":          "done",
		"exit_code":      0,
		"iteration":      7,
		"max_iterations": 100,
	}
	data, _ := json.Marshal(status)

	if err := os.WriteFile(statusPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	summary, err := ReadArtifactSummary(statusPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if summary.Reason != "settled" {
		t.Errorf("Reason = %q, want settled for a zero exit code", summary.Reason)
	}

	if summary.Iterations != 7 {
		t.Errorf("Iterations = %d, want 7", summary.Iterations)
	}
}

// A run that exited nonzero is failed, not settled.
func TestReadArtifactSummaryReadsAFailedRun(t *testing.T) {
	statusPath := filepath.Join(t.TempDir(), "status.json")

	status := map[string]interface{}{
		"state":     "error",
		"exit_code": 1,
	}
	data, _ := json.Marshal(status)

	if err := os.WriteFile(statusPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	summary, err := ReadArtifactSummary(statusPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if summary.Reason != "failed" {
		t.Errorf("Reason = %q, want failed for a nonzero exit code", summary.Reason)
	 }
}

// A missing status file is an error, not silence: a run with no artifacts left
// nothing to read back, and the receipt must say that.
func TestReadArtifactSummaryMissingFileErrors(t *testing.T) {
	if _, err := ReadArtifactSummary(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected an error for a missing status file")
	}
}
