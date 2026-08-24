---
name: testing-and-coverage
description: How tests are written and how the 75% coverage gate is enforced in the rook repository. Read this before adding or changing tests, or when the coverage gate fails.
---

# Testing and coverage

The bar in this repo is not "there is a test" - it is "the test would fail if the
behaviour broke." Coverage is gated at 75% so that bar cannot quietly erode.

## The gate

- `make cover-check` (locally) and the **Coverage gate** CI step run the same
  script, `scripts/coverage.sh`. Total statement coverage across the module may
  not fall below **75%**.
- The script prints per-package coverage before the total, so a failure points at
  the package that dropped. Fix it by testing the code you added - not by lowering
  the threshold. `COVERAGE_THRESHOLD=80 make cover-check` raises the bar locally.
- New code without tests is the usual cause of a drop: a config field, an objective
  parser branch, a status recorder. Coverage falling is the signal that the change
  shipped untested.

## Why 75% and not 90%

zot's gate is 90% because its engine is fully testable in-process. rook wraps that
engine and adds thin packages that are partly wiring:

- `cmd/rook` - CLI entry point; `run()`, `execute()` and `startWatch()` need a
  provider or a blocking loop.
- `internal/buildinfo` - compile-time injected values.
- `internal/agent` - `Run()` boots a real model client, so only the non-provider
  paths (status recording, scope resolution, run IDs) are testable without a
  provider key.

Raise the threshold as coverage grows. The packages above are the frontier.

## What a good test looks like here

- **It asserts behaviour, never a constant.** `if MaxIterations != 10000` is not a
  test - change the constant and it "passes" for the wrong reason. Instead drive
  the behaviour the constant produces. A test that restates a value it reads is
  worse than no test, because it reads as coverage while guaranteeing nothing.
- **It bites.** Before trusting a new test, break the code it covers and confirm
  the test fails. A green test over broken code is a false negative you have now
  committed. The objective parser tests, the scaffold round-trip, and the coverage
  gate itself were all bite-checked this way.
- **It states the failure it prevents** in a sentence, usually as the comment
  above it - "a result whose call was trimmed away would be rejected", not
  "tests dropOrphaned". The comment is why the test exists; the assertion is how.
- **It is table-driven when the cases are variations** of one shape (see the
  objective parse tests), and a named function when the setup differs.

## Levels, and which to reach for

- **Behavioural** (preferred): drive the real function and assert the outcome.
  `internal/objective/objective_test.go` (parse trims whitespace, scaffold
  round-trips), `internal/config/config_test.go` (env override reaches the run).
- **Wire/serialisation**: for anything that has to reach disk, assert the file
  contents, not just the in-memory struct. The objective scaffold writes a YAML
  file that must round-trip through `Load` - setting a field that never
  serialises is a silent no-op.
- **Construction**: acceptable for defaults that a behavioural test would only
  reach slowly. `internal/agent/status_test.go` asserting the recorder writes the
  initial status struct is construction; asserting it does so after a real run
  would require a provider.

## Running

```bash
make test          # the suite
make race          # under the race detector (CI runs this)
make cover         # per-package coverage, no gate - just the numbers
make cover-check   # the gate: fails below 75%
make vet           # go vet over both the release and -tags dev builds
```
