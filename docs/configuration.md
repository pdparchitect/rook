# Configuration

Configuration is layered: **built-in defaults < config file < `ROOK_*` env vars
< CLI flags**. The config file is optional - env vars alone are enough.

```bash
rook config        # opens the config in $EDITOR, creating it from a template
rook config path   # print the config file location
```

The file lives at `~/.config/rook/config.yaml` (override with `$ROOK_CONFIG` or
`--config`). Every scalar has a matching `ROOK_*` env var (`agent.model` →
`ROOK_AGENT_MODEL`, `default_provider` → `ROOK_DEFAULT_PROVIDER`). A provider's
key comes from its provider's conventional variable or `api_key` in the file,
which may be a literal or a `$VAR` reference. A developer build also reads a
`.env` from the working directory - a released one does not (see
[development.md](development.md)). See
[../configs/rook.example.yaml](../configs/rook.example.yaml) for the full,
commented template.

## Engine tuning

Under `agent:` the run's engine knobs mirror zot's, so a run is shaped the same
way in either tool. Each is optional; zero uses the built-in default.

- `max_settles` - nudges to record an outcome before a run is surfaced as unsettled
- `max_calls` - cap on total tool calls (0 = unbounded)
- `max_time` - wall-clock cap (`30m`, `2h`, `90s`); empty = unbounded
- `max_tokens` - cap on a single response's output (0 = unbounded)
- `max_tool_output` - bytes a tool result may return before truncation
- `max_continuations`, `max_recoveries`, `max_cycles`, `max_empties` - recovery/repetition bounds
- `limit_checkpoints` - percentages of a bounded limit to warn at (`[]` turns notices off)
- `context_strategy` - `compact` (summarise older history) or `truncate` (drop oldest)
- `compact_min_tokens`, `compact_min_messages`, `compact_trigger_ratio` - when compaction fires

A model entry under `providers.<name>.models.<model>` additionally takes:

- `context` - the real context window, if the endpoint's is smaller than the model's card
- `vision` - `true` if this model can be shown images (offers the agent the view tool)

Rook cannot import zot's internal model catalogue, so there is no
auto-detection; these are explicit operator overrides - the part that matters
for a custom endpoint.

## Update check

After a run, Rook asks GitHub for the latest release and prints a one-line
notice (with the upgrade command) when the binary is out of date - the only call
it makes that is not to the model provider. Disable it on an air-gapped or
locked-down host with `update_check.disabled: true`, or
`ROOK_UPDATE_CHECK_DISABLED=true` (no config file needed).

## Credential hygiene

Rook strips the resolved provider credential from the environment before the
agent runs, so the commands it executes against a target cannot read it. Every
built-in provider's conventional variable is unset by name, whichever provider
the run uses - so a provider key exported in your shell does not leak into the
agent's process. And a built-in provider's conventional key is withheld once you
set a custom `base_url`, so a key scoped to one host is never forwarded to
another.
