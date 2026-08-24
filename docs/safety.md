# Safety

> ⚠️ **Authorized use only.** Rook is an offensive-security tool. Only run it
> against systems, code and services you own or are explicitly authorized to
> test. You are responsible for staying within the scope you have permission for.

## What Rook can touch

Rook has real file read/write and shell access, and it runs commands against
whatever you point it at. The working directory (`--dir`, default the current
directory) is **not a sandbox** - the agent can read, edit and execute within
the process's own permissions. Run it against a disposable checkout or an
isolated host, not your primary environment.

Because the agent executes commands against a target, Rook takes care that a
provider credential in the environment cannot leave with one of those commands:

- Every built-in provider's conventional key variable (e.g. `OPENAI_API_KEY`) is
  unset from the environment before the agent runs.
- A built-in provider's conventional key is withheld once you set a custom
  `base_url`, so a key scoped to one host is never forwarded to another.
- A released binary does not read a `.env` from its working directory, so a
  stray committed `.env` in the code under review never reaches the process.

See [configuration.md](configuration.md) for the details.

## Bounding a run

- Point `--dir` at a disposable checkout or run inside an isolated host/VM.
- Use the objective's `rules_of_engagement` to state non-negotiable constraints
  (no network, do not modify the target, and so on) - see
  [objectives.md](objectives.md).
- Bound the run with `--max-iterations`, or `max_time` / `max_calls` in config
  (see [configuration.md](configuration.md)).
- On an air-gapped box, pre-place skills and disable the update check
  (`ROOK_UPDATE_CHECK_DISABLED=true`) so Rook reaches nothing but the model
  provider.
