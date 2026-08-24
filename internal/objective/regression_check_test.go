package objective

import (
	"testing"
)

// The full journey of the reply that killed a real draft on 2026-08-20: parse
// the summary, scaffold it, and load the scaffold back as a runnable objective.
func TestDraftedReplyScaffoldsAndReloads(t *testing.T) {
	reply := `success:
- "nuclei -u host -t injection -j" exits 0, verifying the template fired and produced JSON
- the scan results are written to findings.json
rules_of_engagement:
- read-only, no exploitation
`

	drafted, err := ParseDraft("audit the api for injection bugs", reply)
	if err != nil {
		t.Fatalf("ParseDraft: %v", err)
	}

	path, err := Scaffold(t.TempDir(), drafted)
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("the drafted scaffold does not load: %v", err)
	}

	if len(loaded.Success) != 2 || loaded.Success[0] != drafted.Success[0] {
		t.Errorf("Success = %q, want the drafted criteria surviving the round trip", loaded.Success)
	}
}
