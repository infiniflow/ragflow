# RAGFlow Sandbox

A secure, pluggable code execution backend for RAGFlow and beyond.

## 🔧 Features

- ✅ **Seamless RAGFlow Integration** — Out-of-the-box compatibility with the `code` component.
- 🔐 **High Security** — Leverages [gVisor](https://gvisor.dev/) for syscall-level sandboxing.
- 🔧 **Customizable Sandboxing** — Easily modify `seccomp` settings as needed.
- 🧩 **Pluggable Runtime Support** — Easily extend to support any programming language.
- ⚙️ **Developer Friendly** — Get started with a single command using `Makefile`.

## 🏗 Architecture

<p align="center">
  <img src="asserts/code_executor_manager.svg" width="520" alt="Architecture Diagram">
</p>

## 🚀 Quick Start

### 📋 Prerequisites

#### Required

- Linux distro compatible with gVisor
- [gVisor](https://gvisor.dev/docs/user_guide/install/)
- Docker >= `24.0.0`
- Docker Compose >= `v2.26.1` like [RAGFlow](https://github.com/infiniflow/ragflow)
- [uv](https://docs.astral.sh/uv/) as package and project manager

#### Optional (Recommended)

- [GNU Make](https://www.gnu.org/software/make/) for simplified CLI management

---

### 🐳 Build Docker Base Images

We use isolated base images for secure containerized execution:

```bash
# Build base images manually
docker build -t sandbox-base-python:latest ./sandbox_base_image/python
docker build -t sandbox-base-nodejs:latest ./sandbox_base_image/nodejs

# OR use Makefile
make build
```

Then, build the executor manager image:

```bash
docker build -t sandbox-executor-manager:latest ./executor_manager
```

---

### 📦 Running with RAGFlow

1. Ensure gVisor is correctly installed.
2. Configure your `.env` in `docker/.env`:

   - Uncomment sandbox-related variables.
   - Enable sandbox profile at the bottom.
3. Add the following line to `/etc/hosts` as recommended:

   ```text
   127.0.0.1 sandbox-executor-manager
   ```

4. Start RAGFlow service.

---

### 🧭 Running Standalone

#### Manual Setup

1. Initialize environment:

   ```bash
   cp .env.example .env
   ```

2. Launch:

   ```bash
   docker compose -f docker-compose.yml up
   ```

3. Test:

   ```bash
   source .venv/bin/activate
   export PYTHONPATH=$(pwd)
   uv pip install -r executor_manager/requirements.txt
   uv run tests/sandbox_security_tests_full.py
   ```

#### With Make

```bash
make          # setup + build + launch + test
```

---

### 📈 Monitoring

```bash
docker logs -f sandbox-executor-manager  # Manual
make logs                                 # With Make
```

---

### 🧰 Makefile Toolbox

| Command           | Description                                      |
| ----------------- | ------------------------------------------------ |
| `make`            | Setup, build, launch and test all at once        |
| `make setup`      | Initialize environment and install uv            |
| `make ensure_env` | Auto-create `.env` if missing                    |
| `make ensure_uv`  | Install `uv` package manager if missing          |
| `make build`      | Build all Docker base images                     |
| `make start`      | Start services with safe env loading and testing |
| `make stop`       | Gracefully stop all services                     |
| `make restart`    | Shortcut for `stop` + `start`                    |
| `make test`       | Run full test suite                              |
| `make logs`       | Stream container logs                            |
| `make clean`      | Stop and remove orphan containers and volumes    |

---

## 🔐 Security

The RAGFlow sandbox is designed to balance security and usability, offering solid protection without compromising developer experience.

### ✅ gVisor Isolation

At its core, we use [gVisor](https://gvisor.dev/docs/architecture_guide/security/), a user-space kernel, to isolate code execution from the host system. gVisor intercepts and restricts syscalls, offering robust protection against container escapes and privilege escalations.

### 🔒 Optional seccomp Support (Advanced)

For users who need **zero-trust-level syscall control**, we support an additional `seccomp` profile. This feature restricts containers to only a predefined set of system calls, as specified in `executor_manager/seccomp-profile-default.json`.

> ⚠️ This feature is **disabled by default** to maintain compatibility and usability. Enabling it may cause compatibility issues with some dependencies.

#### To enable seccomp

1. Edit your `.env` file:

   ```dotenv
   SANDBOX_ENABLE_SECCOMP=true
   ```

2. Customize allowed syscalls in:

   ```
   executor_manager/seccomp-profile-default.json
   ```

   This profile is passed to the container with:

   ```bash
   --security-opt seccomp=/app/seccomp-profile-default.json
   ```

### 🧠 Python Code AST Inspection

In addition to sandboxing, Python code is **statically analyzed via AST (Abstract Syntax Tree)** before execution. Potentially malicious code (e.g. file operations, subprocess calls, etc.) is rejected early, providing an extra layer of protection.

---

This security model strikes a balance between **robust isolation** and **developer usability**. While `seccomp` can be highly restrictive, our default setup aims to keep things usable for most developers — no obscure crashes or cryptic setup required.

## 📦 Add Extra Dependencies for Supported Languages

Currently, the following languages are officially supported:

| Language | Priority |
| -------- | -------- |
| Python   | High     |
| Node.js  | Medium   |

### 🐍 Python

To add Python dependencies, simply edit the following file:

```bash
sandbox_base_image/python/requirements.txt
```

Add any additional packages you need, one per line (just like a normal pip requirements file).

### 🟨 Node.js

To add Node.js dependencies:

1. Navigate to the Node.js base image directory:

   ```bash
   cd sandbox_base_image/nodejs
   ```

2. Use `npm` to install the desired packages. For example:

   ```bash
   npm install lodash
   ```

3. The dependencies will be saved to `package.json` and `package-lock.json`, and included in the Docker image when rebuilt.

---

## 🤝 Contribution

Contributions are welcome!
