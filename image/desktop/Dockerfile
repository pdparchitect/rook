# syntax=docker/dockerfile:1.7
#
# Rook on the Launcher desktop base.
#
# Rook is a single self-contained Go binary: an autonomous security agent for
# vulnerability research, bug hunting and source-code auditing, built on the
# ChatBotKit API. It is not an interactive TUI - each run takes a task and an
# authorization scope - so the product surface is a persistent terminal in
# /workspace where the researcher drives `rook`, not an agent opened on start.
#
# Build from the repository root so the compile stage sees the Go module:
#
#     docker build -f image/desktop/Dockerfile -t rook-desktop .
#
# The binary is compiled from this repo rather than downloaded from a release,
# so the image can never drift from the source it ships beside.

# The Launcher desktop substrate, published by the Launcher repo. Declared here
# in the global scope (before the first FROM) so it can be used in the product
# stage's FROM below; an ARG consumed by a FROM must precede every FROM. Pin to
# a released substrate version for reproducible builds.
ARG DESKTOP_IMAGE=ghcr.io/pdparchitect/launcher-image-base-desktop:latest

# --- build stage: compile rook from source ---------------------------------
# Pinned to the build platform and cross-compiled with GOOS/GOARCH, so a
# multi-arch build never pays for QEMU on the Go compile.
FROM --platform=${BUILDPLATFORM} golang:1.23-bookworm AS build

WORKDIR /src

# Warm the module cache on the manifest alone, so source edits do not re-download
# dependencies. The committed go.mod pins a published SDK, so this needs no
# workspace or local checkout.
COPY go.mod go.sum ./
RUN go mod download

# embed.go bakes skills/ into the binary, so the whole module is the build input.
COPY . .

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" go build \
        -trimpath \
        -ldflags "-s -w -X github.com/chatbotkit/rook/internal/version.Version=$(cat VERSION)" \
        -o /out/rook ./cmd/rook

# --- product stage: rook on the desktop substrate --------------------------
# DESKTOP_IMAGE is declared in the global scope above so it resolves here.
FROM ${DESKTOP_IMAGE}

USER root

COPY --from=build /out/rook /usr/local/bin/rook
RUN chmod 0755 /usr/local/bin/rook && rook version

# Rook is stateless - it needs only the API and a key. What is worth persisting
# is its config: ~/.config/rook/config.yaml, where `rook config` stores the relay
# provider key (so a researcher does not re-enter it after every container
# replacement). rook reads it directly; it lives under the XDG config dir, kept
# out of the workspace files view.
RUN mkdir -p /home/agent/.config/rook && \
    chmod 0700 /home/agent/.config/rook && \
    chown -R agent:agent /home/agent

# Product entry points, terminal palette, wallpaper, browser page, key handling.
COPY image/desktop/overlay /
RUN chmod 0755 \
        /usr/local/bin/desktop-harness \
        /usr/local/bin/desktop-panel-status \
        /usr/local/bin/desktop-selftest \
        /usr/local/bin/desktop-welcome \
        /usr/local/bin/rook-greeting

# Recolour the inherited desktop chrome into Rook's palette: an obsidian canvas
# and chrome with a single crimson accent - the red-team signature - carrying
# the interactive bits. The script maps the base's colour vocabulary role by
# role rather than replacing themerc, tint2rc and gtk.css, so later substrate
# changes still reach this product. The focused window keeps the base's white
# outline untouched.
COPY image/desktop/theme/apply-theme.sh /tmp/apply-theme.sh
RUN bash /tmp/apply-theme.sh && rm /tmp/apply-theme.sh

RUN kasm-patch "Rook"

ENV DESKTOP_TITLE="Rook" \
    DESKTOP_PERSISTENT_PATHS="/home/agent/.config/rook"

LABEL org.opencontainers.image.title="Rook" \
    org.opencontainers.image.description="Rook autonomous security agent in a Launcher-managed browser desktop" \
    org.opencontainers.image.source="https://github.com/chatbotkit/rook"

VOLUME ["/workspace", "/home/agent/.config/rook"]

WORKDIR /workspace
