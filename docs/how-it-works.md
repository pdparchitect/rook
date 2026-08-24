# How it works

```
cmd/rook          CLI: flags, .env, signal handling, version
internal/config   Central config: default model, max iterations, system prompt
internal/agent    Loads the catalog + on-disk skills, registers tools, drives the loop
internal/version  Build-time version + GitHub release update check
embed.go          //go:embed skills  →  the embedded catalog
skills/           the skill-catalog bootstrap (no skill library)
```

The engine is [zot](https://github.com/openzot/openzot), running in-process:
the agentic loop, thread assembly, compaction, loop detection, provider
transports and the read-only viewer all live in the zot module, pinned in
`go.mod`. Rook adds the security persona, the objective/ledger workflow, and the
skill catalog on top.

The default model and the agent's system prompt (backstory) live in one place -
[`internal/config/config.go`](../internal/config/config.go) - so they can be
tuned without touching the CLI or the agent loop.

At startup Rook loads the embedded catalog with `agent.LoadSkillsFromFS` and
layers the on-disk skills directory over it with `agent.NewSkillLoader`, which
rescans that directory each turn. Skills are read with the `read` tool - an
embedded skill through an `embedded-skill://` URL, an on-disk one through its
file path. Rook registers `agent.DefaultTools()`, builds a security-focused
backstory, and runs `agent.ExecuteWithTools` until the agent records an outcome
by calling `_success` or `_failure` (settle mode: a run ends only when it records
an outcome, never because its prose sounded conclusive).

Every run writes a session log under `.rook/sessions/`, so a run can be inspected
afterwards and resumed with `rook --resume last` (or an id, or a path). It also
writes per-run artifacts (`status.json` + `events.jsonl`) the desktop status
widget reads, and closes with an end-of-run digest carrying the session id and a
ready-to-copy resume command.
