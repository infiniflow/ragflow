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

# Force using Bash
SHELL := /bin/bash

# Environment variable definitions
VENV := .venv
PYTHON := $(VENV)/bin/python
RUFF := $(VENV)/bin/ruff
SYS_PYTHON := python3
PYTHONPATH := $(shell pwd)

# Default paths to check (can be overridden)
CHECK_PATH ?= .
FIX_PATH ?= .

.PHONY: help ruff-install ruff-check ruff-fix ruff-format ruff-all lint format check-structure

help: ## Show this help message
	@echo "Available targets:"
	@echo "  make ruff-install    - Install ruff in virtual environment"
	@echo "  make ruff-check      - Run ruff lint checks (read-only)"
	@echo "  make ruff-fix        - Run ruff lint checks and auto-fix issues"
	@echo "  make ruff-format     - Format code with ruff"
	@echo "  make ruff-all        - Run format + check + fix (recommended)"
	@echo "  make lint            - Alias for ruff-check"
	@echo "  make format          - Alias for ruff-format"
	@echo "  make check-structure - Check code structure (imports, etc.)"
	@echo ""
	@echo "Usage examples:"
	@echo "  make ruff-check CHECK_PATH=test/                    # Check specific directory"
	@echo "  make ruff-fix CHECK_PATH=api/apps/user_app.py      # Fix specific file"
	@echo "  make ruff-all                                      # Format and fix all code"

# 🔧 Install ruff in virtual environment
ruff-install:
	@echo "📦 Installing ruff..."
	@if [ ! -d "$(VENV)" ]; then \
		echo "⚠️  Virtual environment not found. Creating one..."; \
		$(SYS_PYTHON) -m venv $(VENV); \
	fi
	@$(VENV)/bin/pip install -q ruff || (echo "❌ Failed to install ruff" && exit 1)
	@echo "✅ Ruff installed successfully"

# 🔍 Run ruff lint checks (read-only, no modifications)
ruff-check: ruff-install
	@echo "🔍 Running ruff lint checks on $(CHECK_PATH)..."
	@$(RUFF) check $(CHECK_PATH) || (echo "❌ Lint checks failed" && exit 1)
	@echo "✅ Lint checks passed"

# 🔧 Run ruff lint checks and auto-fix issues
ruff-fix: ruff-install
	@echo "🔧 Running ruff lint checks and auto-fixing issues on $(FIX_PATH)..."
	@$(RUFF) check --fix $(FIX_PATH) || (echo "⚠️  Some issues could not be auto-fixed" && exit 1)
	@echo "✅ Auto-fix complete"

# 💅 Format code with ruff
ruff-format: ruff-install
	@echo "💅 Formatting code with ruff on $(FIX_PATH)..."
	@$(RUFF) format $(FIX_PATH) || (echo "❌ Formatting failed" && exit 1)
	@echo "✅ Code formatted successfully"

# 🎯 Run all ruff operations: format + check + fix (recommended workflow)
ruff-all: ruff-install
	@echo "🎯 Running complete ruff workflow (format + check + fix)..."
	@echo "1️⃣ Formatting code..."
	@$(RUFF) format $(FIX_PATH) || (echo "❌ Formatting failed" && exit 1)
	@echo "2️⃣ Running lint checks and auto-fixing..."
	@$(RUFF) check --fix $(FIX_PATH) || (echo "⚠️  Some issues could not be auto-fixed" && exit 1)
	@echo "3️⃣ Final lint check..."
	@$(RUFF) check $(FIX_PATH) || (echo "❌ Final lint check failed" && exit 1)
	@echo "✅ All ruff checks passed!"

# 📋 Check code structure (imports, unused code, etc.)
check-structure: ruff-install
	@echo "📋 Checking code structure..."
	@echo "Checking for unused imports..."
	@$(RUFF) check --select F401 $(CHECK_PATH) || true
	@echo "Checking import order..."
	@$(RUFF) check --select I $(CHECK_PATH) || true
	@echo "✅ Structure check complete"

# Aliases for convenience
lint: ruff-check ## Alias for ruff-check
format: ruff-format ## Alias for ruff-format
fix: ruff-fix ## Alias for ruff-fix

