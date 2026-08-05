# Rook

<img width="1672" height="941" alt="image" src="https://github.com/user-attachments/assets/0900d959-a741-4212-a8b8-bedd5eca3203" />

**Rook** is an AI bug-hunting harness for vulnerability research, bug hunting and
source-code auditing. It is a single Go executable that drives a model through
the whole hunt, built on the [ChatBotKit Go SDK](https://github.com/chatbotkit/go-sdk),
with a library of security skills embedded directly into the binary - no external
files, no setup beyond a provider key.

Give Rook a target and a scope, and it works through the problem the way a
researcher would: reconnaissance, analysis, hypothesis, verification, and a
written report.

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
  containers. Its only external dependency is the ChatBotKit API (and your key).
- **The hard parts run as a service.** This is the real reason Rook feels so
  light. Model orchestration, the reasoning and tool-execution loop, skill
  handling, scaling and reliability all run as a managed service on ChatBotKit,
  built and maintained by a dedicated team of engineers who do only this. The
  binary doesn't reimplement any of that complexity; it embeds the skills and
  streams the conversation. So the harness stays small and focused on the hunt,
  and you inherit backend improvements without shipping a new build.
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
[releases page](https://github.com/chatbotkit/rook/releases), for Linux, macOS
and Windows on both amd64 and arm64. Each archive contains a single `rook`
binary (plus README and LICENSE), and a `checksums.txt` is published alongside.

Pick the archive for your platform - e.g. `rook-v0.1.0-linux-amd64.tar.gz` - then
download, (optionally) verify, extract and put `rook` on your `PATH`:

```bash
VERSION=v0.1.0
OS=linux       # linux | darwin | windows
ARCH=amd64     # amd64 | arm64
BASE="https://github.com/chatbotkit/rook/releases/download/${VERSION}"

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
go install github.com/chatbotkit/rook/cmd/rook@latest
```

Or clone and build with the provided `Makefile`:

```bash
make build      # → ./rook
```

## Backends

A run targets a **backend** - the provider Rook talks to. Rook defaults to
**`relay`**; the ChatBotKit backends are an alternative for account holders. Pick
one with `--backend` or `default_backend` in config.

| Backend      | Endpoint                     | Auth style | Credential                   |
| ------------ | ---------------------------- | ---------- | ---------------------------- |
| `relay`      | `https://relay.cbk.ai`       | per-model  | your provider key, per model |
| `cbk`        | `https://api.cbk.ai`         | Bearer     | `CBK_API_SECRET`             |
| `chatbotkit` | `https://api.chatbotkit.com` | Bearer     | `CHATBOTKIT_API_SECRET`      |

Model names come from the ChatBotKit catalogue and are resolved against the
chosen backend; open models like `glm-5.2`, `kimi-k3` and `deepseek-v4-flash`
suit bug-hunting work.

### `relay` — the default (bring your own key)

The CBK Relay is a free proxy, and **there is no relay API key.** It
authenticates each model with _your own provider key_, carried inside the model
string as `<model>/authorization=<key>` - because on the relay each model is a
different provider (Z.AI, Moonshot, DeepSeek, …) with its own key. So you set the
key **per model** in config (paste it literally, or reference an env var):

```yaml
default_backend: relay
backends:
  relay:
    # authorization: $ZAI_API_KEY   # optional: one default key for all relay models
    models:
      glm-5.2:
        authorization: $ZAI_API_KEY      # or paste the key literally
      kimi-k3:
        authorization: $MOONSHOT_API_KEY
      deepseek-v4-flash:
        authorization: $DEEPSEEK_API_KEY
```

```bash
export ZAI_API_KEY="sk-..."
rook --model glm-5.2 "…"                          # sends glm-5.2/authorization=sk-... to the relay
rook --model 'glm-5.2/authorization=sk-...' "…"   # or inline it; Rook leaves it as-is
```

Rook composes the `authorization=` param onto the model automatically from the
per-model (or backend-level) config; a key you inline into `--model` is left as-is.

### `cbk` / `chatbotkit` — ChatBotKit account backends

If you have a ChatBotKit account, target it directly with a Bearer token instead
of bringing per-model keys. `cbk` and `chatbotkit` are the same platform on its
two hosts and take the same credential value under their own variable:

```bash
export CBK_API_SECRET="..."          # or CHATBOTKIT_API_SECRET for --backend chatbotkit
rook --backend cbk --model glm-5.2 "…"
```

Provide the token via that env var, or as `api_secret` under the backend in
config. Create one on the Tokens page
([chatbotkit.com/tokens](https://chatbotkit.com/tokens)); an account is at
[chatbotkit.com](https://chatbotkit.com) or [console.cbk.ai](https://console.cbk.ai).

## Configuration

Configuration is layered: **built-in defaults < config file < `ROOK_*` env vars
< CLI flags**. The config file is optional - env vars alone are enough.

```bash
rook config        # opens the config in $EDITOR, creating it from a template
rook config path   # print the config file location
```

The file lives at `~/.config/rook/config.yaml` (override with `$ROOK_CONFIG` or
`--config`). Every scalar has a matching `ROOK_*` env var (`agent.model` →
`ROOK_AGENT_MODEL`, `default_backend` → `ROOK_DEFAULT_BACKEND`). The Bearer
backends' credentials can come from their own env var or `api_secret` in the
file; the relay's per-model provider key is set in the file (or `$`-referenced
from the environment). A `.env` in the working directory is also loaded as a
convenience. See [configs/rook.example.yaml](configs/rook.example.yaml).

Rook strips the resolved backend credential from the environment before the
agent runs, so the commands it executes against a target cannot read it.

### Recommended: run under a sub-account

For better **isolation, cost control and observability**, we suggest running
Rook under a dedicated **sub-account** rather than your main account - each
engagement, tool or user then gets its own usage, billing and logs. For a
sub-account that is fully dedicated to Rook, a **standard token is enough**.

### Recommended: use a scoped token

We also recommend a **scoped token**, which limits the token to specific
ChatBotKit API routes (principle of least privilege), so a leaked key can't
touch the rest of your account. This matters less for a fully dedicated
sub-account, but it is good practice everywhere.

Rook runs **statelessly**, so it only needs the stateless completion route.
When creating the token, set its `allowedRoutes` to:

```yaml
allowedRoutes:
  - conversation/complete
```

Route patterns omit the `/v1/` prefix. See
[How to Create Scoped API Tokens](https://chatbotkit.com/tutorials/how-to-create-scoped-api-tokens-for-restricted-access)
for the full guide.

## Usage

```bash
# default "relay" backend: set your per-model provider key in the config file
# (see Backends above); or use a ChatBotKit backend: export CBK_API_SECRET=...

# Audit a local codebase
rook --scope "repo: ./server, no network access" \
     "Audit the HTTP handlers in ./server for injection and auth bypass bugs"

# Hunt with reasoning streamed to the terminal
rook -v --scope-file scope.txt "Find SSRF in the URL-fetching service"

# Version
rook version
```

Rook loads a `.env` file automatically if present (see `.env.example`).

### Flags

| Flag               | Default                      | Description                                        |
| ------------------ | ---------------------------- | -------------------------------------------------- |
| `--backend`        | `relay`                      | Backend to target: `relay`, `cbk`, or `chatbotkit` |
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
registers `agent.DefaultTools()`, builds a security-focused backstory that
pins the agent to your authorized scope, and runs `agent.ExecuteWithTools`
until the agent calls `exit`.

## Development

The committed `go.mod` pins a published version of the Go SDK, so the
standalone repository builds from a clean clone with no extra steps:

```bash
git clone https://github.com/chatbotkit/rook
cd rook
go build ./...        # or: make build
```

```bash
make build    # build ./rook
make test     # run tests
make vet      # go vet
make dist     # cross-platform release archives under dist/
```

### Developing against a local go-sdk

To build against a local checkout of the Go SDK instead of the published
module, place it at `../go-sdk` (or anywhere) and create a Go workspace:

```bash
make workspace        # writes a gitignored go.work
```

`go.work` is **gitignored**, so it only affects your local builds. See
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
