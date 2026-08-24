<h1 align="center">Rook</h1>

<p align="center">
  <strong>Point it at a target. Walk away. Come back to findings.</strong>
</p>

<p align="center">
  <img width="1672" height="941" alt="rook running an objective" src="https://github.com/user-attachments/assets/0900d959-a741-4212-a8b8-bedd5eca3203" />
</p>

Rook is an AI bug-hunting harness for vulnerability research, bug hunting and
source-code auditing, in a single Go binary. It takes an objective - the goal,
the success criteria that define done, the rules of engagement - and works the
problem the way a researcher would: enumerating, reading code, probing, and
chaining what it finds, then handing back concrete findings with the evidence to
back them. The autonomous engine ([zot](https://github.com/openzot/openzot)) runs
inside the binary and talks straight to any OpenAI-compatible provider with your
own key. No hosted service, no telemetry.

> ⚠️ **Authorized use only.** Rook is an offensive-security tool. Only run it
> against systems, code and services you own or are explicitly authorized to
> test. Read [safety](docs/safety.md) first.

## Install

```bash
curl -fsSL https://github.com/pdparchitect/rook/releases/latest/download/install.sh | bash
```

That fetches the latest release for your platform (Linux or macOS, amd64 or
arm64), verifies its checksum, and puts `rook` in `~/.local/bin`. The same
command upgrades in place. Pin a version with `ROOK_VERSION=vX.Y.Z`, or change
the directory with `ROOK_INSTALL_DIR`.

Prefer to do it by hand? Grab a tarball from the
[releases page](https://github.com/pdparchitect/rook/releases), or
[build from source](docs/development.md).

## Use

Rook defaults to the `zai` provider running `glm-5.2`. Export a key, write an
objective, hand it over:

```bash
export ZAI_API_KEY="sk-..."
rook new "Audit the HTTP handlers in ./server for injection and auth-bypass bugs"
rook
```

`rook new` writes a small YAML objective under `.rook/objectives/`; edit its
success criteria, then a bare `rook` runs everything outstanding, skipping what
the ledger already records as done. `rook --watch` turns the folder into a drop
box. Any OpenAI-compatible provider works - `--provider anthropic`, a local
`--provider ollama`, a gateway, a custom endpoint - see
[providers](docs/providers.md).

The agent's findings are its response; it does not write files unless the
objective asks. Every run records a resumable session (`rook --resume last`) and
closes with a digest carrying the session id.

## Why Rook

- **Objectives, not prompts.** Mission files that queue, batch and stream, with a ledger of what's done. [→ objectives](docs/objectives.md)
- **One portable binary.** Static, cross-platform, nothing to install; carry it onto a target box and remove it cleanly. [→ how it works](docs/how-it-works.md)
- **Skills on demand.** The binary carries a catalog, not a library; the agent fetches the playbooks a hunt needs. [→ skills](docs/skills.md)
- **Built to run unattended.** In-process engine, compaction, loop detection, resumable logs - your key, your provider, your machine. [→ how it works](docs/how-it-works.md)

## ⚠️ Safety

Rook has real file-write and shell access and runs commands against whatever you
point it at; `--dir` is **not** a sandbox. Point it at a disposable checkout or
an isolated host, and bound the run with the objective's rules of engagement.
Read [safety](docs/safety.md) first.

## Documentation

- [docs/objectives.md](docs/objectives.md) - objective files, the ledger, watch mode, flags, directories
- [docs/providers.md](docs/providers.md) - providers, credentials, gateways, custom endpoints
- [docs/configuration.md](docs/configuration.md) - config file, env vars, engine tuning, credential hygiene
- [docs/skills.md](docs/skills.md) - the skill catalog, where skills live, adding your own
- [docs/how-it-works.md](docs/how-it-works.md) - the harness, the engine under it, sessions
- [docs/safety.md](docs/safety.md) - what Rook can touch and how to bound it
- [docs/development.md](docs/development.md) - building from source, release vs developer builds
- [CHANGELOG.md](CHANGELOG.md) · [RELEASES.md](RELEASES.md)

## Ecosystem

| Project                                          | Role                                                                             |
| ------------------------------------------------ | -------------------------------------------------------------------------------- |
| [Zot](https://github.com/openzot/openzot)        | The autonomous engine Rook runs on - an automated software factory in one binary |
| [Pion](https://github.com/pdparchitect/pion)     | A defensive AI security harness for automatic monitoring and incident prevention |
| [Pantalk](https://github.com/pantalk/pantalk)    | Connect coding agents to the chat platforms people already use                   |

## License

Rook is MIT licensed - see [LICENSE](LICENSE). It bundles no third-party content:
skill collections are fetched at runtime and keep their own licenses (see
[docs/skills.md](docs/skills.md#skill-collections)).

## Status

Rook is **0.x** and in active use. Flags, config and behavior may change before
1.0 - pin a version and skim the [changelog](CHANGELOG.md) before upgrading.
Small, focused pull requests are welcome; anything large is worth an issue first.
