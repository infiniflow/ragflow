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

Build and run the complete Go API, admin, ingestor, and syncer services on your host, with supporting services in Docker. Run all commands from the repository root unless a step says otherwise. The build is developed and tested on Ubuntu 24.04 x86_64, which is the supported platform. Native dependency archives are also published for darwin (amd64/arm64); a source build on macOS may work with the Homebrew toolchain in step 1, but macOS is not part of project CI and the steps are provided as-is.

:::note
This page covers running the Go process directly on your host. The prebuilt Docker image targets Linux x86_64 only.
:::

The RAGFlow open-source 1.0 DeepDoc backend uses CPU inference for layout analysis, OCR, and table recognition.

All long-running RAGFlow processes started below use `bin/ragflow_server`.

## Prerequisites

- A recommended starting configuration of 4 CPU cores, 16 GB RAM, and 50 GB free disk space. Actual requirements depend on the selected document engine, local models, data volume, parsing workload, and concurrency.
- Docker 24.0.0 or later and Docker Compose v2.26.1 or later.
- Go 1.27 or later, as declared in `go.mod` (check `go version`).
- CMake 4.0 or later, Clang 20, and PCRE2 development headers. On Linux, LLD 20 is also required.
- Node.js 18.20.4 or later and npm for the frontend.
- Python 3.10 or later for `tools/scripts/download_dev_deps.py` and `deps/download_deps.py`, which download the native libraries and model resources required by the Go build.

See the [Docker installation guide](https://docs.docker.com/engine/install/) if Docker is not installed. Use the Go version declared in `go.mod` and the compiler versions listed above when preparing the build environment.

## 1. Get the source and build dependencies

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow
go version
cmake --version
clang++ --version
ld.lld --version   # Linux only
```

Install Go 1.27 or later if needed; make sure the Go executable selected by your shell meets the requirement in `go.mod`. Install `uv` as well; the dependency downloaders run through it. On Ubuntu, install the compiler, linker, PCRE2 headers, Python 3, and command-line tools used by the build with:

```bash
sudo apt update
sudo apt install -y clang-20 lld-20 libpcre2-dev python3-venv \
  build-essential binutils pkg-config curl ca-certificates
```

Ubuntu's enabled repositories may not provide `clang-20` and `lld-20`. If APT cannot locate them, configure the official LLVM APT repository for LLVM 20, then run the installation command again. Follow the repository instructions at [apt.llvm.org](https://apt.llvm.org/) rather than substituting an older LLVM release.

Make `clang++` resolve to Clang 20 and `ld.lld` to LLD 20 before building. Installing `lld-20` alone does not necessarily replace an older system default `ld.lld`. Check both versions above: an older LLD can produce a binary that builds successfully but fails before the Go server starts. Install CMake 4.0 or later separately if your distribution package is older; the Go development guide includes the Ubuntu 24.04 Kitware repository setup. The `binutils` package supplies `objcopy` and `nm`; do not ignore a missing-`objcopy` warning because skipping the RE2 symbol-renaming step can cause a runtime crash.

On macOS, contributors provide the toolchain with Homebrew:

```bash
brew install llvm@20 cmake simde pcre2
```

- Put Homebrew LLVM 20 first on `PATH` and point `CC` / `CXX` at its Clang.
- CMake must be 4.0 or later; install it separately if Homebrew provides an older version.
- `simde` supplies the x86 SSE/AVX headers used in ARM builds.
- LLD is only required on Linux.

Export before running `build.sh`, deriving the paths from the installed Homebrew prefix (`/opt/homebrew/opt/llvm@20` on Apple Silicon, `/usr/local/opt/llvm@20` on Intel):

```bash
LLVM_PREFIX="$(brew --prefix llvm@20)"
export PATH="$LLVM_PREFIX/bin:$PATH"
export CC="$LLVM_PREFIX/bin/clang"
export CXX="$LLVM_PREFIX/bin/clang++"
```

Known limitation: `bash build.sh --all` fails to configure on Intel Macs because the build passes Go's `amd64` to CMake as `CMAKE_OSX_ARCHITECTURES`, while the PCRE2 architecture check expects `x86_64`. See [#22759](https://github.com/infiniflow/ragflow/issues/22759); until it is fixed, pass a matching PCRE2 archive with `RAGFLOW_CMAKE_ARGS=-DPCRE2_STATIC_LIB=/path/to/libpcre2-8.a`. Apple Silicon is unaffected.

macOS builds are not covered by project CI and are not independently verified by the maintainers; report build problems as issues.

Download the native static libraries and the runtime model / tokenizer resources with the two dependency downloaders (`uv run` installs each script's declared Python dependencies):

```bash
uv run tools/scripts/download_dev_deps.py   # local-dev one-stop: native .a + cl100k, DeepDoc .ort, tokenizer assets
```

`download_dev_deps.py` is the local-dev one-stop: it fetches the static libraries needed by `build.sh` (pdfium / pdf_oxide / office_oxide / onnxruntime → `~/ragflow-native-libs`) AND the runtime resources the backend loads at runtime — `det.ort`, `layout.ort`, `tsr.ort`, `rec.ort`, `ocr.res` into `internal/rag/res/deepdoc/` (required by the in-process Go DeepDoc backend) and `deps/cl100k_base.tiktoken` and the embedding tokenizer assets under `deps/huggingface.co/`. Run it once and the local environment is ready; already-present files are skipped on re-run. The image-facing `deps/download_deps.py` fetches the same runtime resources and skips the native `.a`; run it only when building the dependency image or in CI. Native archives are published for linux/darwin on amd64/arm64; the downloader selects the host platform (override with `RAGFLOW_TARGET_OS` / `RAGFLOW_TARGET_ARCH`). Keep the server's working directory at the repository root so it can find the DeepDoc models automatically; if you launch it elsewhere, set `DEEPDOC_MODEL_DIR` to the absolute path of `internal/rag/res/deepdoc`.

Skip either script if you have prepared the same resources by other means.

Build the C++ bindings and Go binaries:

```bash
bash build.sh --all
```

For a smaller production binary, use `bash build.sh --strip --all` instead. The dependency downloader prepares `deps/cl100k_base.tiktoken`. If the table is missing, download it before starting the server:

```bash
curl -fsSL -o deps/cl100k_base.tiktoken https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken
```

Check that the newly built executable can load and parse its arguments:

```bash
./bin/ragflow_server --help
```

It should print server usage information and exit successfully. This command only checks that the binary can load and parse arguments; it does not initialize DeepDoc, connect to dependencies, or prove that the API can start. Complete the runtime checks in section 5 after starting the dependencies and Go processes.

On Linux, the Go/C++ build requires LLD 20. Confirm that `ld.lld --version` reports LLD 20 before building; merely installing `lld-20` is insufficient when an older `ld.lld` remains the system default. If the binary builds successfully but exits or crashes before reaching Go `main`, verify the selected linker and rebuild with LLD 20. macOS uses the system linker and does not require LLD.

To prepare the resource image used by the Go Dockerfile, first run `uv run deps/download_deps.py` (above) to populate `deps/`, then build from `deps/`:

```bash
cd deps
docker build -f Dockerfile -t infiniflow/ragflow_deps:latest .
cd ..
```

The downloader prepares the five Go DeepDoc files under `deps/huggingface.co/InfiniFlow/deepdoc/` for the resource image, as well as `internal/rag/res/deepdoc/` for local use. The Go Dockerfile copies these models into the runtime model directory. Analyzer dictionaries and fonts are installed separately by the Go Dockerfile.

## 2. Start supporting services

With the default `conf/service_conf.yaml`, the host-run Go processes need Elasticsearch, MySQL, MinIO, NATS, Kvrocks, and ClickHouse. Start these services explicitly from the repository root:

The default Elasticsearch service requires `vm.max_map_count` of at least `262144` in the Docker engine's Linux kernel. With Docker Engine installed directly on a Linux host, check and set it on the host before starting the containers:

```bash
sysctl vm.max_map_count
sudo sysctl -w vm.max_map_count=262144
```

The change made with `sysctl -w` is temporary. To preserve it after a reboot, add `vm.max_map_count=262144` to `/etc/sysctl.conf`.

Docker Desktop runs the engine inside its own Linux VM, where a host-side `sysctl` does not apply. The parameter is preconfigured in recent Docker Desktop releases; if Elasticsearch still reports a `max virtual memory areas` error, adjust it inside the VM as described in `internal/development.md`.

```bash
docker compose --env-file docker/.env -f docker/docker-compose-base.yml up -d --wait es01 mysql minio nats kvrocks clickhouse
docker ps
```

The base Compose file also defines an unprofiled Redis service. Starting every service with `up -d` can make Redis and Kvrocks compete for host port 6379; the explicit service list above starts Kvrocks for the Go backend. Compose may print a warning that `REDIS_PORT` is unset because the unused Redis service is still parsed.

Check that the services are ready before migrating. For this host-run setup, edit **`conf/service_conf.yaml`** if you changed the published ports or credentials in `docker/.env`. The defaults include MySQL at `localhost:3306`, Elasticsearch at `localhost:1200`, MinIO at `localhost:9000`, Kvrocks at `localhost:6379`, NATS at `localhost:4222`, and ClickHouse at `localhost:9900`. Do not edit `docker/service_conf.yaml.template` for a Go process launched directly on the host, and no `/etc/hosts` entries for Docker service names are needed.

If you publish Kvrocks on a non-default host port, set the same `host:port` in `kvrocks.host`, either in `conf/service_conf.yaml` or in a config copy passed with `-f`; the host-run server reads the address from the configuration file, while `KVROCKS_PORT` in `docker/.env` only changes the container's published port.

## 3. Launch the Go backend

The analyzer needs dictionary files at startup. Clone the dictionary repository once and point `RAGFLOW_DICT_PATH` at it:

```bash
git clone --depth 1 https://github.com/infiniflow/resource.git "$HOME/ragflow-native-libs/resource"
export RAGFLOW_DICT_PATH="$HOME/ragflow-native-libs/resource"
```

Then start the server from the repository root:

```bash
./bin/ragflow_server
```

The OSS server runs all services in one Go process. It runs database migrations first, starts Admin and initializes the
superuser, waits for API readiness, then starts Ingestor, Syncer and Chatter.
Services run as goroutines with shared connections, model resources and tokenizers.
All services stop together on cancellation or a service failure.
Admin defaults to `9381` and API to `9380`. `bash build.sh --run` builds and
starts the same server.

Use `-f` / `--config` to select a configuration file. Other common options are
`-v` / `--version`, `--log-level`, `--profile`, and `-h` / `--help`.
Set service names with `<service>.name` or `RAGFLOW_<SERVICE>_NAME`.
Set admin discovery with `admin.host` / `admin.http_port` or
`RAGFLOW_ADMIN_HOST` / `RAGFLOW_ADMIN_HTTP_PORT`.
Set worker and page concurrency with `ingestor.concurrent_workers` /
`ingestor.page_concurrency` or `RAGFLOW_INGESTOR_CONCURRENT_WORKERS` /
`RAGFLOW_INGESTOR_PAGE_CONCURRENCY`. Environment variables override the file.

Update `conf/service_conf.yaml` for host-run dependency ports and credentials.
`kvrocks.host` is a single `host:port` value, defaulting to `localhost:6379`.
Leave `RAGFLOW_DEV_MODE` unset in production; it bypasses downgrade protection.

If startup reports `no in-process DeepDoc backend serving`, check that all five model files above are in `internal/rag/res/deepdoc/`, then re-run the Go dependency downloader and rebuild if necessary. If startup fails while creating or loading the base analyzer, check that `RAGFLOW_DICT_PATH` points at the cloned `resource` directory above. If the tokenizer reports a missing `cl100k_base.tiktoken`, ensure the BPE table is in `deps/`.

## 4. Start the frontend

In another terminal:

```bash
cd web
npm install
npm run dev
```

The frontend routes requests to the Go backend by default: regular `/api` and `/v1` requests use port `9380`, while `/api/v1/admin` requests use port `9381`.
Open http://127.0.0.1:9222/ unless Vite prints a different frontend port.

## 5. Verify the startup

From a separate terminal, check the frontend, an API request through its Go proxy, and the ClickHouse HTTP health endpoint:

```bash
curl -fsS http://127.0.0.1:9380/api/v1/system/healthz
curl -fsS -o /dev/null -w 'frontend: HTTP %{http_code}\n' http://127.0.0.1:9222/
curl -fsS http://127.0.0.1:9222/api/v1/system/version
curl -fsS http://127.0.0.1:8123/ping
```

The direct Go API health check and frontend should return HTTP 200, the version request should return JSON with `"code":0`, and ClickHouse should respond `Ok.`. These runtime checks confirm that the Go API, frontend proxy, and ClickHouse are reachable; also confirm that the startup logs report the in-process DeepDoc backend as registered before testing your intended RAGFlow workflow.

## 6. Stop the services

Press `Ctrl+C` in the frontend and Go server terminals. To stop the dependency containers started in step 2:

```bash
docker compose --env-file docker/.env -f docker/docker-compose-base.yml stop \
  es01 mysql minio nats kvrocks clickhouse
```

`stop` preserves the dependency containers for the next development session. To remove the containers and Compose network while keeping named data volumes, use `docker compose --env-file docker/.env -f docker/docker-compose-base.yml down`.
