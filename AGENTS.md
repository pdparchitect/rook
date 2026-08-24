# AGENTS.md

rook is an autonomous offensive-security agent in a single Go binary. It wraps
the zot engine (loop, thread assembly, compaction, provider transports,
tokenizer, session logs) and the `tui/` viewer, and ships lean: the only
embedded skill is a catalog pointing the agent at external skill collections it
fetches on demand. `cmd/` is the CLI; `internal/` holds the engine wiring
(runner, status recorder, config, objectives).

## Working here

- **This is 0.x: break things freely.** Breaking changes are expected and welcome
  before 1.0. Backward compatibility is _not_ a goal - keeping a deprecated field,
  an alias, a legacy code path, or an old on-disk format "just in case" is an
  anti-pattern here: it adds surface, hides the real design, and there are no
  external users to protect yet. When you rename or replace something, rename or
  replace it everywhere and delete the old thing. Note the break in `CHANGELOG.md`
  rather than carrying it.
- **Test everything you change.** `make test`, and `make cover-check` must pass -
  total coverage may not fall below 75%. Write tests that assert _behaviour_, not
  constants; a test that restates a value it reads is worse than none.
- **Both build variants compile.** `make vet` runs `go vet` over the release and
  `-tags dev` builds. CI also runs `govulncheck`.
- **Match the surrounding code.** Same naming, comment density, and idioms.
- See `.agents/skills/` for deeper conventions (start with `testing-and-coverage`).
