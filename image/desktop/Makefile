SHELL := /bin/bash
.DEFAULT_GOAL := help

# The Rook desktop image compiles rook from source, so the Docker build context
# is the repository root (two levels up), while this Makefile and the Dockerfile
# live here under image/desktop.
ROOT := $(abspath $(CURDIR)/../..)

DOCKER ?= docker
IMAGE ?= pdparchitect/rook-desktop:local
CONTAINER ?= rook-desktop
NATIVE_ARCH := $(shell uname -m | sed \
	-e 's/^x86_64$$/amd64/' \
	-e 's/^aarch64$$/arm64/')
PLATFORM ?= linux/$(NATIVE_ARCH)
TARGETARCH ?= $(word 2,$(subst /, ,$(PLATFORM)))
DESKTOP_IMAGE ?= ghcr.io/pdparchitect/launcher-image-base-desktop:latest

BIND_ADDRESS ?= 127.0.0.1
# PORT: the desktop (KasmVNC). PREVIEW_PORT: the desktop bridge, which serves
# /healthz, /preview.jpg and /notifications.
PORT ?= 6901
PREVIEW_PORT ?= 6902
VOLUME_PREFIX ?= rook-desktop

# Pass render nodes only. `--device=/dev/dri` would also hand over the DRM
# master node, which nothing in this container needs.
GPU_DEVICE := $(shell for node in /dev/dri/renderD*; do \
	[ -e "$$node" ] && echo "--device=$$node"; done)

# Rook reads its backend key from the environment; pass any that are set on the
# host straight through, so a run can reach the relay/cbk/chatbotkit backends
# without running rook-setup inside the session first.
KEY_ENV := $(foreach v,RELAY_API_KEY CBK_API_SECRET CHATBOTKIT_API_SECRET,\
	$(if $(strip $($(v))),--env "$(v)=$($(v))",))

.PHONY: help check build run recreate up test smoke stop logs vnc-log status url

help:
	@echo "Rook desktop local Docker workflow"
	@echo
	@echo "  make check      Validate overlay scripts and image metadata"
	@echo "  make build      Build $(IMAGE) (compiles rook from source)"
	@echo "  make run        Start or create the Rook desktop container"
	@echo "  make recreate   Recreate the container without rebuilding"
	@echo "  make up         Build and recreate the container"
	@echo "  make test       Check, build, run, and smoke-test the desktop"
	@echo "  make smoke      Test the running desktop and product selftest"
	@echo "  make logs       Follow container logs"
	@echo "  make vnc-log    Follow the KasmVNC session log"
	@echo "  make status     Show container and health status"
	@echo "  make stop       Stop and remove the container"
	@echo "  make url        Print the local desktop and bridge URLs"
	@echo
	@echo "Overrides: PORT=7000 PREVIEW_PORT=7001"
	@echo "           PLATFORM=linux/arm64 (default: $(PLATFORM))"
	@echo "           DESKTOP_IMAGE=$(DESKTOP_IMAGE)"
	@echo "           RELAY_API_KEY=sk-... (passed through to the container)"

check:
	bash tests/test-image.sh

build:
	$(DOCKER) build \
		--platform "$(PLATFORM)" \
		--build-arg "DESKTOP_IMAGE=$(DESKTOP_IMAGE)" \
		--file "$(CURDIR)/Dockerfile" \
		--tag "$(IMAGE)" \
		"$(ROOT)"

run:
	@if $(DOCKER) container inspect "$(CONTAINER)" >/dev/null 2>&1; then \
		if [ "$$($(DOCKER) container inspect --format '{{.State.Running}}' "$(CONTAINER)")" = "true" ]; then \
			echo "Container $(CONTAINER) is already running."; \
		else \
			$(DOCKER) start "$(CONTAINER)"; \
		fi; \
	else \
		$(DOCKER) run --detach \
			--name "$(CONTAINER)" \
			--platform "$(PLATFORM)" \
			--restart unless-stopped \
			--shm-size 1g \
			$(GPU_DEVICE) \
			--publish "$(BIND_ADDRESS):$(PORT):6901" \
			--publish "$(BIND_ADDRESS):$(PREVIEW_PORT):6902" \
			$(KEY_ENV) \
			--volume "$(VOLUME_PREFIX)-workspace:/workspace" \
			--volume "$(VOLUME_PREFIX)-config:/home/agent/.config/rook" \
			"$(IMAGE)"; \
	fi
	@$(MAKE) --no-print-directory url

recreate:
	@$(MAKE) --no-print-directory stop
	@$(MAKE) --no-print-directory run

up: build recreate

test: check up smoke

smoke:
	@echo "Waiting for the Rook desktop at http://$(BIND_ADDRESS):$(PORT) ..."
	@ready=false; \
	for attempt in $$(seq 1 60); do \
		if curl --fail --silent "http://$(BIND_ADDRESS):$(PORT)/index.html" >/dev/null && \
			curl --fail --silent "http://$(BIND_ADDRESS):$(PREVIEW_PORT)/healthz" >/dev/null; then \
			ready=true; \
			break; \
		fi; \
		sleep 2; \
	done; \
	if [ "$$ready" != "true" ]; then \
		echo "Rook desktop did not become ready within 120 seconds."; \
		$(DOCKER) logs --tail 150 "$(CONTAINER)" || true; \
		exit 1; \
	fi
	@$(DOCKER) exec "$(CONTAINER)" desktop-selftest
	@echo "Rook desktop is ready."

stop:
	@if $(DOCKER) container inspect "$(CONTAINER)" >/dev/null 2>&1; then \
		$(DOCKER) rm --force "$(CONTAINER)"; \
	else \
		echo "Container $(CONTAINER) does not exist."; \
	fi

logs:
	$(DOCKER) logs --follow "$(CONTAINER)"

vnc-log:
	$(DOCKER) exec "$(CONTAINER)" \
		bash -c 'tail --lines=200 --follow /home/agent/.vnc/*:1.log'

status:
	@$(DOCKER) ps --all \
		--filter "name=^/$(CONTAINER)$$" \
		--format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}'

url:
	@echo "Desktop: http://$(BIND_ADDRESS):$(PORT)"
	@echo "Preview: http://$(BIND_ADDRESS):$(PREVIEW_PORT)/preview.jpg"
	@echo "Health:  http://$(BIND_ADDRESS):$(PREVIEW_PORT)/healthz"
