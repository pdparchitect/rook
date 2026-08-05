# Changelog

All notable changes to Rook, following [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and [Semantic Versioning](https://semver.org/).

## [0.4.0] - 2026-08-05

### Features

- Run artifacts: every run now writes its own `status.json` (live state: model, scope, iteration, current tool, timestamps, exit) and `events.jsonl` (an append-only event log) into a per-run directory `<run_dir>/<runid>/`. This is telemetry, separate from the workspace the agent works in, and is what the desktop status widget reads. It is always on, multi-instance safe (`<runid>` is `<timestamp>-<pid>`), and the base directory is `$XDG_STATE_HOME/rook/runs` by default (override with `--run-dir`, `run_dir` in config, or `$ROOK_RUN_DIR`).

## [0.3.0] - 2026-08-05

### Features

- `rook config` opens the config file in your `$EDITOR`, creating it from a commented template (embedded in the binary) on first run. This is the setup path - choose a backend and model and set your provider key by editing the file. `rook config path` prints the file location.

### Changed

- **Breaking (auth):** the `relay` backend no longer reads a `RELAY_API_KEY` environment variable. Its credential is your own provider key, set per model (`backends.relay.models.<model>.authorization`) or as a backend-level default (`backends.relay.authorization`) in the config file, or inlined into `--model` as `<model>/authorization=<key>`. A relay run with no such key fails with an actionable error rather than falling back to an env var.
- The default model is now `glm-5.2` (was `qwen-3.6-plus`) - a strong open model suited to autonomous security work.
- The example config showcases open security models (`glm-5.2`, `kimi-k3`, `deepseek-v4-flash`).

## [0.2.0] - 2026-08-05

### Features

- Configuration file: Rook now reads a layered configuration - built-in defaults < `~/.config/rook/config.yaml` < `ROOK_*` environment variables < CLI flags. The file path is overridable with `$ROOK_CONFIG` or `--config`, and values may reference secrets with `$VAR`. The file is optional; env vars alone still configure a run. See [configs/rook.example.yaml](configs/rook.example.yaml).
- Backends: a run targets a named **backend** via `--backend` (or `default_backend` in config). Three ship built in - `relay` (CBK Relay, the default), `cbk` (ChatBotKit at `api.cbk.ai`) and `chatbotkit` (ChatBotKit at `api.chatbotkit.com`). Under `backends.<name>.models`, a custom entry can alias a real model id and override `max_iterations`.
- Relay per-model authorization: the CBK Relay authenticates each model with its own provider key, carried inside the model string as `<model>/authorization=<key>` (each model may be a different provider). Rook composes it from a model's `authorization`, a backend-level default, or `$RELAY_API_KEY`; a key already inlined into `--model` is left untouched.
- Secret hygiene: resolved backend credentials - Bearer secrets and provider authorizations, backend-level and per-model - are stripped from the environment before the agent runs, so the commands it executes against a target cannot read them.

### Changed

- **Breaking (auth):** the default backend is now `relay`, whose credential is `RELAY_API_KEY` (your OpenAI/OpenRouter key). To reach ChatBotKit as before, use `--backend cbk` with `CBK_API_SECRET`, or `--backend chatbotkit` with `CHATBOTKIT_API_SECRET`.
- The Go SDK is bumped to `github.com/chatbotkit/go-sdk v0.4.0`.
- `.env` is now loaded only as a convenience for populating the environment; the config file is the primary configuration surface.

## [0.1.2]

- Initial public releases: single self-contained Go binary, embedded security skill library, autonomous agent loop over the ChatBotKit API, and cross-platform release archives.
