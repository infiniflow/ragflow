#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

# Force using Bash to ensure the source command is available
SHELL := /bin/bash

# Environment variable definitions
VENV := .venv
PYTHON := $(VENV)/bin/python
UV := uv
ACTIVATE_SCRIPT := $(VENV)/bin/activate
SYS_PYTHON := python3
PYTHONPATH := $(shell pwd)

.PHONY: all setup ensure_env ensure_uv start stop restart build clean test logs

all: setup start

# 🌱 Initialize environment + install dependencies
setup: ensure_env ensure_uv
	@echo "📦 Installing dependencies with uv..."
	@$(UV) sync --python 3.13
	source $(ACTIVATE_SCRIPT) && \
	export PYTHONPATH=$(PYTHONPATH)
	@$(UV) pip install -r executor_manager/requirements.txt
	@echo "✅ Setup complete."

# 🔑 Ensure .env exists (copy from .env.example on first run)
ensure_env:
	@if [ ! -f ".env" ]; then \
		if [ -f ".env.example" ]; then \
			echo "📝 Creating .env from .env.example..."; \
			cp .env.example .env; \
		else \
			echo "⚠️ Warning: .env.example not found, creating empty .env"; \
			touch .env; \
		fi; \
	else \
		echo "✅ .env already exists."; \
	fi

# 🔧 Ensure uv is executable (install using system Python)
ensure_uv:
	@if ! command -v $(UV) >/dev/null 2>&1; then \
		echo "🛠️ Installing uv using system Python..."; \
		$(SYS_PYTHON) -m pip install -q --upgrade pip; \
		$(SYS_PYTHON) -m pip install -q uv || (echo "⚠️ uv install failed, check manually" && exit 1); \
	fi

# 🐳 Service control (using safer variable loading)
start:
	@echo "🚀 Starting services..."
	source $(ACTIVATE_SCRIPT) && \
	export PYTHONPATH=$(PYTHONPATH) && \
	[ -f .env ] && source .env || true && \
	bash scripts/start.sh

stop:
	@echo "🛑 Stopping services..."
	source $(ACTIVATE_SCRIPT) && \
	bash scripts/stop.sh

restart: stop start
	@echo "🔁 Restarting services..."

build:
	@echo "🔧 Building base sandbox images..."
	@if [ -f .env ]; then \
		source .env && \
		echo "🐍 Building base sandbox image for Python ($$SANDBOX_BASE_PYTHON_IMAGE)..." && \
		docker build -t "$$SANDBOX_BASE_PYTHON_IMAGE" ./sandbox_base_image/python && \
		echo "⬢ Building base sandbox image for Nodejs ($$SANDBOX_BASE_NODEJS_IMAGE)..." && \
		docker build -t "$$SANDBOX_BASE_NODEJS_IMAGE" ./sandbox_base_image/nodejs; \
	else \
		echo "⚠️ .env file not found, skipping build."; \
	fi

test:
	@echo "🧪 Running sandbox security tests..."
	source $(ACTIVATE_SCRIPT) && \
	export PYTHONPATH=$(PYTHONPATH) && \
	$(PYTHON) tests/sandbox_security_tests_full.py

logs:
	@echo "📋 Showing logs from api-server and executor-manager..."
	docker compose logs -f

# 🧹 Clean all containers and volumes
clean:
	@echo "🧹 Cleaning all containers and volumes..."
	@docker compose down -v || true
	@# Same discovery as scripts/stop.sh: the containers that exist, not a range derived from
	@# SANDBOX_EXECUTOR_MANAGER_POOL_SIZE, which misses a pool created at a different size, skips
	@# cleanup entirely when .env is gone, and never covers a container that failed to join the
	@# executor-manager queue.
	@# A failed `docker ps` and an empty result both collapse to an empty string, and only the
	@# first means the containers are still running. Tolerate grep's no-match exit, not docker's.
	@if ! all_containers="$$(docker ps -a --format '{{.Names}}')"; then \
		echo "❌ Could not list Docker containers; sandbox containers were NOT removed" >&2; \
		exit 1; \
	fi; \
	sandbox_containers="$$(printf '%s\n' "$$all_containers" | \
		grep -E '^sandbox_(python|nodejs)_[0-9]+$$' || true)"; \
	if [ -n "$$sandbox_containers" ]; then \
		while IFS= read -r container; do \
			echo "🧹 Deleting $$container..."; \
			docker rm -f "$$container" >/dev/null 2>&1 || true; \
		done <<<"$$sandbox_containers"; \
	else \
		echo "✅ No sandbox containers found"; \
	fi
