# Rook

<img width="1672" height="941" alt="image" src="https://github.com/user-attachments/assets/0900d959-a741-4212-a8b8-bedd5eca3203" />

**Rook** is an AI bug-hunting harness for vulnerability research, bug hunting and
source-code auditing. It is a single Go executable that drives a model through
the whole hunt: the autonomous engine ([zot](https://github.com/openzot/openzot))
runs inside the binary and talks straight to a model provider, with a library of
security skills embedded directly into the binary - no external files, no hosted
service, no setup beyond a provider key.

Give Rook a target and a scope, and it works through the problem the way a
researcher would.

> ⚠️ **Authorized use only.** Rook is an offensive-security tool. Only run it
> against systems, code and services you own or are explicitly authorized to
> test. Always pass an explicit `--scope`.

## What can it do?

A single binary, a plain-English task, and an explicit scope. Each example
below is backed by Rook's built-in [skills](#embedded-skills):

```bash
# Source-code audit - injection, IDOR and broken access control
rook --scope "repo: ./api, read-only, no network" \
     "Audit ./api for SQL injection, IDOR and auth bypass"

# Web app / API - SSRF in a URL-fetching feature (authorized target)
rook --scope-file scope.txt \
     "Test the link-preview endpoint on staging.example.com for SSRF to cloud metadata"

# External recon & OSINT - map an organisation's attack surface
rook --scope "domain: example.com + subdomains, passive recon only" \
     "Map example.com's external surface: subdomains, exposed services and leaked secrets"

# Cloud misconfiguration - read-only review
rook --scope "AWS, describe/list only, no mutations" \
     "Check for public S3 buckets, over-permissive IAM roles and IMDS exposure"

# Smart-contract audit
rook --scope "repo: ./contracts" \
     "Audit the Solidity contracts for reentrancy, access-control and oracle bugs"

# Supply chain - dependencies and CI exposure
rook --scope "repo: ., read-only" \
     "Review dependencies for known CVEs and flag supply-chain risks"
```

Rook also covers OAuth/SAML/JWT flaws, file-upload and SSTI/RCE chains,
business-logic and race conditions, HTTP request smuggling, and enterprise
identity/infrastructure attack surfaces (M365/Entra, Okta, VPN appliances,
vCenter, SharePoint) - see the full [skill library](#embedded-skills).

## Why Rook?

Security work happens in awkward places - a hardened bastion, an air-gapped
network, a throwaway cloud VM, a CI runner, someone else's laptop during an
engagement. Rook is built for exactly those:

- **One single executable.** Everything - the agent loop, the tools, and the
  entire skill library - is compiled into one binary via Go's `embed`. There is
  no runtime to install, no interpreter, no `node_modules`, no virtualenv, no
  config files to ship alongside it. Download one file, `chmod +x`, run.
- **Portable everywhere.** Statically linked (`CGO_ENABLED=0`) and
  cross-compiled for Linux, macOS and Windows on both amd64 and arm64. The same
  tool drops onto an Apple-silicon laptop, an x86 server, or an ARM box with no
  changes. Nothing to match against the host's libraries or OS version.
- **Nothing to fetch at runtime.** Because the skills are baked in, Rook works
  in locked-down or offline environments where you can't `pip install` or pull
  containers. Its only external dependency is the model provider you point it at
  (and your key) - or nothing at all off the machine, if you run a local model.
- **The engine runs in the binary.** The reasoning and tool-execution loop,
  thread management, compaction and loop detection all run in-process, in the
  same statically-linked file as the skills. Nothing about a run depends on a
  service staying up, and a run is reproducible offline. You point Rook at
  whichever provider you already pay for.
- **Trivial to distribute and audit.** A single artifact with a published
  checksum is easy to vet, copy onto a target box, version-pin, and remove
  cleanly afterwards - important when you're operating inside someone else's
  scope.
- **Purpose-built, not a general chatbot.** Rook is a focused bug-hunting
  harness: it knows the methodology, the bug classes, and the reporting
  discipline out of the box, and stays within the authorization boundary you
  give it.

In short: the value isn't just "an AI security tool" - it's an AI bug-hunting
harness you can carry anywhere as **one file** and run with **zero setup**.

## Features

- **Single self-contained binary.** The skill library is compiled into the
  executable via Go's `embed`, so it ships and runs as one file.
- **Autonomous agent loop.** Built on the Go SDK's `agent.ExecuteWithTools` -
  the agent plans, acts, tracks progress and exits on its own, bounded by
  `--max-iterations`.
- **Built-in tools.** File read/write/edit and sandboxed shell execution via
  the SDK's `DefaultTools`.
- **Embedded skill library.** Phase-by-phase security playbooks (see below)
  surfaced to the model through the SDK skills feature.
- **Cross-platform releases.** GitHub Actions builds binaries for Linux, macOS
  and Windows (amd64/arm64) on every tag.

## Install

### From a release (recommended)

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

## Backends

A run targets a **backend** - the provider Rook talks to. Rook speaks to each one
directly over the OpenAI-compatible API; there is no gateway and no account in
between, so all you need is a provider key. Pick a backend with `--backend`, or
set `default_backend` in config.

| Backend      | Endpoint                         | Credential from      |
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
offensive tasks. The model must be one the chosen backend serves.

The common case is one exported variable and nothing else:

```bash
export ZAI_API_KEY="sk-..."
rook --scope "repo: ./server" "Audit the HTTP handlers for injection bugs"
```

Switch provider with a flag:

```bash
export OPENAI_API_KEY="sk-..."
rook --backend openai --model gpt-5 "…"
```

For sensitive material that must not leave the machine, a local model is the
right choice - and the one backend that never sends data off-host:

```bash
rook --backend ollama --model llama-4 "…"
```

### Any other provider

Anything that speaks the OpenAI-compatible API works. Name a backend, give it a
base URL and a key:

```yaml
default_backend: mygateway
backends:
  mygateway:
    provider: custom
    base_url: https://gateway.internal.example.com/v1
    api_key: '$GATEWAY_KEY'
```

A key can be written literally or as a `$VAR` reference so no secret is on disk.

## Configuration## Configuration

Configuration is layered: **built-in defaults < config file < `ROOK_*` env vars
< CLI flags**. The config file is optional - env vars alone are enough.

```bash
rook config        # opens the config in $EDITOR, creating it from a template
rook config path   # print the config file location
```

The file lives at `~/.config/rook/config.yaml` (override with `$ROOK_CONFIG` or
`--config`). Every scalar has a matching `ROOK_*` env var (`agent.model` →
`ROOK_AGENT_MODEL`, `default_backend` → `ROOK_DEFAULT_BACKEND`). A backend's key
comes from its provider's conventional variable or `api_key` in the file, which
may be a literal or a `$VAR` reference. A developer build also reads a `.env`
from the working directory - a released one does not (see
[Development](#development)). See
[configs/rook.example.yaml](configs/rook.example.yaml).

Rook strips the resolved backend credential from the environment before the
agent runs, so the commands it executes against a target cannot read it.

## Files & directories

Rook uses three distinct locations - it helps to keep them straight:

| Location          | What it holds                                                                                                                                               | Default path                                                                                                     |
| ----------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| **Workspace**     | The directory Rook works _in_ - what it reads, edits, and runs commands against. Any file the agent writes (a report you asked for, a PoC) lands here.      | the current working directory (the desktop image opens in `/workspace`)                                          |
| **Run artifacts** | Rook's own record of _each run_: `status.json` (live state) and `events.jsonl` (append-only log). Telemetry, not work product - the status widget reads it. | `~/.local/state/rook/runs/<runid>/` (`$XDG_STATE_HOME`; override with `--run-dir` / `run_dir` / `$ROOK_RUN_DIR`) |
| **Config**        | Your settings and provider keys.                                                                                                                            | `~/.config/rook/config.yaml` (`$ROOK_CONFIG` / `--config`)                                                       |

The **run** is Rook's log of _what it did_; the **workspace** is _where it did
it_. They never mix: run artifacts are telemetry under your state directory,
while the agent's file writes stay in the workspace.

Rook does not create files in the workspace on its own - the findings **report**
is delivered as the agent's response. If you want it saved, ask for it in the
task and the agent writes it into the workspace, e.g.
`rook --scope "repo: ., read-only" "audit this repo and write the report to report.md"`.

Each run gets its own `runs/<runid>/` directory (`<timestamp>-<pid>`), so
concurrent runs never overwrite each other; the desktop widget shows the most
recent active run.

## Usage

```bash
export ZAI_API_KEY="sk-..."       # or --backend openai with OPENAI_API_KEY, etc.

# Audit a local codebase
rook --scope "repo: ./server, no network access" \
     "Audit the HTTP handlers in ./server for injection and auth bypass bugs"

# Hunt with reasoning streamed to the terminal
rook -v --scope-file scope.txt "Find SSRF in the URL-fetching service"

# Version
rook version
```

A developer build loads a `.env` from the working directory; a released binary
does not (see [Development](#development)).

### Flags

| Flag               | Default                      | Description                                        |
| ------------------ | ---------------------------- | -------------------------------------------------- |
| `--backend`        | `zai`                        | Backend to target: any provider, or one named in config |
| `--config`         | `~/.config/rook/config.yaml` | Path to the config file (or `$ROOK_CONFIG`)        |
| `--model`          | `glm-5.2`                    | Model the agent reasons with (overrides config)    |
| `--max-iterations` | `10000`                      | Maximum agent iterations before a forced stop      |
| `--scope`          | -                            | Authorization boundary (hosts, repos, paths)       |
| `--scope-file`     | -                            | Read the authorization scope from a file           |
| `-v`, `--verbose`  | `false`                      | Stream the agent's reasoning tokens to stdout      |
| `-V`, `--version`  | -                            | Print version and exit                             |

Flags override `ROOK_*` environment variables, which override the config file,
which overrides the built-in defaults.

The agent's findings stream to **stderr**; with `--verbose`, reasoning tokens
stream to **stdout**. The final report is delivered as the agent's response -
Rook does not write files on its own. If you want the report (or any other
artifact) saved to disk, ask for it in the task and the agent will use its
`write` tool.

## Embedded Skills

Rook ships with **51 security skills** - each a `SKILL.md` playbook under
[`skills/`](skills/), embedded into the binary at build time and offered to the
agent as it works. They cover, roughly:

- **Methodology & mindset** - `bug-bounty`, `bb-methodology`, `redteam-mindset`,
  `bb-local-toolkit`, `hunt-dispatch`.
- **Web/API vulnerability hunting** (24 `hunt-*` classes + `security-arsenal`) -
  IDOR, SQLi, XSS, SSRF, RCE, SSTI, XXE, CSRF, OAuth, SAML, GraphQL, auth/MFA
  bypass, ATO, business logic, cache poisoning, HTTP smuggling, file upload,
  API misconfig, race conditions, and more.
- **Enterprise & infrastructure attack chains** - `m365-entra-attack`,
  `okta-attack`, `cloud-iam-deep`, `vmware-vcenter-attack`,
  `enterprise-vpn-attack`, `hunt-sharepoint`, `hunt-aspnet`, `hunt-ntlm-info`,
  `apk-redteam-pipeline`, `supply-chain-attack-recon`.
- **Recon & OSINT** - `web2-recon`, `offensive-osint`, `osint-methodology`,
  `hunt-subdomain`.
- **Web3** - `web3-audit`, `meme-coin-audit`.
- **Triage, reporting & hygiene** - `triage-validation`, `bugcrowd-reporting`,
  `report-writing`, `redteam-report-template`, `evidence-hygiene`,
  `mid-engagement-ir-detection`.

These skills are sourced from the **claude-bughunter** project - see
[Credits](#credits).

### Adding a skill

Create `skills/<name>/SKILL.md` with YAML front matter:

```markdown
---
name: My Skill
description: One sentence the model uses to decide when to apply this skill.
---

# My Skill

Step-by-step guidance...
```

Rebuild the binary - the new skill is picked up automatically by the `embed`
directive. No registration code required.

## How it works

```
cmd/rook          CLI: flags, .env, signal handling, version
internal/config   Central config: default model, max iterations, system prompt
internal/agent    Loads embedded skills, registers tools, drives the agent loop
internal/version  Build-time version + GitHub release update check
embed.go          //go:embed skills  →  the embedded skill library
skills/           SKILL.md playbooks compiled into the binary
```

The default model and the agent's system prompt (backstory) live in one place -
[`internal/config/config.go`](internal/config/config.go) - so they can be tuned
without touching the CLI or the agent loop.

At startup Rook loads the embedded skills with `agent.LoadSkillsFromFS`,
registers `agent.DefaultTools()` plus a `skill` tool serving the embedded
library, builds a security-focused backstory that pins the agent to your
authorized scope, and runs `agent.ExecuteWithTools` until the agent records an
outcome by calling `_success` or `_failure`.

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

## Credits

Rook's embedded skill library is sourced from the **claude-bughunter** project
by **[Sachin Sharma](https://www.linkedin.com/in/sachinsharma8080/)**:

> https://github.com/elementalsouls/Claude-BugHunter

The skills are used under the MIT License (Copyright © 2026 Sachin Sharma). The
full upstream license is preserved in [NOTICE.md](NOTICE.md). Our thanks to the
author and the bug-bounty community whose disclosed reports informed them.

## License

Rook itself is MIT licensed - see [LICENSE](LICENSE). Bundled third-party
content retains its original license; see [NOTICE.md](NOTICE.md).
