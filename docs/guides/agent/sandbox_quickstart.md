---
sidebar_position: 20
slug: /sandbox_quickstart
---

# Sandbox quickstart

A secure, pluggable code execution backend designed for RAGFlow and other applications requiring isolated code execution environments.

## Features: 

- Seamless RAGFlow Integration — Works out-of-the-box with the code component of RAGFlow.
- High Security — Uses gVisor for syscall-level sandboxing to isolate execution.
- Customisable Sandboxing — Modify seccomp profiles easily to tailor syscall restrictions.
- Pluggable Runtime Support — Extendable to support any programming language runtime.
- Developer Friendly — Quick setup with a convenient Makefile.

## Architecture

The architecture consists of isolated Docker base images for each supported language runtime, managed by the executor manager service. The executor manager orchestrates sandboxed code execution using gVisor for syscall interception and optional seccomp profiles for enhanced syscall filtering.

## Prerequisites

- Linux distribution compatible with gVisor.
- gVisor installed and configured.
- Docker version 24.0.0 or higher.
- Docker Compose version 2.26.1 or higher (similar to RAGFlow requirements).
- uv package and project manager installed.
- (Optional) GNU Make for simplified command-line management.

## Build Docker base images

The sandbox uses isolated base images for secure containerised execution environments.

Build the base images manually:

```bash
docker build -t sandbox-base-python:latest ./sandbox_base_image/python
docker build -t sandbox-base-nodejs:latest ./sandbox_base_image/nodejs
```

Alternatively, build all base images at once using the Makefile:

```bash
make build
```

Next, build the executor manager image:

```bash
docker build -t sandbox-executor-manager:latest ./executor_manager
```

## Running with RAGFlow 

1. Verify that gVisor is properly installed and operational.

2. Configure the .env file located at docker/.env:

- Uncomment sandbox-related environment variables.
- Enable the sandbox profile at the bottom of the file.

3. Add the following entry to your /etc/hosts file to resolve the executor manager service:

```bash
127.0.0.1 es01 infinity mysql minio redis sandbox-executor-manager
```

4. Start the RAGFlow service as usual.

## Running standalone

### Manual setup

1. Initialize the environment variables:

```bash
cp .env.example .env
```

2. Launch the sandbox services with Docker Compose:

```bash
docker compose -f docker-compose.yml up
```

3. Test the sandbox setup:

```bash
source .venv/bin/activate
export PYTHONPATH=$(pwd)
uv pip install -r executor_manager/requirements.txt
uv run tests/sandbox_security_tests_full.py
```

### Using Makefile

Run all setup, build, launch, and tests with a single command:

```bash
make
```

### Monitoring

To follow logs of the executor manager container:

```bash
docker logs -f sandbox-executor-manager
```

Or use the Makefile shortcut:

```bash
make logs
```