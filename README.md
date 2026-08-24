# Rook

<img width="1672" height="941" alt="image" src="https://github.com/user-attachments/assets/0900d959-a741-4212-a8b8-bedd5eca3203" />

**Rook** is an AI bug-hunting harness for vulnerability research, bug hunting and
source-code auditing. It is a single Go executable that drives a model through
the whole hunt: the autonomous engine ([zot](https://github.com/openzot/openzot))
runs inside the binary and talks straight to a model provider - no hosted
service, no setup beyond a provider key. Rook ships lean: the security skills
that guide a hunt live in external collections it fetches on demand, so the
library grows and stays current without a new release.

Give Rook an objective and it works through the problem the way a
researcher would.

> ⚠️ **Authorized use only.** Rook is an offensive-security tool. Only run it
> against systems, code and services you own or are explicitly authorized to
> test.

## What you get

Point Rook at a target and walk away. It works the problem the way
a researcher would - enumerating, reading code, probing, and chaining what it
finds - then hands back concrete findings: what the issue is, where it lives, how
to reproduce it, and why it matters. Not a checklist of maybes, but the ones it
worked through to a conclusion, with the evidence to back them - and a full log
of everything it tried along the way.

## What can it do?

A single binary and an objective file. Write the mission,
walk away, come back to findings. A few examples of the kinds of objectives
Rook works through - each guided by [skills](#skills) it fetches on demand:

```bash
# Reverse-engineer an entire binary or firmware image - recover structure,
# embedded secrets, and the attack surface it exposes
rook new "Reverse engineer the firmware image in ./firmware.bin: recover embedded credentials, map services, and identify remotely reachable bugs"

# Full engagement: gain a foothold and map how deep access goes
rook new "Gain initial access to the target network from the external surface, then map lateral-movement paths to the domain controller"

# Whole-codebase audit - not one endpoint, the whole project
rook new "Audit the entire ./monorepo for the full OWASP taxonomy: injection, broken access control, auth flaws, crypto misuse, and unsafe deserialization"

# Cloud compromise assessment - find the path from a foothold to data exfiltration
rook new "Assess the AWS environment for paths to privilege escalation; chain IAM, S3 and IMDS exposure into a full data-exfiltration chain"

# External attack-surface mapping
rook new "Map example.com's external surface: subdomains, exposed services, leaked credentials, and anything that should not be internet-facing"

# Smart-contract security review - the whole protocol, not one function
rook new "Audit the Solidity protocol in ./contracts for reentrancy, access-control, oracle manipulation and economic attacks across every contract"
```

Each `rook new` drops a file under `.rook/objectives/`. Edit it to set the
success criteria, then a bare `rook` runs every outstanding objective in the
dossier - skipping what the ledger already records as done. `--watch` turns the
folder into a drop box.

```bash
rook                                          # run every outstanding objective
rook .rook/objectives/firmware-recon.yaml   # run one by name
rook --watch                                  # drop-box mode
```

The skills a hunt can draw on cover far more - OAuth/SAML/JWT flaws, file-upload
and SSTI/RCE chains, business-logic and race conditions, HTTP request smuggling,
and enterprise identity/infrastructure attack surfaces (M365/Entra, Okta, VPN
appliances, vCenter, SharePoint) - fetched from a collection when the objective
needs them. See [Skills](#skills).

## Why Rook?

Security work happens in awkward places - a hardened bastion, an air-gapped
network, a throwaway cloud VM, a CI runner, someone else's laptop during an
engagement. Rook is built for exactly those:

- **One single executable.** Everything that runs a hunt - the agent loop and
  the tools - is compiled into one binary via Go's `embed`. There is no runtime
  to install, no interpreter, no `node_modules`, no virtualenv, no config files
  to ship alongside it. Download one file, `chmod +x`, run.
- **Portable everywhere.** Statically linked (`CGO_ENABLED=0`) and
  cross-compiled for Linux, macOS and Windows on both amd64 and arm64. The same
  tool drops onto an Apple-silicon laptop, an x86 server, or an ARM box with no
  changes. Nothing to match against the host's libraries or OS version.
- **Skills fetched on demand, or pre-placed.** Rook ships with a catalog that
  names external skill collections and clones them into `~/.config/rook/skills`
  when a hunt needs them. For a locked-down or air-gapped box, drop a skills
  directory there ahead of time and Rook runs with no fetch at all - the skills
  are just files it reads. The binary itself carries no third-party skill
  content, so there is nothing baked in to go stale or to carry someone else's
  license.
- **The engine runs in the binary.** The reasoning and tool-execution loop,
  thread management, compaction and loop detection all run in-process. Nothing
  about a run depends on a service staying up, and a run is reproducible offline.
  You point Rook at whichever provider you already pay for.
- **Trivial to distribute and audit.** A single artifact with a published
  checksum is easy to vet, copy onto a target box, version-pin, and remove
  cleanly afterwards - important when you're operating inside someone else's
  environment.
- **Purpose-built, not a general chatbot.** Rook is a focused bug-hunting
  harness: it knows the methodology, the bug classes, and the reporting
  discipline out of the box, and stays within the authorization boundary you
  give it.

In short: the value isn't just "an AI security tool" - it's an AI bug-hunting
harness you can carry anywhere as **one file** and run with **zero setup**.

## Features

- **Single self-contained binary.** The engine and tools are compiled into the
  executable via Go's `embed`, so it ships and runs as one file.
- **Objectives, not prompts.** A mission file - the durable goal, success
  criteria, and rules of engagement - is the contract a
  run is dispatched from. `rook new "the objective"` scaffolds one; a bare
  `rook` runs every outstanding objective; `--watch` turns the folder into a
  drop box. A ledger records what has run, so finished work is skipped and
  edited objectives are re-queued.
- **Autonomous agent loop.** Built on zot's engine
  (`agent.ExecuteWithTools`) - the agent plans, acts, tracks progress and
  exits on its own, bounded by `--max-iterations` and settle mode (a run ends
  only when it records an outcome, never because its prose sounded conclusive).
- **Built-in tools.** File read/write/edit and shell execution via the
  engine's `DefaultTools`.
- **On-demand skills.** A tiny embedded catalog points the agent at external
  skill collections; it clones what an objective needs into a local directory
  and reads from there, so the library can grow without a rook release (see
  [Skills](#skills)).
- **Cross-platform releases.** GitHub Actions builds binaries for Linux, macOS
  and Windows (amd64/arm64) on every tag.

## Install

### One-line installer (recommended)

```bash
curl -fsSL https://github.com/pdparchitect/rook/releases/latest/download/install.sh | bash
```

Installs the latest release for your platform (Linux/macOS, amd64/arm64),
verifies it against the release checksums, and puts `rook` in `~/.local/bin`.
The same command upgrades in place, and `rook`'s own update notice prints it
when a newer release exists. Pin a version or change the directory with
`ROOK_VERSION` / `ROOK_INSTALL_DIR`, or pass a tag: `... | bash -s -- v0.6.0`.
The installer is `install.sh` in this repo, published as a release asset.

### From a release archive

Prebuilt, self-contained binaries are published for every release on the
[releases page](https://github.com/pdparchitect/rook/releases), for Linux, macOS
and Windows on both amd64 and arm64. Each archive contains a single `rook`
binary (plus README and LICENSE), and a `checksums.txt` is published alongside.

Pick the archive for your platform - e.g. `rook-v0.1.0-linux-amd64.tar.gz` - then
download, (optionally) verify, extract and put `rook` on your `PATH`:

```bash
VERSION=v0.1.0
OS=linux       # linux | darwin | windows
ARCH=amd64     # amd64 | arm64
BASE="https://github.com/pdparchitect/rook/releases/download/${VERSION}"

# download the archive and checksums
curl -sSLO "${BASE}/rook-${VERSION}-${OS}-${ARCH}.tar.gz"
curl -sSLO "${BASE}/checksums.txt"

# verify (optional but recommended)
sha256sum --ignore-missing -c checksums.txt

# extract and install
tar -xzf "rook-${VERSION}-${OS}-${ARCH}.tar.gz"
sudo mv "rook-${VERSION}-${OS}-${ARCH}/rook" /usr/local/bin/rook

rook version
```

On Windows, download `rook-<version>-windows-amd64.tar.gz`, extract it, and add
`rook.exe` to a directory on your `PATH`.

### From source

```bash
go install github.com/pdparchitect/rook/cmd/rook@latest
```

Or clone and build with the provided `Makefile`:

```bash
make build      # → ./rook
```

## Providers

A run targets a **provider** - the model provider Rook talks to. Rook speaks to
each one directly over the OpenAI-compatible API; there is no gateway and no
account in between, so all you need is a provider key. Pick a provider with
`--provider`, or set `default_provider` in config.

| Provider     | Endpoint                         | Credential from      |
| ------------ | -------------------------------- | -------------------- |
| `zai`        | `https://api.z.ai/api/paas/v4`   | `ZAI_API_KEY`        |
| `openai`     | `https://api.openai.com/v1`      | `OPENAI_API_KEY`     |
| `anthropic`  | `https://api.anthropic.com/v1`   | `ANTHROPIC_API_KEY`  |
| `groq`       | `https://api.groq.com/openai/v1` | `GROQ_API_KEY`       |
| `mistral`    | `https://api.mistral.ai/v1`      | `MISTRAL_API_KEY`    |
| `deepseek`   | `https://api.deepseek.com/v1`    | `DEEPSEEK_API_KEY`   |
| `openrouter` | `https://openrouter.ai/api/v1`   | `OPENROUTER_API_KEY` |
| `together`   | `https://api.together.xyz/v1`    | `TOGETHER_API_KEY`   |
| `cerebras`   | `https://api.cerebras.ai/v1`     | `CEREBRAS_API_KEY`   |
| `xai`        | `https://api.x.ai/v1`            | `XAI_API_KEY`        |
| `moonshot`   | `https://api.moonshot.cn/v1`     | `MOONSHOT_API_KEY`   |
| `qwen`       | DashScope compatible mode        | `DASHSCOPE_API_KEY`  |
| `ollama`     | `http://localhost:11434/v1`      | none (local)         |

Rook defaults to **`zai`** running **`glm-5.2`** - a strong open model for
bug-hunting work: large context for reading codebases, and permissive for
offensive tasks. The model must be one the chosen provider serves.

The common case is one exported variable and nothing else:

```bash
export ZAI_API_KEY="sk-..."
rook new "Reverse engineer ./firmware.bin and identify remotely reachable bugs"
# edit .rook/objectives/reverse-engineer-firmware-bin-and-identify-.yaml
# to set the success criteria, then:
rook
```

Switch provider with a flag:

```bash
export OPENAI_API_KEY="sk-..."
rook --provider openai --model gpt-5 "…"
```

For sensitive material that must not leave the machine, a local model is the
right choice - and the one provider that never sends data off-host:

```bash
rook --provider ollama --model llama-4 "…"
```

### Any other provider

Anything that speaks the OpenAI-compatible API works. Name a provider, give it a
base URL and a key:

```yaml
default_provider: mygateway
providers:
  mygateway:
    driver: custom
    base_url: https://gateway.internal.example.com/v1
    api_key: '$GATEWAY_KEY'
```

A key can be written literally or as a `$VAR` reference so no secret is on disk.

## Configuration

Configuration is layered: **built-in defaults < config file < `ROOK_*` env vars
< CLI flags**. The config file is optional - env vars alone are enough.

```bash
rook config        # opens the config in $EDITOR, creating it from a template
rook config path   # print the config file location
```

The file lives at `~/.config/rook/config.yaml` (override with `$ROOK_CONFIG` or
`--config`). Every scalar has a matching `ROOK_*` env var (`agent.model` →
`ROOK_AGENT_MODEL`, `default_provider` → `ROOK_DEFAULT_PROVIDER`). A provider's key
comes from its provider's conventional variable or `api_key` in the file, which
may be a literal or a `$VAR` reference. A developer build also reads a `.env`
from the working directory - a released one does not (see
[Development](#development)). See
[configs/rook.example.yaml](configs/rook.example.yaml).

Under `agent:` the run's engine knobs mirror zot's - `max_settles`, `max_calls`,
`max_time`, `max_tokens`, `max_tool_output`, the recovery and cycle caps,
`limit_checkpoints`, `context_strategy` and the `compact_*` trio - each optional,
zero using the engine default. A model entry additionally takes `context` (a
smaller real context window than the model's card) and `vision` (this model can
be shown images). See [configs/rook.example.yaml](configs/rook.example.yaml).

Rook strips the resolved provider credential from the environment before the
agent runs, so the commands it executes against a target cannot read it. A
built-in provider's conventional key is withheld once you set a custom
`base_url`, so a key scoped to one host is never forwarded to another.

## Files & directories

Rook uses four distinct locations - it helps to keep them straight:

| Location          | What it holds                                                                                                                                               | Default path                                                                                                     |
| ----------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| **Workspace**     | The directory Rook works _in_ - what it reads, edits, and runs commands against. Any file the agent writes (a report you asked for, a PoC) lands here.      | the current working directory (the desktop image opens in `/workspace`)                                          |
| **Dossier**       | The project's objectives and run records: `.rook/objectives/<slug>.yaml` (the mission files) and `.rook/records/<slug>/<run>.yaml` (the ledger receipts).    | `./.rook/` (override with `--objectives-dir` / `--records-dir`)                                                  |
| **Run artifacts** | Rook's own record of _each run_: `status.json` (live state) and `events.jsonl` (append-only log). Telemetry, not work product - the status widget reads it. | `~/.local/state/rook/runs/<runid>/` (`$XDG_STATE_HOME`; override with `--run-dir` / `run_dir` / `$ROOK_RUN_DIR`) |
| **Config**        | Your settings and provider keys.                                                                                                                            | `~/.config/rook/config.yaml` (`$ROOK_CONFIG` / `--config`)                                                       |

The **dossier** is the contract: objective files are what a run is dispatched
from, and the ledger records what has been done. The **run artifacts** are
Rook's log of _what it did_; the **workspace** is _where it did it_. They never
mix: run artifacts are telemetry under your state directory, while the agent's
file writes stay in the workspace.

Rook does not create files in the workspace on its own - the findings **report**
is delivered as the agent's response. If you want it saved, ask for it in the
objective and the agent writes it into the workspace.

Each run gets its own `runs/<runid>/` directory (`<timestamp>-<pid>`), so
concurrent runs never overwrite each other; the desktop widget shows the most
recent active run.

## Usage

```bash
export ZAI_API_KEY="sk-..."       # or --provider openai with OPENAI_API_KEY, etc.

# Write an objective, then run it
rook new "Gain access to the target network and map paths to domain admin"
# edit .rook/objectives/gain-access-to-the-target-network-and-map-pa.yaml
# to set the success criteria, then:
rook

# Run a single objective with reasoning streamed to the terminal
rook -v .rook/objectives/firmware-recon.yaml

# Version
rook version
```

A developer build loads a `.env` from the working directory; a released binary
does not (see [Development](#development)).

### Flags

| Flag               | Default                      | Description                                        |
| ------------------ | ---------------------------- | -------------------------------------------------- |
| `--provider`        | `zai`                        | Model provider to run against, or one named in config |
| `--config`         | `~/.config/rook/config.yaml` | Path to the config file (or `$ROOK_CONFIG`)        |
| `--model`          | `glm-5.2`                    | Model the agent reasons with (overrides config)    |
| `--max-iterations` | `10000`                      | Maximum agent iterations before a forced stop      |
| `--objectives-dir` | `./.rook/objectives`         | Where this project's objectives live, run by a bare `rook` |
| `--records-dir`    | `./.rook/records`            | Where run records (the ledger) are written         |
| `--watch`          | `false`                      | Stay up and run objectives as they arrive          |
| `--rerun`           | `false`                      | Run objectives even when the ledger says done      |
| `-v`, `--verbose`  | `false`                      | Stream the agent's reasoning tokens to stdout      |
| `-V`, `--version`  | -                            | Print version and exit                             |

Flags override `ROOK_*` environment variables, which override the config file,
which overrides the built-in defaults.

### Objective files

An objective is a small YAML file with four fields:

```yaml
# .rook/objectives/firmware-recon.yaml
# rook objective - what to do and what "done" means.

title: Firmware reverse engineering

# The durable goal of the engagement. The objective will not run until this is filled in.
objective: |-
  Reverse engineer the firmware image: recover embedded credentials, map the
  services it exposes, and identify remotely reachable memory-corruption bugs

# The objective is not met until every one of these holds.
success:
  - every credential is extracted and documented with its location and purpose
  - every remote service is enumerated with its protocol and entry point
  - every exploitable bug has a working PoC and impact assessment
  - the engagement report is delivered as the run's outcome

# Rules that hold for the whole run - non-negotiable constraints on how the
# objective may be pursued.
rules_of_engagement:
  - no network access; work entirely offline against the image
  - do not modify the original firmware image
```

The agent's findings stream to **stderr**; with `--verbose`, reasoning tokens
stream to **stdout**. The final report is delivered as the agent's response -
Rook does not write files on its own. If you want the report (or any other
artifact) saved to disk, ask for it in the objective and the agent will use its
`write` tool.

## Skills

A skill is a `SKILL.md` playbook - methodology, or a vulnerability-class hunting
guide - that the agent reads when it decides the skill is relevant. Rook does
**not** compile a skill library into the binary. The only embedded skill is a
**catalog**: an index that tells the agent where external skill collections live
and how to install them. Everything else is fetched on demand.

**Where skills live.** Rook loads skills from `~/.config/rook/skills`
(`$XDG_CONFIG_HOME/rook/skills`), each as `<skill-name>/SKILL.md`. It rescans
that directory every turn, so a skill added while a run is going - including one
the agent clones itself - is available on the next step, no restart. A skill on
disk overrides an embedded one of the same name.

**How the agent gets them.** The catalog names a public collection and the
agent, using its `shell` tool, clones it into the skills directory - for
example [Claude-BugHunter](https://github.com/elementalsouls/Claude-BugHunter),
a broad library of bug-bounty methodology and per-vulnerability-class playbooks
(SQLi, XSS, SSRF, IDOR, OAuth, SAML, GraphQL, cloud, business logic, and more).
Nothing is baked into the binary, so the collection can grow and stay current
without a rook release, and Rook carries no third-party skill content or license
of its own.

**Adding your own skill.** Drop `<name>/SKILL.md` into `~/.config/rook/skills`:

```markdown
---
name: My Skill
description: One sentence the model uses to decide when to apply this skill.
---

# My Skill

Step-by-step guidance...
```

No rebuild - it is picked up on the next turn. A private methodology skill, or
an override of a collection's shipped one, is just a file in that directory.

## How it works

```
cmd/rook          CLI: flags, .env, signal handling, version
internal/config   Central config: default model, max iterations, system prompt
internal/agent    Loads the catalog + on-disk skills, registers tools, drives the loop
internal/version  Build-time version + GitHub release update check
embed.go          //go:embed skills  →  the embedded catalog
skills/           the skill-catalog bootstrap (no skill library)
```

The default model and the agent's system prompt (backstory) live in one place -
[`internal/config/config.go`](internal/config/config.go) - so they can be tuned
without touching the CLI or the agent loop.

At startup Rook loads the embedded catalog with `agent.LoadSkillsFromFS` and
layers the on-disk skills directory over it with `agent.NewSkillLoader`, which
rescans that directory each turn. Skills are read with the `read` tool - an
embedded skill through an `embedded-skill://` URL, an on-disk one through its
file path. Rook registers `agent.DefaultTools()`, builds a security-focused
backstory, and runs `agent.ExecuteWithTools` until the agent records an outcome
by calling `_success` or `_failure`.

## Development

The engine is the published [zot](https://github.com/openzot/openzot) module,
pinned in `go.mod`, so the repository builds from a clean clone with no extra
steps:

```bash
git clone https://github.com/pdparchitect/rook
cd rook
make            # lists the targets
make build      # build ./rook
```

`make` on its own prints the targets rather than assuming one, because Rook has
two build variants:

```bash
make build    # release binary
make dev       # developer binary - reads a .env from the working directory
make test      # run tests
make race      # tests under the race detector
make vet       # go vet over both build variants
make dist      # cross-platform release archives under dist/
```

**Release vs developer builds.** A released binary does **not** read a `.env`
from its working directory; a developer build does. Rook runs shell commands
against targets with a provider key in the process, so a released binary must
not take credentials from whatever directory it was pointed at - a stray
committed `.env` in the code under review would otherwise reach the process
about to run commands against it. The switch is a build tag (`-tags dev`) that
defaults to off; `rook --version` prints which kind you have. See
[RELEASES.md](RELEASES.md) for the release flow.

## Skill collections

Rook bundles no third-party skill content. The catalog points at external
collections you fetch on demand; the default one is **claude-bughunter** by
**[Sachin Sharma](https://www.linkedin.com/in/sachinsharma8080/)**:

> https://github.com/elementalsouls/Claude-BugHunter

That collection is MIT-licensed and keeps its own license when you clone it -
Rook neither redistributes nor relicenses it. Our thanks to the author and the
bug-bounty community whose disclosed reports informed it.

## License

Rook is MIT licensed - see [LICENSE](LICENSE). It bundles no third-party
content: skill collections are fetched at runtime and keep their own licenses
(see [Skill collections](#skill-collections)).
