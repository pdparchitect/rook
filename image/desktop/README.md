# Rook desktop image

A [Launcher](https://github.com/pdparchitect/launcher) product image: Rook, the
autonomous security agent, packaged as an isolated browser desktop you can
install, run on real work, and remove cleanly.

Rook is not an interactive agent — each run takes a task and an authorization
scope — so the desktop opens a **persistent terminal in `/workspace`**, not an
agent process. You drive `rook` from there.

## Layout

```text
image/desktop/
  Dockerfile        multi-stage: compile rook from this repo, then FROM the
                    published Launcher desktop base
  overlay/          files copied over the base defaults (root filesystem layout)
    etc/profile.d/rook-env.sh          exports the saved API key into shells
    etc/xdg/kitty/theme.conf           the terminal palette
    opt/browser/index.html             the browser landing page
    usr/local/bin/desktop-welcome      the terminal opened on start
    usr/local/bin/desktop-harness      Ctrl-Shift-G opens another terminal
    usr/local/bin/desktop-panel-status the panel's API-key indicator
    usr/local/bin/desktop-selftest     live assertions for the smoke test
    usr/local/bin/rook-greeting        the banner printed in the terminal
    usr/local/bin/rook-setup           saves the ChatBotKit API secret
    usr/share/backgrounds/…            the wallpaper
  theme/apply-theme.sh                 recolours the inherited chrome to Rook's
                                       obsidian + crimson palette (build time)
  launcher/         the Launcher application bundle (application.json, artwork)
  tests/test-image.sh                  static checks
  Makefile                             local Docker workflow (build/run/smoke)
```

## Build and run

A `Makefile` here gives the same local Docker workflow as the sibling image
projects (buzzbox, buzznode). Run it from this directory:

```sh
cd image/desktop
make build     # build pdparchitect/rook-desktop:local (compiles rook from source)
make run       # start the container, publish the desktop, print the URLs
make smoke     # wait for readiness and run the product selftest
make stop      # remove the container
make help      # all targets and overrides
make test      # check + build + run + smoke
```

`make run` publishes the desktop on <http://localhost:6901> and the bridge on
<http://localhost:6902> (`/healthz`, `/preview.jpg`), mounts named volumes for
`/workspace` and `/home/agent/.config/rook`, and passes any `RELAY_API_KEY` /
`CBK_API_SECRET` / `CHATBOTKIT_API_SECRET` set in your shell straight through, so
a run can reach a backend without `rook-setup` first. Common overrides:

```sh
make run PORT=7000 PREVIEW_PORT=7001         # different host ports
make build PLATFORM=linux/arm64              # cross-build
make build DESKTOP_IMAGE=ghcr.io/pdparchitect/launcher-image-base-desktop:0.1.9
```

The build context is the **repository root** (the compile stage needs the Go
module and embedded skills); the Makefile handles that. The equivalent raw
command, if you prefer not to use `make`:

```sh
docker build -f image/desktop/Dockerfile -t rook-desktop .   # run from the repo root
docker run --rm -it --shm-size=1g -p 6901:6901 -p 6902:6902 rook-desktop
```

`--shm-size=1g` matters: Chromium crashes on Docker's 64MB default. The desktop
base is pulled via the `DESKTOP_IMAGE` build arg (default
`ghcr.io/pdparchitect/launcher-image-base-desktop:latest`); pin it to a released
substrate version for reproducible builds.

## The API key

Rook defaults to the CBK Relay backend, whose credential is `RELAY_API_KEY`
(your OpenAI/OpenRouter key). Launcher bakes an application's environment into the
container and does not let a user set a per-agent variable, and Rook has no
interactive sign-in — it reads the key from the environment. So it's entered
inside the session, once: run `rook-setup`, which saves it to the persistent
`~/.config/rook` volume; `/etc/profile.d/rook-env.sh` then exports it into every
later terminal. The panel shows `ROOK · RUN rook-setup` until a key is present.

To use the ChatBotKit backends instead, run `rook-setup CBK_API_SECRET` (or
`CHATBOTKIT_API_SECRET`) and `rook --backend cbk` (or `chatbotkit`).

## A future benchmark variant

`image/desktop/` is one variant. A headless image for running Rook in
benchmarks would sit beside it as `image/bench/` with its own Dockerfile — most
likely `FROM` a minimal base rather than the desktop substrate, sharing this
repo's build stage. Nothing here assumes `desktop` is the only variant.

## Publishing to Launcher

To appear in Launcher's catalogue this image must publish an OCI application
artifact (the `launcher/` bundle) alongside the multi-arch image, and be listed
in a publisher feed Launcher resolves. See the Launcher repo's
`docs/application-registry.md`. A real `screenshot.jpg` is required first — see
[launcher/SCREENSHOT.md](launcher/SCREENSHOT.md).
