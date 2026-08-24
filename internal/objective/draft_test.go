package objective

import (
	"strings"
	"testing"
)

func TestParseDraftFillsTheObjective(t *testing.T) {
	o, err := ParseDraft("  hunt for SSRF  ",
		"success:\n  - every finding has a PoC\n  - the report is delivered\nrules_of_engagement:\n  - no exploitation\n")
	if err != nil {
		t.Fatalf("ParseDraft: %v", err)
	}

	if o.Objective != "hunt for SSRF" {
		t.Errorf("Objective = %q", o.Objective)
	}

	if len(o.Success) != 2 || o.Success[1] != "the report is delivered" {
		t.Errorf("Success = %q", o.Success)
	}

	if len(o.RulesOfEngagement) != 1 {
		t.Errorf("RulesOfEngagement = %q", o.RulesOfEngagement)
	}
}

// Models preface the document with prose despite being told not to; the
// document is what was asked for, so it is anchored rather than the whole
// draft failing over a pleasantry.
func TestParseDraftToleratesLeadingProse(t *testing.T) {
	o, err := ParseDraft("do it",
		"Here is the draft, based on the Makefile:\n\nsuccess:\n  - nuclei scan passes\nrules_of_engagement:\n  - no network egress\n")
	if err != nil {
		t.Fatalf("ParseDraft: %v", err)
	}

	if len(o.Success) != 1 || o.Success[0] != "nuclei scan passes" {
		t.Errorf("Success = %q", o.Success)
	}
}

// The regression that motivated the line-based format: a natural criterion
// quotes the command it names, which is prose to a YAML parser. Every
// character of the line must come through verbatim.
func TestParseDraftKeepsQuotedCommandsVerbatim(t *testing.T) {
	o, err := ParseDraft("check injection coverage",
		`success:
- "nuclei -u host -t injection" exits 0, verifying the template fired
- the scan results are saved to findings.json
rules_of_engagement:
- read-only, no exploitation
`)
	if err != nil {
		t.Fatalf("ParseDraft: %v", err)
	}

	if len(o.Success) != 2 {
		t.Fatalf("Success = %q", o.Success)
	}

	if o.Success[0] != `"nuclei -u host -t injection" exits 0, verifying the template fired` {
		t.Errorf("Success[0] = %q, want the quoted command kept verbatim", o.Success[0])
	}
}

// A model told the lines are verbatim may still wrap a whole item in quotes;
// the wrapping goes, interior quoting stays.
func TestParseDraftUnwrapsWhollyQuotedItems(t *testing.T) {
	o, err := ParseDraft("do it",
		"success:\n- \"nuclei scan passes\"\n- \"a\" and \"b\" both exist\n")
	if err != nil {
		t.Fatalf("ParseDraft: %v", err)
	}

	if o.Success[0] != "nuclei scan passes" {
		t.Errorf("Success[0] = %q, want the wrapping quotes stripped", o.Success[0])
	}

	if o.Success[1] != `"a" and "b" both exist` {
		t.Errorf("Success[1] = %q, want interior quoting untouched", o.Success[1])
	}
}

// Models wrap replies in code fences despite being told not to; a fence must
// not fail the draft.
func TestParseDraftToleratesACodeFence(t *testing.T) {
	o, err := ParseDraft("do it", "```yaml\nsuccess:\n  - it works\n```")
	if err != nil {
		t.Fatalf("ParseDraft: %v", err)
	}

	if len(o.Success) != 1 || o.Success[0] != "it works" {
		t.Errorf("Success = %q", o.Success)
	}
}

func TestParseDraftErrors(t *testing.T) {
	tests := map[string]struct{ objective, reply string }{
		"no objective":           {"   ", "success:\n  - x\n"},
		"prose without the list": {"do it", "Sure! Here are some ideas..."},
		"no criteria proposed":   {"do it", "success:\nrules_of_engagement:\n  - x\n"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDraft(test.objective, test.reply); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

// "roe:" is a natural abbreviation for rules_of_engagement, and a draft that
// uses it must still parse.
func TestParseDraftAcceptsROEAbbreviation(t *testing.T) {
	o, err := ParseDraft("do it", "success:\n- x\nroe:\n- read-only\n")
	if err != nil {
		t.Fatalf("ParseDraft: %v", err)
	}

	if len(o.RulesOfEngagement) != 1 || o.RulesOfEngagement[0] != "read-only" {
		t.Errorf("RulesOfEngagement = %q, want the abbreviated heading read", o.RulesOfEngagement)
	}
}

// The draft run's instructions are the contract that keeps a draft grounded
// and read-only: they must carry the objective, demand observable criteria,
// point at the survey tools, and name the terminal tool that delivers the
// draft.
func TestDraftInstructions(t *testing.T) {
	instructions := DraftInstructions("hunt for SSRF")

	for _, want := range []string{
		"hunt for SSRF",
		"observable",
		`"read"`,
		`"list"`,
		`"success"`,
		"Do not attempt the objective",
	} {
		if !strings.Contains(instructions, want) {
			t.Errorf("instructions are missing %q:\n%s", want, instructions)
		}
	}
}

// The survey can name the objective as well as define it: a title is a label a
// person scanning a list of objectives reads, and the model that just surveyed
// the tree is well placed to propose one.
func TestParseDraftReadsATitle(t *testing.T) {
	tests := []struct {
		name  string
		reply string
		want  string
	}{
		{
			name:  "a proposed title is read",
			reply: "title: API audit\nsuccess:\n- every finding has a PoC\n",
			want:  "API audit",
		},
		{
			name:  "however the model cases the heading",
			reply: "Title: API audit\nsuccess:\n- every finding has a PoC\n",
			want:  "API audit",
		},
		{
			name:  "quoted, as a model told about verbatim lines may do",
			reply: "title: \"API audit\"\nsuccess:\n- every finding has a PoC\n",
			want:  "API audit",
		},
		{
			name:  "a draft without one is still a good draft",
			reply: "success:\n- every finding has a PoC\n",
			want:  "",
		},
		{
			name:  "a criterion that merely mentions a title is not one",
			reply: "success:\n- title: is shown in the viewer\n- every finding has a PoC\n",
			want:  "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			drafted, err := ParseDraft("hunt for SSRF", test.reply)
			if err != nil {
				t.Fatalf("ParseDraft: %v", err)
			}

			if drafted.Title != test.want {
				t.Errorf("Title = %q, want %q", drafted.Title, test.want)
			}

			// the draft is still the draft whatever the title did
			if len(drafted.Success) == 0 {
				t.Error("the success criteria must survive title parsing")
			}
		})
	}
}

// The survey has to be asked for a title, or it will not propose one.
func TestDraftInstructionsAskForATitle(t *testing.T) {
	instructions := DraftInstructions("hunt for SSRF")

	if !strings.Contains(instructions, "title:") {
		t.Errorf("the drafting prompt does not ask for a title:\n%s", instructions)
	}
}
