---
sidebar_position: 3
title: Launch Service from Source
sidebar_label: Launch Service from Source
slug: /launch_ragflow_from_source
sidebar_custom_props: {
  categoryIcon: LucideMonitorPlay
}
---

# Launch Service from Source

Build and run the complete Go API, admin, ingestor, and syncer services on your host, with supporting services in Docker. Run all commands from the repository root unless a step says otherwise. This guide uses the default Elasticsearch and MySQL configuration on Ubuntu 24.04 x86_64; the native ONNX Runtime archive used by this build targets Linux x86_64.

The RAGFlow open-source 1.0 DeepDoc backend uses CPU inference for layout
analysis, OCR, and table recognition.

All long-running RAGFlow processes started below use `bin/ragflow_server`.

## Prerequisites

- At least 4 CPU cores, 16 GB RAM, and 50 GB free disk space.
- Docker 24.0.0 or later and Docker Compose v2.26.1 or later.
- Go 1.27 or later, as declared in `go.mod` (check `go version`).
- CMake 4.0 or later, Clang 20, LLD 20, and PCRE2 development headers.
- Node.js 18.20.4 or later and npm for the frontend.
- Python 3.10 or later only for `ragflow_deps/download_go_deps.py`, which downloads the native libraries and model resources required by the Go build.

See the [Docker installation guide](https://docs.docker.com/engine/install/) if Docker is not installed. For Ubuntu 24.04 CMake installation details, see `internal/development.md` in the repository. Its Go installation example and `build.sh --help` may mention older versions; follow `go.mod` for the required Go version.

## 1. Get the source and build dependencies

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow
go version
cmake --version
clang++ --version
ld.lld --version
```

Install Go 1.27 or later if needed; make sure the Go executable selected by your shell meets the requirement in `go.mod`. On Ubuntu, install the compiler, linker, PCRE2 headers, Python virtual-environment support, and command-line tools used by the build with:

```bash
sudo apt update
sudo apt install -y clang-20 lld-20 libpcre2-dev python3-venv \
  build-essential binutils pkg-config curl ca-certificates
```

Ubuntu's enabled repositories may not provide `clang-20` and `lld-20`. If APT cannot locate them, configure the official LLVM APT repository for LLVM 20, then run the installation command again. Follow the repository instructions at [apt.llvm.org](https://apt.llvm.org/) rather than substituting an older LLVM release.

Make `clang++` resolve to Clang 20 and `ld.lld` to LLD 20 before building. Installing `lld-20` alone does not necessarily replace an older system default `ld.lld`. Check both versions above: an older LLD can produce a binary that builds successfully but fails before the Go server starts. Install CMake 4.0 or later separately if your distribution package is older; the Go development guide includes the Ubuntu 24.04 Kitware repository setup. The `binutils` package supplies `objcopy` and `nm`; do not ignore a missing-`objcopy` warning because skipping the RE2 symbol-renaming step can cause a runtime crash.

Download the native libraries and Go DeepDoc model weights with a small, isolated Python environment:

```bash
python3 -m venv /tmp/ragflow-go-download-venv
/tmp/ragflow-go-download-venv/bin/python -m pip install requests huggingface-hub
/tmp/ragflow-go-download-venv/bin/python ragflow_deps/download_go_deps.py
```

The downloader fetches the static libraries needed by `build.sh`, plus `det.ort`, `layout.ort`, `tsr.ort`, `rec.ort`, and `ocr.res` into `rag/res/deepdoc/`. These files are required by the in-process Go DeepDoc backend. Keep the server's working directory at the repository root so it can find them automatically; if you launch it elsewhere, set `DEEPDOC_MODEL_DIR` to the absolute path of `rag/res/deepdoc`.

The isolated download environment can be removed after the resources have been
prepared.

Build the C++ bindings and Go binaries:

```bash
bash build.sh --all
```

For a smaller production binary, use `bash build.sh --strip --all` instead. The build script also tries to place `ragflow_deps/cl100k_base.tiktoken` in the repository. If it reports that the BPE table could not be provisioned, download it before starting the server:

```bash
curl -fsSL -o ragflow_deps/cl100k_base.tiktoken https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken
```

Check that the newly built executable can load and parse API arguments:

```bash
./bin/ragflow_server --api --help
```

It should print API usage information and may exit with status 1. This command
only checks that the binary can load and parse arguments; it does not initialize
DeepDoc, connect to dependencies, or prove that the API can start. Complete the
runtime checks in section 5 after starting the dependencies and Go processes.

In an Ubuntu 24.04 x86_64 verification, `build.sh --all` completed with the
system's default LLD 18, but the resulting executable crashed during actual
server startup. Relinking with LLD 20 resolved the crash. Confirm that the
linker actually selected for the Go/C++ build is LLD 20; installing `lld-20`
alongside an older default is insufficient.

## 2. Start supporting services

With the default `conf/service_conf.yaml`, the host-run Go processes need Elasticsearch, MySQL, MinIO, NATS, Kvrocks, and ClickHouse. Start these services explicitly from the repository root:

The default Elasticsearch service requires `vm.max_map_count` of at least
`262144` on the Docker host. Check and set it before starting the containers:

```bash
sysctl vm.max_map_count
sudo sysctl -w vm.max_map_count=262144
```

The change made with `sysctl -w` is temporary. To preserve it after a reboot,
add `vm.max_map_count=262144` to `/etc/sysctl.conf`.

```bash
docker compose --env-file docker/.env-go -f docker/docker-compose-base.yml up -d --wait es01 mysql minio nats kvrocks clickhouse
docker compose --env-file docker/.env-go -f docker/docker-compose-base.yml ps
```

The base Compose file also defines an unprofiled Redis service. Starting every service with `up -d` can make Redis and Kvrocks compete for host port 6379; the explicit service list above starts Kvrocks for the Go backend. Compose may print a warning that `REDIS_PORT` is unset because the unused Redis service is still parsed.

Check that the services are ready before migrating. For this host-run setup, edit **`conf/service_conf.yaml`** if you changed the published ports or credentials in `docker/.env-go`. The defaults include MySQL at `localhost:3306`, Elasticsearch at `localhost:1200`, MinIO at `localhost:9000`, Kvrocks at `localhost:6379`, NATS at `localhost:4222`, and ClickHouse at `localhost:9900`. Do not edit `docker/service_conf.yaml.template` for a Go process launched directly on the host, and no `/etc/hosts` entries for Docker service names are needed.

## 3. Migrate and launch the Go backend

Run the migration once before starting any server process. The examples below
use development mode, so keep `RAGFLOW_DEV_MODE=true` consistent across the
migration and every server process. For a production build whose version
matches the database migration marker, omit this variable from every command.

```bash
RAGFLOW_DEV_MODE=true ./bin/ragflow_server --migrate
```

Then start each Go mode in a separate terminal, from the repository root. These
four modes are the complete backend service chain. Start Admin first so API,
Ingestor, and Syncer can report their heartbeats:

```bash
# Terminal 1: admin (target port 9381)
RAGFLOW_DEV_MODE=true ./bin/ragflow_server --admin
```

```bash
# Terminal 2: API (target port 9380)
RAGFLOW_DEV_MODE=true ./bin/ragflow_server --api
```

```bash
# Terminal 3: document ingestion
RAGFLOW_DEV_MODE=true ./bin/ragflow_server --ingestor
```

Start the Go syncer in another terminal:

```bash
RAGFLOW_DEV_MODE=true ./bin/ragflow_server --syncer
```

After the migration completes, `RAGFLOW_DEV_MODE=true bash build.sh --run` can
be used as a shortcut to start the Go Admin, Ingestor, and API modes. The
shortcut does not start Syncer; run the Syncer command above separately when
the complete service chain is required. It does not replace the standalone
migration step.

If you changed a dependency's published port or credentials, update
`conf/service_conf.yaml` before starting the Go processes. In particular,
`kvrocks.host` is a single `host:port` value; the default source configuration
is `localhost:6379`. Do not rely on `KVROCKS_HOST` and `KVROCKS_PORT` to override
this source configuration.

`RAGFLOW_DEV_MODE=true` bypasses the code-versus-database version check. Use it
only for development, not to run an older production binary against a newer
database. The standalone `--migrate` action must still complete first.

If startup reports `no in-process DeepDoc backend serving`, check that all five model files above are in `rag/res/deepdoc/`, then re-run the Go dependency downloader and rebuild if necessary. If the tokenizer reports a missing `cl100k_base.tiktoken`, ensure the BPE table is in `ragflow_deps/`.

## 4. Start the frontend

In another terminal:

```bash
cd web
npm install
API_PROXY_SCHEME=go npm run dev
```

The target `go` proxy configuration sends API requests to port 9380 and Admin
requests to port 9381. Open
[http://127.0.0.1:9222/](http://127.0.0.1:9222/) unless Vite prints a different
frontend port.

## 5. Verify the startup

From a separate terminal, check the frontend, an API request through its Go proxy, and the ClickHouse HTTP health endpoint:

```bash
curl -fsS http://127.0.0.1:9380/api/v1/system/healthz
curl -fsS -o /dev/null -w 'frontend: HTTP %{http_code}\n' http://127.0.0.1:9222/
curl -fsS http://127.0.0.1:9222/api/v1/system/version
curl -fsS http://127.0.0.1:8123/ping
```

The direct Go API health check and frontend should return HTTP 200, the version
request should return JSON with `"code":0`, and ClickHouse should respond `Ok.`.
These runtime checks confirm that the Go API, frontend proxy, and ClickHouse are
reachable; also confirm that the startup logs report the in-process DeepDoc
backend as registered before testing your intended RAGFlow workflow.

## 6. Stop the services

Press `Ctrl+C` in the frontend and Go server terminals. To stop the dependency containers started in step 2:

```bash
docker compose --env-file docker/.env-go -f docker/docker-compose-base.yml stop es01 mysql minio nats kvrocks clickhouse
```

`stop` preserves the dependency containers for the next development session.
To remove the containers and Compose network while keeping named data volumes,
use `docker compose --env-file docker/.env-go -f docker/docker-compose-base.yml down`.
