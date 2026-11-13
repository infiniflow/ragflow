#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$SCRIPT_DIR"

# EE (enterprise) build extensions - sourced so the OSS body below stays
# untouched and merge conflicts against the EE branch are limited to this
# single line plus the guarded hook call inside run_native_integration_tests.
[[ -f "${SCRIPT_DIR}/build_ee.sh" ]] && source "${SCRIPT_DIR}/build_ee.sh"
# Version-keyed native-lib resolution: each native lib sits under a
# v<version>/ subdir of the user cache (~/ragflow-native-libs/<lib>/v<version>/),
# so a single mounted cache holds several versions at once. The version numbers
# come from go.mod (office_oxide / pdf_oxide), environments.go (ORT) and
# native_lib_version.sh (pdfium) — see that file for the single source of truth.
source "${SCRIPT_DIR}/tools/scripts/native_lib_version.sh"
# ort_stamp.sh: pure helper that computes the relative, read-only-safe ORT
# version-stamp symlink target (Fix #1/#2).
source "${SCRIPT_DIR}/tools/scripts/ort_stamp.sh"
# ort_present.sh: pure helper that decides whether the required ORT static lib
# is already in the version+platform-keyed user cache (Fix: version/platform
# aware presence gate).
source "${SCRIPT_DIR}/tools/scripts/ort_present.sh"

# Native static-lib user-cache root. Overridable via RAGFLOW_NATIVE_LIBS_DIR so
# CI can mount the cache at a fixed host path (sep-tests.yml Slice 3); defaults
# to ~/ragflow-native-libs. Captured once here and reused by every <lib>/v<version>
# prefix below so the whole script agrees on a single cache root.
NATIVE_LIBS_DIR="$(ragflow_native_libs_dir)"
# Build directories
CPP_DIR="$PROJECT_ROOT/internal/binding/cpp"
BUILD_DIR="$CPP_DIR/cmake-build-release"
RAGFLOW_SERVER_BINARY="$PROJECT_ROOT/bin/ragflow_server"
RAGFLOW_CLI_BINARY="$PROJECT_ROOT/bin/ragflow-cli"

# Strip symbols from Go binaries (set via --strip / -s)
STRIP_SYMBOLS=""

# Extra Go build tags appended to every build (comma separated). This tree
# carries the enterprise sources, so RAGFLOW_GO_TAGS=ee selects them through the
# //go:build ee constraints; without it the same command lines build the
# community edition.
EXTRA_GO_TAGS="${RAGFLOW_GO_TAGS:-}"
GO_BUILD_TAGS="cgo,static,sonic"
[ -n "$EXTRA_GO_TAGS" ] && GO_BUILD_TAGS="$GO_BUILD_TAGS,$EXTRA_GO_TAGS"
# The go test calls below predate `go build`, so they spell tags both
# comma-separated ("cgo,static") and space-separated ("cgo static
# integration"). Both forms accept the extra tags, so keep one variant each.
EXTRA_TAGS_COMMA=""
EXTRA_TAGS_SPACE=""
if [ -n "$EXTRA_GO_TAGS" ]; then
    EXTRA_TAGS_COMMA=",$EXTRA_GO_TAGS"
    EXTRA_TAGS_SPACE=" $EXTRA_GO_TAGS"
fi

# Native static library settings. These are the user-cache paths (~/ragflow-native-libs/).
# Each lib lives under a version-keyed subdir (v<version>/) placed there by
# tools/scripts/download_dev_deps.py (idempotent, version-pinned) — run once by a
# developer, or mounted in from the build host on CI. There is NO /opt pre-bake and
# NO flat (version-less) fallback: a missing version dir means the cache is
# incomplete and build.sh triggers the downloader itself (see setup_cgo_env).

# office_oxide native library settings — static linking. Version is derived from
# go.mod (single source of truth, exactly as tools/scripts/download_dev_deps.py
# derives it). An empty value (module absent from go.mod) would produce a broken
# .../lib/v link path, so it is fatal. The prefix is set after the version, since
# it embeds the version.
OFFICE_OXIDE_VERSION="$(go_module_version "github.com/yfedoseev/office_oxide/go" "$PROJECT_ROOT")"
if [ -z "$OFFICE_OXIDE_VERSION" ]; then
    echo -e "${RED}Error: could not read office_oxide version from go.mod${NC}" >&2; exit 1
fi
OFFICE_OXIDE_PREFIX="${NATIVE_LIBS_DIR}/office_oxide/v${OFFICE_OXIDE_VERSION}"

# pdfium native library settings — static linking (kognitos/pdfium-static).
# Version is read from the single source of truth tools/scripts/native_lib_version.sh
# (pdfium_static_version), exactly as download_dev_deps.py does, so the bare build
# number lives in exactly one place. An empty value is fatal.
PDFIUM_STATIC_VERSION="$(pdfium_static_version "$PROJECT_ROOT")"
if [ -z "$PDFIUM_STATIC_VERSION" ]; then
    echo -e "${RED}Error: could not read pdfium version from native_lib_version.sh${NC}" >&2; exit 1
fi
PDFIUM_STATIC_PREFIX="${NATIVE_LIBS_DIR}/pdfium-static/v${PDFIUM_STATIC_VERSION}"

# pdf_oxide native library settings — static linking (go-ffi tarball). Version
# is read from go.mod (single source of truth) instead of a hardcoded constant,
# so bumping the dependency needs no build.sh edit. The empty case is fatal for
# the same reason as office_oxide. The prefix is set after the version.
PDF_OXIDE_VERSION="$(go_module_version "github.com/yfedoseev/pdf_oxide/go" "$PROJECT_ROOT")"
if [ -z "$PDF_OXIDE_VERSION" ]; then
    echo -e "${RED}Error: could not read pdf_oxide version from go.mod${NC}" >&2; exit 1
fi
PDF_OXIDE_PREFIX="${NATIVE_LIBS_DIR}/pdf_oxide/v${PDF_OXIDE_VERSION}"

# onnxruntime native library settings — static linking for the in-process
# (Go) DeepDoc backend. libonnxruntime*.a is linked into the server binary
# (--undefined=OrtGetApiBase + --dynamic-list, no --whole-archive); OrtGetApiBase
# is then resolved via dlopen(NULL) (the process-global symbol table, not the
# executable's own path), so no libonnxruntime.so is needed at runtime.
# Downloaded by tools/scripts/download_dev_deps.py into onnxruntime/static_lib.
ONNXRUNTIME_STATIC_PREFIX="${NATIVE_LIBS_DIR}/onnxruntime/static_lib"

# ── target platform detection ──────────────────────────────────────────
# Defaults to the build host (uname), overridable with RAGFLOW_TARGET_OS /
# RAGFLOW_TARGET_ARCH for cross-baked dependency downloads. goos ∈ {linux,
# darwin}; goarch ∈ {amd64, arm64}.
TARGET_OS=""
TARGET_ARCH=""
detect_target_platform() {
    TARGET_OS="${RAGFLOW_TARGET_OS:-$(uname -s)}"
    TARGET_ARCH="${RAGFLOW_TARGET_ARCH:-$(uname -m)}"
    case "$TARGET_OS" in
        Linux)  TARGET_OS="linux" ;;
        Darwin) TARGET_OS="darwin" ;;
    esac
    case "$TARGET_ARCH" in
        x86_64|amd64) TARGET_ARCH="amd64" ;;
        arm64|aarch64) TARGET_ARCH="arm64" ;;
    esac
    export TARGET_OS TARGET_ARCH
}

# Map the target platform to the release asset filename for each native lib.
# These mirror tools/scripts/download_dev_deps.py's *_ASSETS maps and the
# verified upstream release assets.
office_oxide_asset() {
    case "${TARGET_OS}_${TARGET_ARCH}" in
        linux_amd64)  echo "native-linux-x86_64.tar.gz" ;;
        linux_arm64)  echo "native-linux-aarch64.tar.gz" ;;
        darwin_amd64) echo "native-macos-x86_64.tar.gz" ;;
        darwin_arm64) echo "native-macos-aarch64.tar.gz" ;;
    esac
}
pdfium_asset() {
    case "${TARGET_OS}_${TARGET_ARCH}" in
        linux_amd64)  echo "pdfium-linux-x64-static.tgz" ;;
        linux_arm64)  echo "pdfium-linux-arm64-static.tgz" ;;
        darwin_amd64) echo "pdfium-mac-x64-static.tgz" ;;
        darwin_arm64) echo "pdfium-mac-arm64-static.tgz" ;;
    esac
}
pdf_oxide_asset() {
    case "${TARGET_OS}_${TARGET_ARCH}" in
        linux_amd64)  echo "pdf_oxide-go-ffi-linux-amd64.tar.gz" ;;
        linux_arm64)  echo "pdf_oxide-go-ffi-linux-arm64.tar.gz" ;;
        darwin_amd64) echo "pdf_oxide-go-ffi-darwin-amd64.tar.gz" ;;
        darwin_arm64) echo "pdf_oxide-go-ffi-darwin-arm64.tar.gz" ;;
    esac
}
# Release target token of the ONNX Runtime zip published under
# infiniflow/ragflow-build tag onnxruntime-v{version}: linux-x86_64,
# linux-aarch64, osx-arm64, osx-universal (no osx-x86_64 is published —
# Intel Macs use the universal archive). Mirrors download_dev_deps.py's
# ort_target().
ort_release_target() {
    case "${TARGET_OS}_${TARGET_ARCH}" in
        linux_amd64)  echo "linux-x86_64" ;;
        linux_arm64)  echo "linux-aarch64" ;;
        darwin_amd64) echo "osx-universal" ;;
        darwin_arm64) echo "osx-arm64" ;;
    esac
}
# Top-level dir prefix of the extracted ONNX Runtime static lib under
# $ONNXRUNTIME_STATIC_PREFIX, e.g. onnxruntime-linux-x64-static_lib-.
# download_dev_deps.py renames each release zip's top-level dir to
# onnxruntime-<os>-<arch>-static_lib-<version>[-glibc2_28] (see its
# ort_normalized_dir), so this prefix — plus the DeepDocORTVersion parsed
# below — pins the dir we link. Keeping it platform-specific stops a shared
# cache from linking an ORT built for another OS/arch. The arch token is not
# GOARCH: Linux uses x64/aarch64, and macOS uses arm64 or universal (Intel Macs
# share the universal archive — ragflow-build publishes no osx-x86_64).
ort_static_lib_prefix() {
    case "${TARGET_OS}_${TARGET_ARCH}" in
        linux_amd64)  echo "onnxruntime-linux-x64-static_lib-" ;;
        linux_arm64)  echo "onnxruntime-linux-aarch64-static_lib-" ;;
        darwin_amd64) echo "onnxruntime-osx-universal-static_lib-" ;;
        darwin_arm64) echo "onnxruntime-osx-arm64-static_lib-" ;;
    esac
}

# ── native static library provisioning ──────────────────────────────────
# The native .a files live in the version-keyed user cache
# (~/ragflow-native-libs/<lib>/v<version>/), placed there by
# tools/scripts/download_dev_deps.py — idempotently and version-pinned — which a
# developer runs once, and which CI mounts in from the build host. There is NO
# /opt pre-bake and NO flat (version-less) fallback any more: a missing version
# dir means the cache is incomplete, so build.sh triggers the downloader itself
# (controlled auto-download) rather than silently linking a stale or wrong lib.

# True when every native lib the Go build links is already in the versioned user
# cache for the target platform. Used to skip the network entirely on an
# already-provisioned host.
_native_libs_present() {
    [ -f "${OFFICE_OXIDE_PREFIX}/lib/liboffice_oxide.a" ] || return 1
    [ -f "${OFFICE_OXIDE_PREFIX}/include/office_oxide_c/office_oxide.h" ] || return 1
    [ -f "${PDFIUM_STATIC_PREFIX}/lib/libpdfium.a" ] || return 1
    [ -f "${PDF_OXIDE_PREFIX}/lib/${TARGET_OS}_${TARGET_ARCH}/libpdf_oxide.a" ] || return 1
    # ORT must match the SAME version+platform-keyed dir that check_onnxruntime_deps
    # links (*/${ort_dir_prefix}${ort_version}*/lib/*.a). A shared cache holding
    # only an OLD version or a DIFFERENT platform's ORT must be reported missing so
    # the controlled auto-downloader runs, instead of being silently treated as
    # present and failing only at link time.
    if [ -z "$_TARGET_PLATFORM_DETECTED" ]; then
        detect_target_platform
        _TARGET_PLATFORM_DETECTED=1
    fi
    local ort_version ort_dir_prefix
    ort_version="$(deepdoc_ort_version "$PROJECT_ROOT")" || return 1
    ort_dir_prefix="$(ort_static_lib_prefix)"
    ragflow_ort_present "$ONNXRUNTIME_STATIC_PREFIX" "$ort_dir_prefix" "$ort_version" || return 1
    return 0
}

# Controlled auto-download: if the versioned native libs are not yet in the user
# cache, run the one-stop downloader ONCE. It is idempotent and version-pinned,
# so it can only land the exact pinned .a — never a wrong version. Fail loud only
# when the cache is still incomplete afterwards (a locked-down host with no
# network and no pre-placed libs).
_ensure_native_libs_installed() {
    _native_libs_present && return 0
    echo "One or more native static libraries are missing from the user cache;"
    echo "  running downloader: uv run tools/scripts/download_dev_deps.py"
    ( cd "$PROJECT_ROOT" && uv run tools/scripts/download_dev_deps.py ) || {
        echo -e "${RED}Error: native library downloader failed.${NC}" >&2
        exit 1
    }
    if _native_libs_present; then
        return 0
    fi
    echo -e "${RED}Error: native static libraries still missing after download.${NC}" >&2
    echo "  Pre-place the version-keyed dirs manually on a locked-down host:" >&2
    echo "    ${OFFICE_OXIDE_PREFIX}" >&2
    echo "    ${PDFIUM_STATIC_PREFIX}" >&2
    echo "    ${PDF_OXIDE_PREFIX}" >&2
    echo "    ${ONNXRUNTIME_STATIC_PREFIX}" >&2
    exit 1
}

# ── cl100k_base BPE table ─────────────────────────────────────────────
# internal/tokenizer loads this table from disk only (no network) and PANICS in
# NumTokensFromString / TrimContentToTokenLimit when it is absent: a deployment
# that forgot the file must not silently zero every token count (see
# internal/tokenizer/bpe_loader.go and failfast_test.go). CI provisions it in a
# workflow step before running the Go tests; build.sh does the same so the
# documented local test commands are self-contained. Order mirrors CI: system
# pre-seed first, then the network. A failure here is deliberately NOT fatal —
# the loader stays the final gate, and its panic message names the file.
CL100K_TABLE_URL="https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken"
CL100K_TABLE_RELPATH="deps/cl100k_base.tiktoken"
SYSTEM_DEPS_TOKENIZER="/opt/ragflow_deps"

# Provision the cl100k_base BPE table into deps/ (no-op if present).
# Returns 0 when the table is in place, 1 when it could not be provisioned.
_ensure_cl100k_table() {
    local dest="${PROJECT_ROOT}/${CL100K_TABLE_RELPATH}"
    local sys_copy="${SYSTEM_DEPS_TOKENIZER}/cl100k_base.tiktoken"

    echo "check if the cl100k_base BPE table exists at ${dest}"

    if [ -s "$dest" ]; then
        echo "  cl100k_base.tiktoken → ${dest} (already present)"
        return 0
    fi

    if [ -s "$sys_copy" ]; then
        mkdir -p "$(dirname "$dest")"
        cp "$sys_copy" "$dest"
        echo "  cl100k_base.tiktoken → ${dest} (system)"
        return 0
    fi

    # Download to a temp file and move it into place only after a successful
    # transfer: an interrupted download would otherwise leave a truncated table
    # behind, which the loader rejects on digest.
    local tmp="${dest}.download"
    mkdir -p "$(dirname "$dest")"
    if command -v curl >/dev/null 2>&1; then
        if curl -fsSL -o "$tmp" "$CL100K_TABLE_URL"; then
            mv "$tmp" "$dest"
            echo "  cl100k_base.tiktoken → ${dest} (downloaded)"
            return 0
        fi
    elif command -v wget >/dev/null 2>&1; then
        if wget -q -O "$tmp" "$CL100K_TABLE_URL"; then
            mv "$tmp" "$dest"
            echo "  cl100k_base.tiktoken → ${dest} (downloaded)"
            return 0
        fi
    fi
    rm -f "$tmp"

    echo -e "  ${YELLOW}cl100k_base BPE table NOT provisioned${NC} (offline, or the network is blocked)"
    echo "  internal/tokenizer panics on a missing table by design, so every test that"
    echo "  counts tokens will fail loudly. Recover with any of:"
    echo "    - re-run this command (a transient network error usually clears);"
    echo "    - curl -fsSL -o ${CL100K_TABLE_RELPATH} ${CL100K_TABLE_URL}"
    echo "    - or point TIKTOKEN_CACHE_DIR at a directory holding the table."
    return 1
}

echo -e "${GREEN}=== RAGFlow Go Server Build Script ===${NC}"

# Function to print section headers
print_section() {
    echo -e "\n${YELLOW}>>> $1${NC}"
}

# Detect the package-install command for pcre2 development files.
# Outputs the command on stdout; empty string if no supported manager is found.
detect_pcre2_install_cmd() {
    if [ "$(uname)" = "Darwin" ]; then
        echo "brew install pcre2"
    elif command -v apt-get >/dev/null 2>&1; then
        echo "sudo apt-get install -y libpcre2-dev"
    elif command -v zypper >/dev/null 2>&1; then
        echo "sudo zypper install -y pcre2-devel"
    elif command -v dnf >/dev/null 2>&1; then
        echo "sudo dnf install -y pcre2-devel"
    elif command -v pacman >/dev/null 2>&1; then
        echo "sudo pacman -S --noconfirm pcre2"
    else
        echo ""
    fi
}

# Check whether libpcre2-8 is available (static or shared).
check_pcre2() {
    # Prefer pkg-config when available — works across distros.
    if command -v pkg-config >/dev/null 2>&1 && pkg-config --exists libpcre2-8; then
        return 0
    fi
    # Fall back to known library paths:
    #   Debian/Ubuntu  -> /usr/lib/x86_64-linux-gnu
    #   openSUSE/RHEL  -> /usr/lib64
    #   generic Linux  -> /usr/lib, /usr/local/lib
    #   macOS Homebrew -> /opt/homebrew/lib (Apple Silicon), /usr/local/lib (Intel)
    for p in \
        /usr/lib/x86_64-linux-gnu/libpcre2-8.a \
        /usr/lib/x86_64-linux-gnu/libpcre2-8.so \
        /usr/lib64/libpcre2-8.a \
        /usr/lib64/libpcre2-8.so \
        /usr/lib/libpcre2-8.a \
        /usr/lib/libpcre2-8.so \
        /usr/local/lib/libpcre2-8.a \
        /usr/local/lib/libpcre2-8.so \
        /usr/local/lib/libpcre2-8.dylib \
        /opt/homebrew/lib/libpcre2-8.a \
        /opt/homebrew/lib/libpcre2-8.dylib; do
        [ -f "$p" ] && return 0
    done
    return 1
}

# Check dependencies
check_cpp_deps() {
    print_section "Checking c++ dependencies"

    command -v cmake >/dev/null 2>&1 || { echo -e "${RED}Error: cmake is required but not installed.${NC}"; exit 1; }
    command -v clang++ >/dev/null 2>&1 || { echo -e "${RED}Error: clang++ is required but not installed.${NC}"; exit 1; }

    if check_pcre2; then
        echo "✓ pcre2 library found"
    else
        install_cmd="$(detect_pcre2_install_cmd)"
        echo -e "${YELLOW}Warning: libpcre2-8 not found. You may need to install it:${NC}"
        if [ -n "$install_cmd" ]; then
            echo "  $install_cmd"
        else
            echo "  (No supported package manager detected — install pcre2 development files manually)"
        fi
    fi

    echo "✓ Required tools are available"
}

check_go_deps() {
    print_section "Checking go dependencies"

    command -v go >/dev/null 2>&1 || { echo -e "${RED}Error: go is required but not installed.${NC}"; exit 1; }

    echo "✓ Required tools are available"
    check_ort_version_consistency
}

# Fail fast when the ONNX Runtime native version is declared inconsistently.
# The in-process (Go) DeepDoc backend statically links libonnxruntime.a built
# from ONE exact ORT release; a mismatch links a wrong/missing .a and only fails
# at runtime (dlopen(NULL) can't find OrtGetApiBase). The version is pinned in
# three Go-side locations that must all agree.
check_ort_version_consistency() {
    print_section "Checking ONNX Runtime version consistency"

    local env_go dockerfile
    env_go="$(deepdoc_ort_version "${PROJECT_ROOT}")"
    dockerfile="$(grep -m1 -E 'ARG[[:space:]]+ORT_VERSION=' "${PROJECT_ROOT}/Dockerfile" | sed -E 's/.*ORT_VERSION=([0-9][^"[:space:]]*).*/\1/')"

    if [ -z "$env_go" ] || [ -z "$dockerfile" ]; then
        echo -e "${RED}Error: could not parse the ONNX Runtime version from one of the pinned locations${NC}" >&2
        exit 1
    fi

    if [ "$env_go" != "$dockerfile" ]; then
        echo -e "${RED}Error: ONNX Runtime version is inconsistent — fix before building:${NC}" >&2
        printf '  %-10s  %s\n' "$env_go" "internal/common/environments.go:DeepDocORTVersion"
        printf '  %-10s  %s\n' "$dockerfile" "dockerfile:ARG ORT_VERSION"
        exit 1
    fi

    echo -e "${GREEN}✓ ONNX Runtime native version consistent: ${env_go}${NC}"
}

# Check the ONNX Runtime static archive the Go DeepDoc backend links.
#
# This mirrors check_office_oxide_deps: it verifies the on-disk archive
# actually matches the released artifact, so a stale/incompatible .a — one
# re-issued under the SAME release tag and asset name, or an older download —
# fails fast HERE with an actionable message, instead of producing a binary
# that links fine but crashes at runtime when the shared-initializer feature
# (SessionGetInitializer*, added by infiniflow/ragflow-build) runs.
#
# The onnxruntime_go binding reaches ORT purely through the OrtApi
# function-pointer table (onnxruntime_go/shared_initializer.go ->
# onnxruntime_wrapper.c -> ort_api->SessionGetInitializer*), so a stale .a
# that lacks those custom slots links successfully and only fails at runtime.
# The check below catches that by comparing the local release ZIP against the
# published {asset}.sha256 sidecar (same source of truth the download scripts
# use). Devs who ran a download script have the zip under deps/; on CI the
# version-keyed cache is mounted from the build host and no zip is present, so
# the sha256 check is skipped there and the mounted cache seed is authoritative.
check_onnxruntime_deps() {
    # Ensure TARGET_OS/TARGET_ARCH are resolved. ort_static_lib_prefix below
    # depends on them; without this guard an early call (before any other path
    # triggers detect_target_platform) would resolve the archive glob against an
    # empty prefix and could pick up an ORT built for the wrong platform from a
    # shared cache. Idempotent: derive once, then reuse the result.
    if [ -z "$_TARGET_PLATFORM_DETECTED" ]; then
        detect_target_platform
        _TARGET_PLATFORM_DETECTED=1
    fi

    local ort_version
    ort_version="$(deepdoc_ort_version "${PROJECT_ROOT}")"
    if [ -z "$ort_version" ]; then
        echo -e "${RED}Error: cannot parse DeepDocORTVersion from internal/common/environments.go${NC}" >&2
        return 1
    fi

    # Locate the ORT static archives under the version dir. Populate the
    # globals ORT_A_FILES (space-separated .a paths) and ORT_A_SHA256 (sha256
    # of the primary libonnxruntime.a) for the caller's version-stamped
    # cache path.
    ORT_A_FILES=""
    local primary=""
    local f
    # Platform-specific top-level dir prefix (see ort_static_lib_prefix)
    # plus the required version — a shared cache can hold ORT built for
    # another OS/arch, and the Python side legitimately keeps another ORT
    # version in the same prefix. Matching both is why coexisting version
    # dirs need no separate fail-fast guard here.
    # The ORT static archives are resolved from the version-keyed user cache
    # (~/ragflow-native-libs/onnxruntime/static_lib, which the downloader
    # populates and CI mounts in). There is no /opt pre-bake to reseed from; an
    # incomplete cache is filled by the controlled auto-download in setup_cgo_env.

    local ort_dir_prefix
    ort_dir_prefix="$(ort_static_lib_prefix)"
    while IFS= read -r f; do
        case "$(basename "$f")" in
            *cuda*|*tensorrt*|*coreml*|*dml*|*migraphx*) continue ;;
        esac
        case "$f" in
            # Only collect .a from the dir matching both the target
            # platform and the required version: ${ort_dir_prefix} pins
            # OS/arch (a shared cache can also hold ORT built for another
            # platform), ${ort_version} pins DeepDocORTVersion (the Python
            # side legitimately keeps another ORT version in the same
            # prefix). Matching both is why coexisting version dirs no
            # longer need a fail-fast guard here.
            */${ort_dir_prefix}${ort_version}*/lib/*.a)
                ORT_A_FILES="$ORT_A_FILES $f"
                [ -z "$primary" ] && primary="$f"
                case "$(basename "$f")" in
                    libonnxruntime.a) primary="$f" ;;
                esac
                ;;
        esac
    done < <(find "$ONNXRUNTIME_STATIC_PREFIX" -type f -name '*.a' 2>/dev/null)

    if [ -z "$ORT_A_FILES" ]; then
        local avail
        avail="$(find "$ONNXRUNTIME_STATIC_PREFIX" -maxdepth 1 -type d \
            -name "${ort_dir_prefix}*" -exec basename {} \; 2>/dev/null | tr '\n' ' ')"
        echo -e "${RED}Error: no ONNX Runtime ${ort_version} static lib under $ONNXRUNTIME_STATIC_PREFIX${NC}" >&2
        echo "  available: ${avail:-<none>}" >&2
        echo "  DeepDocORTVersion=${ort_version}; fetch it with:" >&2
        echo "    uv run tools/scripts/download_dev_deps.py" >&2
        echo "  or pre-seed ${NATIVE_LIBS_DIR}/onnxruntime (CI mounts the cache from the build host)." >&2
        return 1
    fi

    # Pinned sha256 of the published onnxruntime archive (currently the
    # onnxruntime-v1.29.0-patch2 re-issue, which re-adds Clip@13 to the minimal
    # operator set — see tools/scripts/download_dev_deps.py ORT_RELEASE_TAG).
    #
    # Upstream does not re-issue this archive, so we pin the digest here and
    # verify the LOCAL copy against it — no network access at build time. This
    # replaces the previous check that fetched the .sha256 sidecar over the
    # network on every build, which hung the build when the machine was offline
    # or GitHub was unreachable (curl had no connect timeout).
    #
    # A mismatch means the local copy is corrupt or stale; we only WARN (not
    # fail) so a one-off re-issue cannot break the build. The downloader
    # tools/scripts/download_dev_deps.py still verifies against
    # the published sidecar at download time, so a re-issued archive is picked
    # up on the next download.
    #
    # The digest is per release target (see ort_release_target): comparing a
    # macOS zip against the Linux digest would warn on every build, so an
    # unknown target verifies nothing instead.
    local asset="onnxruntime-v${ort_version}-$(ort_release_target).zip"
    local zip_path="${PROJECT_ROOT}/deps/${asset}"
    local expected=""
    case "$asset" in
        onnxruntime-v*-linux-x86_64.zip)
            expected="1d41cc9c2a174200e90fd622e8b0f95e6531426bc9c23cfb5a4b92001c274da1" ;;
        onnxruntime-v*-linux-aarch64.zip)
            expected="712b65d37d7cf4bf1d41aff0763a084020c0e36a09073170bbaf61c018246a83" ;;
        onnxruntime-v*-osx-arm64.zip)
            expected="f0bebbb13901196757820164fe4ae936149e42021ff40c6450822bd6cd6e958a" ;;
        onnxruntime-v*-osx-universal.zip)
            expected="8c1bfab88e17de325376e7e5c2cbe163e5df48af362083912fdbac042f9c80da" ;;
    esac
    if [ -z "$expected" ]; then
        echo "  (no pinned digest for ${asset}; skipping sha256 check)"
    elif [ -f "$zip_path" ]; then
        local actual
        actual="$(sha256sum "$zip_path" | awk '{print $1}')"
        if [ "$actual" != "$expected" ]; then
            echo -e "${YELLOW}Warning: ONNX Runtime archive ${asset} sha256 mismatch${NC}" >&2
            echo "  expected sha256: $expected" >&2
            echo "  local    sha256: $actual" >&2
            echo "  The local copy may be corrupt or stale. Refresh it with:" >&2
            echo "    rm -f ${zip_path}" >&2
            echo "    uv run tools/scripts/download_dev_deps.py" >&2
        else
            echo "  ✓ ${asset} sha256 matches pinned digest"
        fi
    else
        echo "  (no local ${asset}; skipping sha256 check — CI seed assumed authoritative)"
    fi

    # Content hash of the primary archive for the version-stamped cache path.
    ORT_A_SHA256="$(sha256sum "$primary" | awk '{print $1}')"
    echo "✓ onnxruntime v${ort_version} static lib verified under $ONNXRUNTIME_STATIC_PREFIX"
}

# Check office_oxide native library
check_office_oxide_deps() {
    print_section "Checking office_oxide native library"
    # office_oxide is resolved from the version-keyed user cache
    # (${OFFICE_OXIDE_PREFIX}). An incomplete cache is filled by the controlled
    # auto-download in setup_cgo_env; there is no /opt pre-bake to recover from.

    local lib_file="liboffice_oxide.a"
    local lib_path="${OFFICE_OXIDE_PREFIX}/lib/${lib_file}"
    local header_path="${OFFICE_OXIDE_PREFIX}/include/office_oxide_c/office_oxide.h"

    if [ ! -f "$lib_path" ] || [ ! -f "$header_path" ]; then
        echo -e "${RED}Error: office_oxide native library not found${NC}"
        echo "  Expected: ${lib_path}"
        echo "  Run: uv run tools/scripts/download_dev_deps.py"
        echo "  Or manually download: https://github.com/yfedoseev/office_oxide/releases/download/v${OFFICE_OXIDE_VERSION}/$(office_oxide_asset)"
        exit 1
    fi

    # Verify the on-disk lib matches the pinned version. A stale older lib
    # (e.g. v0.1.7) silently reintroduces the PPT97 content-loss bug
    # (github.com/yfedoseev/office_oxide#85, fixed in v0.1.8): PlainText()
    # on a legacy .ppt returns stale metadata instead of slide text.
    if ! strings "$lib_path" 2>/dev/null | grep -Fxq "$OFFICE_OXIDE_VERSION"; then
        local found_version
        found_version=$(strings "$lib_path" 2>/dev/null | grep -E "^0\.[0-9]+\.[0-9]+$" | head -1)
        echo -e "${RED}Error: office_oxide native lib version mismatch${NC}"
        echo "  Required: v${OFFICE_OXIDE_VERSION}; found: ${found_version:-unknown}"
        echo "  A stale lib silently loses PPT97 (.ppt) slide content. Refresh:"
        echo "    uv run tools/scripts/download_dev_deps.py   # re-extracts into ${OFFICE_OXIDE_PREFIX}"
        exit 1
    fi

    echo "✓ office_oxide v${OFFICE_OXIDE_VERSION} native library found at ${OFFICE_OXIDE_PREFIX}"
    return 0
}

# Check pdfium static library.
check_pdfium_deps() {
    # pdfium is resolved from the version-keyed user cache
    # (${PDFIUM_STATIC_PREFIX}). An incomplete cache is filled by the controlled
    # auto-download in setup_cgo_env; there is no /opt pre-bake to recover from.
    local lib_path="${PDFIUM_STATIC_PREFIX}/lib/libpdfium.a"

    if [ -f "$lib_path" ]; then
        echo "  pdfium (static) → ${PDFIUM_STATIC_PREFIX}"
        return 0
    fi

    echo "  pdfium (static) not found"
    echo "  Expected: ${lib_path}"
    echo "  Run: uv run tools/scripts/download_dev_deps.py"
    echo "  Or: curl -fsSL https://github.com/kognitos/pdfium-static/releases/download/chromium%2F${PDFIUM_STATIC_VERSION}/$(pdfium_asset) | tar xz -C ${PDFIUM_STATIC_PREFIX}"
    return 1
}

# Check pdf_oxide static library.
# pdf_oxide_validate_version <pool_text> <required_version>
#
# Validates the pdf_oxide version embedded in a lib's string constant pool
# against the pinned <required_version>. The marker "pdf_oxide <version>" is
# merged into Rust's constant pool and has no fixed length, so a suffixed build
# (e.g. 0.3.73.1) and a clean 0.3.73 whose next pooled constant begins with a
# digit are byte-identical and cannot be told apart.
#
# Capture exactly three dotted segments and reject any trailing [0-9.] as
# ambiguous, rather than silently truncating it into a (wrong) match.
#
# Returns:
#   0  exact match (prints the found version)
#   1  version missing or mismatched (prints the found version, or empty)
#   2  ambiguous: the marker is followed by extra digits/dots in the pool
pdf_oxide_validate_version() {
    local pool_text="$1" required="$2"
    local found esc
    found=$(printf '%s\n' "$pool_text" \
        | grep -oE "pdf_oxide [0-9]+\.[0-9]+\.[0-9]+" | head -1 | cut -d' ' -f2)
    if [ -z "$found" ]; then
        return 1
    fi
    # A digit or dot immediately after the captured "pdf_oxide X.Y.Z" marker
    # means a suffixed build (0.3.73.1) or a constant concatenated on to the
    # version. Treat it as ambiguous rather than as a wrong version.
    # Anchor to the exact captured version (dots escaped) so the third segment
    # cannot backtrack and swallow the trailing digit, which would otherwise
    # flag a clean 0.3.73<letter> marker as ambiguous.
    esc="${found//./\\.}"
    if printf '%s\n' "$pool_text" | grep -qE "pdf_oxide ${esc}[0-9.]"; then
        return 2
    fi
    if [ "$found" != "$required" ]; then
        printf '%s\n' "$found"
        return 1
    fi
    printf '%s\n' "$found"
    return 0
}

check_pdf_oxide_deps() {
    # pdf_oxide is resolved from the version-keyed user cache
    # (${PDF_OXIDE_PREFIX}). An incomplete cache is filled by the controlled
    # auto-download in setup_cgo_env; there is no /opt pre-bake to recover from.
    # Map platform to tarball-internal subdirectory (lib/<os>_<arch>/). Reuses
    # TARGET_OS/TARGET_ARCH set by detect_target_platform instead of re-deriving
    # from uname, so it stays consistent with the other check_*_deps helpers.
    local platform_subdir="${TARGET_OS}_${TARGET_ARCH}"
    case "$platform_subdir" in
        linux_amd64|linux_arm64|darwin_amd64|darwin_arm64) ;;
        *) echo "  pdf_oxide (static) → unsupported platform: $platform_subdir"; return 1 ;;
    esac

    local lib_path="${PDF_OXIDE_PREFIX}/lib/${platform_subdir}/libpdf_oxide.a"

    if [ -f "$lib_path" ]; then
        # Verify the on-disk lib matches the pinned version. The cache is
        # version-keyed (${PDF_OXIDE_PREFIX}), so an earlier pin lives under a
        # different v<version>/ dir and is never silently reused; this check is a
        # defense-in-depth against a stale .a that somehow landed in this dir.
        #
        # The version marker "pdf_oxide <version>" is merged into Rust's string
        # constant pool (unlike office_oxide's standalone "0.1.9" line), so we
        # capture exactly three dotted segments and reject any trailing [0-9.]
        # as ambiguous — a suffixed build (0.3.73.1) or a constant that is
        # byte-identical to one must not be silently accepted. The "pdf_oxide "
        # prefix keeps bare version numbers of vendored dependencies out of the
        # match.
        local pool_text found_version rc
        pool_text=$(strings "$lib_path" 2>/dev/null)
        found_version=$(pdf_oxide_validate_version "$pool_text" "$PDF_OXIDE_VERSION"); rc=$?
        case "$rc" in
            0)
                echo "  pdf_oxide (static) → ${PDF_OXIDE_PREFIX}"
                return 0
                ;;
            2)
                echo -e "${RED}Error: pdf_oxide native lib version ambiguous${NC}"
                echo "  The version marker is followed by extra digits/dots in the"
                echo "  string pool; this is usually a suffixed build (e.g. ${PDF_OXIDE_VERSION}.1)"
                echo "  or a concatenated constant byte-identical to one. Refresh the lib"
                echo "  to a clean pin."
                echo "  Required: v${PDF_OXIDE_VERSION}"
                return 1
                ;;
            *)
                echo -e "${RED}Error: pdf_oxide native lib version mismatch${NC}"
                echo "  Required: v${PDF_OXIDE_VERSION}; found: ${found_version:-unknown}"
                echo "  A stale lib silently reverts PDF parsing fixes. Refresh:"
                echo "    rm -rf ${PDF_OXIDE_PREFIX} deps/$(pdf_oxide_asset)"
                echo "    uv run tools/scripts/download_dev_deps.py"
                return 1
                ;;
        esac
    fi

    echo "  pdf_oxide (static) not found"
    echo "  Expected: ${lib_path}"
    echo "  Run: uv run tools/scripts/download_dev_deps.py"
    echo "  Or: curl -fsSL https://github.com/yfedoseev/pdf_oxide/releases/download/v${PDF_OXIDE_VERSION}/$(pdf_oxide_asset) | tar xz -C ${PDF_OXIDE_PREFIX}"
    return 1
}

# Go 1.27+ scans every linker input named in #cgo LDFLAGS and rejects an ELF
# archive whose members carry LLVM's SHT_LLVM_LINKER_OPTIONS section
# (".linker-options", type 0x6fff4c01):
#   "disallowed ELF embedded linker options section in #cgo LDFLAGS: ..."
# clang-20 emits that section into every object of librag_tokenizer_c_api.a
# (along with .llvm_addrsig, which Go permits), so strip it before any Go
# build/test links the archive. Removing the section drops no code or symbols;
# the dependencies it advertises are already listed explicitly in #cgo LDFLAGS
# (-lstdc++ -lm -lpthread -lpcre2-8).
# Idempotent: a no-op when the section is absent (gcc-built archives, or an
# archive already stripped). Linux/ELF only; macOS objects are Mach-O.
strip_tokenizer_linker_options() {
    [ "$(uname -s)" = "Linux" ] || return 0
    local tok_a="$BUILD_DIR/librag_tokenizer_c_api.a"
    [ -f "$tok_a" ] || return 0

    # Detect before writing so a clean archive stays byte-for-byte untouched,
    # and a missing objcopy is only fatal when the section is actually present.
    # Without readelf, fall through to the strip: objcopy treats a section that
    # is not there as a no-op, so a gcc-built archive stays valid.
    if command -v readelf >/dev/null 2>&1; then
        if ! readelf -S "$tok_a" 2>/dev/null | grep -q '\.linker-options'; then
            return 0
        fi
    fi

    local objcopy_bin=""
    if command -v objcopy >/dev/null 2>&1; then
        objcopy_bin=objcopy
    elif command -v llvm-objcopy >/dev/null 2>&1; then
        objcopy_bin=llvm-objcopy
    else
        echo -e "${RED}Error: librag_tokenizer_c_api.a contains .linker-options sections that Go 1.27+ rejects, but neither objcopy nor llvm-objcopy is installed.${NC}" >&2
        return 1
    fi
    "$objcopy_bin" --remove-section=.linker-options "$tok_a"
    echo -e "${GREEN}✓ Removed LLVM .linker-options sections from librag_tokenizer_c_api.a${NC}"
}

# Build C++ static library
build_cpp() {
    print_section "Building C++ static library"

    mkdir -p "$BUILD_DIR"
    cd "$BUILD_DIR"

    echo "Running cmake..."
    cmake .. -DCMAKE_BUILD_TYPE=Release

    echo "Building librag_tokenizer_c_api.a..."
    local jobs
    jobs="$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 1)"
    make rag_tokenizer_c_api -j"$jobs"

    if [ ! -f "$BUILD_DIR/librag_tokenizer_c_api.a" ]; then
        echo -e "${RED}Error: Failed to build C++ static library${NC}"
        exit 1
    fi

    strip_tokenizer_linker_options

    # Rename the tokenizer's bundled re2 symbols into a private namespace so they
    # never collide with onnxruntime's copy of re2 at final link time.
    #
    # Why this is needed: onnxruntime.a and librag_tokenizer_c_api.a both embed a
    # copy of re2, but they are built against different toolchains/libstdc++ and
    # are ABI-incompatible. Symbol-localization approaches (--exclude-libs, linker
    # version scripts) cannot fix it: both re2 copies must live in the same global
    # symbol table, and the linker resolves the tokenizer's re2 references to
    # whichever copy was archived first (here, onnxruntime's), regardless of which
    # copy is localized. SIGSEGV in re2::DFA::InlinedSearchLoop is the symptom.
    #
    # Renaming the tokenizer's re2 symbols into a private prefix gives each copy
    # its own namespace, so the tokenizer calls its own ABI-compatible re2 and
    # onnxruntime keeps calling its own. Verified to eliminate the crash.
    local tok_a="$BUILD_DIR/librag_tokenizer_c_api.a"
    local rename_map="$BUILD_DIR/re2_rename.map"
    # GNU objcopy is standard on Linux; on macOS it is provided by LLVM as
    # llvm-objcopy (brew install llvm). Prefer whichever is available.
    local objcopy_bin=""
    if command -v objcopy >/dev/null 2>&1; then
        objcopy_bin=objcopy
    elif command -v llvm-objcopy >/dev/null 2>&1; then
        objcopy_bin=llvm-objcopy
    fi
    if [ -n "$objcopy_bin" ]; then
        nm "$tok_a" \
            | awk '$2 ~ /^[TDBRWtdbrwiIVv]$/ && $3 ~ /^_ZN3re2|_ZNK3re2|_ZTVN3re2|_ZTIN3re2|_ZTSN3re2/ { print $3" ragtokre2_"$3 }' \
            | sort -u > "$rename_map"
        if [ -s "$rename_map" ]; then
            "$objcopy_bin" --redefine-syms="$rename_map" "$tok_a"
            echo -e "${GREEN}✓ Renamed $(wc -l < "$rename_map") tokenizer re2 symbols into private namespace (ragtokre2_)${NC}"
        fi
    else
        echo -e "${YELLOW}Warning: objcopy/llvm-objcopy not found, skipping re2 symbol rename (re2 collision with onnxruntime may cause SIGSEGV)${NC}"
    fi

    echo -e "${GREEN}✓ C++ static library built successfully${NC}"
}

# Build C++ test executable
build_cpp_test() {
    print_section "Building C++ test executable"

    if [ ! -d "$BUILD_DIR" ]; then
        echo "Build directory not found, running cmake first..."
        mkdir -p "$BUILD_DIR"
        cd "$BUILD_DIR"
        cmake .. -DCMAKE_BUILD_TYPE=Release
    else
        cd "$BUILD_DIR"
    fi

    echo "Building rag_analyzer_c_test..."
    local jobs
    jobs="$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 1)"
    make rag_analyzer_c_test -j"$jobs"

    if [ ! -f "$BUILD_DIR/rag_analyzer_c_test" ]; then
        echo -e "${RED}Error: Failed to build rag_analyzer_c_test${NC}"
        exit 1
    fi

    echo -e "${GREEN}✓ C++ test executable built successfully: $BUILD_DIR/rag_analyzer_c_test${NC}"
}

# Build Go server
build_go() {
    print_section "Building RAGFlow go"

    cd "$PROJECT_ROOT"

    # Check if C++ library exists
    if [ ! -f "$BUILD_DIR/librag_tokenizer_c_api.a" ]; then
        echo -e "${RED}Error: C++ static library not found. Run with --cpp first.${NC}"
        exit 1
    fi

    if check_pcre2; then
        echo "✓ pcre2 library found"
    else
        install_cmd="$(detect_pcre2_install_cmd)"
        if [ -z "$install_cmd" ]; then
            echo -e "${RED}Error: libpcre2-8 not found and no supported package manager detected.${NC}"
            echo "Please install pcre2 development files manually."
            exit 1
        fi
        if [ "$(uname)" = "Darwin" ]; then
            echo -e "${RED}Error: libpcre2-8 not found. Install with: $install_cmd${NC}"
            exit 1
        fi
        echo -e "${YELLOW}Warning: libpcre2-8 not found. Installing with: $install_cmd${NC}"
        eval "$install_cmd"
    fi

    setup_cgo_env

    # The in-process (Go) DeepDoc backend is statically linked against ONNX
    # Runtime (--undefined=OrtGetApiBase + -Wl,--dynamic-list, see
    # setup_cgo_env). The forked onnxruntime_go binding resolves OrtGetApiBase
    # only at RUNTIME via dlopen(NULL), so a missing ORT archive still lets
    # `go build` SUCCEED and yields a server binary that FAILS AT STARTUP with a
    # fatal "no in-process DeepDoc backend serving". That is exactly the
    # breakage colleagues hit when ORT was not present at build time and
    # setup_cgo_env silently skipped it. Fail the build HERE instead of
    # deferring the breakage to runtime: a server binary without ORT is unusable,
    # not a degraded one.
    if ! printf '%s' "$CGO_LDFLAGS" | grep -q 'libonnxruntime'; then
        echo -e "${RED}Error: ONNX Runtime static libraries are not linked.${NC}" >&2
        echo "  The in-process DeepDoc backend requires libonnxruntime.a to be" >&2
        echo "  statically linked into ragflow_server (--undefined=OrtGetApiBase +" >&2
        echo "  -Wl,--dynamic-list). Without it the binary compiles but dies" >&2
        echo "  at startup with a fatal 'no in-process DeepDoc backend serving'." >&2
        echo "  Fetch the static libs with:" >&2
        echo "    uv run tools/scripts/download_dev_deps.py" >&2
        echo "  or pre-seed them at ${NATIVE_LIBS_DIR}/onnxruntime (CI mounts the cache from the build host)." >&2
        echo "  This production binary must statically include ORT — there is no" >&2
        echo "  ORT-free build path. ORT ends up unlinked only via one of:" >&2
        echo "    - the ORT static_lib dir was never seeded: run" >&2
        echo "      'uv run tools/scripts/download_dev_deps.py', or pre-seed" >&2
        echo "      ${NATIVE_LIBS_DIR}/onnxruntime as the CI mount does;" >&2
        echo "    - setup_cgo_env did not add libonnxruntime to CGO_LDFLAGS." >&2
        return 1
    fi

    local strip_flags=()
    [ -n "$STRIP_SYMBOLS" ] && strip_flags=(-ldflags="-s -w")

    echo "Building RAGFlow binary: $RAGFLOW_CLI_BINARY, $RAGFLOW_SERVER_BINARY"
    set -x
    GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} CGO_ENABLED=1 \
        go build -tags "$GO_BUILD_TAGS" "${strip_flags[@]}" -o "$RAGFLOW_CLI_BINARY" cmd/ragflow-cli.go

    GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} CGO_ENABLED=1 \
        CGO_CFLAGS="$CGO_CFLAGS" CGO_LDFLAGS="$CGO_LDFLAGS" \
        go build -tags "$GO_BUILD_TAGS" "${strip_flags[@]}" -o "$RAGFLOW_SERVER_BINARY" \
        ./cmd
    set +x


    if [ ! -f "$RAGFLOW_SERVER_BINARY" ]; then
        echo -e "${RED}Error: Failed to build RAGFlow main binary${NC}"
        exit 1
    fi

    echo -e "${GREEN}✓ Go ragflow-cli built successfully: $RAGFLOW_CLI_BINARY${NC}"
    echo -e "${GREEN}✓ Go ragflow_server built successfully: $RAGFLOW_SERVER_BINARY${NC}"
}

# Configure CGO flags for native libraries (office_oxide, pdfium, pdf_oxide).
# All three are statically linked — no LD_LIBRARY_PATH or -Wl,-rpath needed.
setup_cgo_env() {
    detect_target_platform

    # --test/--go reuse the existing librag_tokenizer_c_api.a without rebuilding
    # the C++ library. --cpp strips the clang-20 .linker-options section right
    # after make, but a stale archive from an earlier build would still trip
    # Go 1.27+'s cgo LDFLAGS check, so sanitize it on every path that links it.
    strip_tokenizer_linker_options

    # Controlled auto-download: fill any missing version-keyed native lib from
    # the user cache before resolving individual libs. On an already-provisioned
    # host (the common CI case, where the cache is mounted in) this is a no-op;
    # on a fresh host it runs the one-stop downloader once. There is no /opt
    # pre-bake and no flat fallback to recover from.
    _ensure_native_libs_installed

    # ── office_oxide ──────────────────────────────────────────────────
    check_office_oxide_deps

    # The office_oxide prefix is already version-keyed
    # (${OFFICE_OXIDE_PREFIX} = .../office_oxide/v<version>), so the linked path
    # carries the version and Go's build cache (which keys CGO_LDFLAGS as a
    # string, not the .a contents) automatically relinks when the version bumps.
    export CGO_CFLAGS="-I${OFFICE_OXIDE_PREFIX}/include/office_oxide_c${CGO_CFLAGS:+ $CGO_CFLAGS}"
    export CGO_LDFLAGS="${OFFICE_OXIDE_PREFIX}/lib/liboffice_oxide.a"

    # ── pdfium ────────────────────────────────────────────────────────
    check_pdfium_deps || return 1
    export CGO_LDFLAGS="$CGO_LDFLAGS ${PDFIUM_STATIC_PREFIX}/lib/libpdfium.a"
    # Linux: Chromium-built objects use Clang's .eh_frame format which GNU ld
    # cannot merge. Use lld (LLVM linker) which handles them correctly.
    # --allow-multiple-definition: pdf_oxide and office_oxide are both Rust
    # staticlibs that embed the Rust runtime; linking them together produces
    # duplicate rust_eh_personality / compiler-rt builtins. Those duplicates are
    # ABI-IDENTICAL, so merging them is benign.
    if [ "$(uname -s)" = "Linux" ]; then
        if ! command -v ld.lld >/dev/null 2>&1; then
            echo -e "${RED}Error: ld.lld not found. Install with: sudo apt install lld-20 && sudo ln -s /usr/bin/ld.lld-20 /usr/bin/ld.lld${NC}"
            echo "  lld is required to static-link Chromium-built pdfium (.eh_frame format)"
            return 1
        fi
        export CGO_LDFLAGS="$CGO_LDFLAGS \
            ${PDFIUM_STATIC_PREFIX}/lib/libc++.a \
            ${PDFIUM_STATIC_PREFIX}/lib/libc++abi.a \
            -fuse-ld=lld -Wl,--allow-multiple-definition"
        # The re2 regex-library collision between onnxruntime.a and
        # librag_tokenizer_c_api.a is fixed at the .a level in build_cpp():
        # the tokenizer's bundled re2 symbols are renamed into a private
        # namespace (ragtokre2_) so the two re2 copies never share a symbol.
        # See build_cpp() for details.
    fi

    # ── pdf_oxide ─────────────────────────────────────────────────────
    check_pdf_oxide_deps || return 1
    # The go-ffi tarball places the .a under lib/<platform_subdir>/. Reuses
    # TARGET_OS/TARGET_ARCH set by detect_target_platform (instead of
    # re-deriving from uname) so RAGFLOW_TARGET_OS/ARCH overrides apply here
    # exactly as they do in check_pdf_oxide_deps.
    local pdf_oxide_subdir="${TARGET_OS}_${TARGET_ARCH}"
    case "$pdf_oxide_subdir" in
        linux_amd64|linux_arm64|darwin_amd64|darwin_arm64) ;;
        *) echo "pdf_oxide: unsupported platform: $pdf_oxide_subdir"; return 1 ;;
    esac
    # The pdf_oxide prefix is already version-keyed
    # (${PDF_OXIDE_PREFIX} = .../pdf_oxide/v<version>), so the linked path
    # carries the version and Go's build cache relinks on a version bump.
    local pdf_oxide_lib_dir="${PDF_OXIDE_PREFIX}/lib/${pdf_oxide_subdir}"
    export CGO_LDFLAGS="$CGO_LDFLAGS ${pdf_oxide_lib_dir}/libpdf_oxide.a"

    # ── onnxruntime (static, resolved via dlopen(NULL)) ────────────────
    # Statically link libonnxruntime*.a into the binary. The org Go binding
    # (onnxruntime_go, github.com/infiniflow/onnxruntime_go) resolves OrtGetApiBase
    # with dlopen(NULL) + dlsym(handle, "OrtGetApiBase"), so the ONLY symbol that
    # must be visible in the process-global table is OrtGetApiBase. No
    # libonnxruntime.so/.dylib is required or supported at runtime; there is no
    # dynamic fallback.
    #
    # The two linkers need different flags to get there (see the platform
    # branch further down where CGO_LDFLAGS is assembled):
    #   Linux (GNU ld): force-pull just the archive member defining OrtGetApiBase
    #     and export exactly that symbol — no --whole-archive, so archive-level GC
    #     drops every ORT kernel/EP object nothing references (that flag is why
    #     every ORT size-trimming build flag had near-zero effect before). The
    #     binding reaches ORT purely through the OrtApi function-pointer table
    #     returned by OrtGetApiBase, and a minimal/reduced-ops build registers
    #     only the operators the deepdoc models actually use, so the reachable
    #     closure is small. --dynamic-list exports just OrtGetApiBase (the only
    #     symbol dlopen needs) and leaves Go's runtime symbols untouched, unlike
    #     a "local: *" version script which breaks PIE absolute relocations.
    #   macOS (ld64): ld64 uses -u _OrtGetApiBase to pull the OrtGetApiBase
    #     symbol from the static archive without -force_load (which would pull
    #     all unused objects and missing transitive dependencies), and exports
    #     it via -export_dynamic so runtime dlopen(NULL) resolves it.
    #
    # The static ORT archives come from the version-keyed user cache (populated
    # by download_dev_deps.py and mounted in on CI) — there is no /opt pre-bake.
    # An incomplete cache is filled by the controlled auto-download at the top
    # of setup_cgo_env.
    check_onnxruntime_deps || return 1

    # Version-stamp the archive paths so an in-place .a upgrade invalidates
    # Go's build cache. Go's cache keys CGO_LDFLAGS as a string and does NOT
    # hash the referenced .a contents, so swapping the .a in place (same path,
    # e.g. a re-issue under the same tag/asset name) silently reuses a stale
    # linked binary. Stamp with the primary archive's sha256 so the flag string
    # changes when the content changes → automatic relink.
    local ort_version
    ort_version="$(deepdoc_ort_version "${PROJECT_ROOT}")"
    local ort_versioned_dir="${ONNXRUNTIME_STATIC_PREFIX}/v${ort_version}-${ORT_A_SHA256:0:16}"
    local ort_a_versioned=""
    local f stamp needs_write want_target
    for f in $ORT_A_FILES; do
        # Relative, read-only-safe stamp (Fix #1/#2): the symlink target is
        # relative to the versioned dir so it resolves under any cache mount, and
        # the write is skipped when an existing link already points at the same
        # archive (a read-only CI mount is then a no-op).
        stamp="$(ragflow_ort_stamp_target "$ort_versioned_dir" "$f")"
        needs_write="${stamp%%|*}"
        want_target="${stamp#*|}"
        if [ "$needs_write" = "yes" ]; then
            mkdir -p "$ort_versioned_dir"
            ln -sf "$want_target" "$ort_versioned_dir/$(basename "$f")"
        fi
        ort_a_versioned="$ort_a_versioned $ort_versioned_dir/$(basename "$f")"
    done

    if [ "$TARGET_OS" = "darwin" ]; then
        # ld64 uses -u _OrtGetApiBase (with leading underscore) to force-pull
        # OrtGetApiBase without -force_load, avoiding dragging in unneeded
        # objects from libonnxruntime.a. Export dynamic symbols so the runtime
        # dlopen(NULL) resolves OrtGetApiBase.
        export CGO_LDFLAGS="$CGO_LDFLAGS -Wl,-export_dynamic -Wl,-u,_OrtGetApiBase $ort_a_versioned -lc++"
    else
        # Export exactly one symbol (OrtGetApiBase) for the binding's
        # dlopen(NULL)+dlsym lookup via --dynamic-list (NOT --version-script
        # with "local: *", which hides Go's runtime type symbols and breaks
        # the PIE absolute relocations). No --whole-archive, so GNU ld's
        # archive-level GC drops any ORT kernel/EP object nothing references.
        #
        # The dynamic list is written to a STABLE, project-scoped path (not
        # mktemp) so CGO_LDFLAGS is reproducible across builds and survives
        # across invocations; its content never changes, so overwriting is
        # safe and nothing leaks in /tmp. .cache/ is gitignored.
        local ort_dynamic_list="${PROJECT_ROOT}/.cache/ort_dynamic_list.txt"
        mkdir -p "$(dirname "$ort_dynamic_list")"
        printf '{\n  OrtGetApiBase;\n};\n' > "$ort_dynamic_list"
        # --undefined=OrtGetApiBase force-pulls the archive member that
        # defines OrtGetApiBase (the Go binding reaches ORT only via
        # dlsym("OrtGetApiBase"), so nothing references it at link time and
        # it would otherwise be GC'd). From there the minimal build's CPU-EP
        # registration call chain pulls in the operators the models use.
        # The explicit space between $ort_dynamic_list and the versioned .a
        # list keeps the two as separate linker arguments regardless of the
        # list's leading space.
        export CGO_LDFLAGS="$CGO_LDFLAGS -Wl,--undefined=OrtGetApiBase -Wl,--dynamic-list=$ort_dynamic_list $ort_a_versioned -lstdc++"
    fi
    echo "  onnxruntime (static) → $ONNXRUNTIME_STATIC_PREFIX"
    # The re2 regex-library collision between onnxruntime.a and
    # librag_tokenizer_c_api.a is fixed at the .a level in build_cpp():
    # the tokenizer's bundled re2 symbols are renamed into a private
    # namespace (ragtokre2_) so the two re2 copies never share a symbol
    # name. OrtGetApiBase is exported into the process dynamic symbol
    # table on Linux via --dynamic-list (above) — or, on macOS, via
    # -export_dynamic — which is what the binding's dlopen(NULL)+dlsym
    # lookup needs at runtime.

    # ── platform-specific system libraries ────────────────────────────
    case "$(uname -s)" in
        Linux)
            export CGO_LDFLAGS="$CGO_LDFLAGS -lm -lpthread -ldl -lrt -lgcc_s -lutil -lc"
            ;;
        Darwin)
            export CGO_LDFLAGS="$CGO_LDFLAGS \
                -framework CoreGraphics -framework CoreFoundation \
                -framework Security -framework SystemConfiguration \
                -framework Foundation \
                -liconv -lresolv -lc++"
            ;;
    esac

    echo "CGO_CFLAGS:   $CGO_CFLAGS"
    echo "CGO_LDFLAGS:  $CGO_LDFLAGS"
}

# Run Go unit tests with the same CGO env as `build_go`. Any extra args are
# forwarded to `go test`, e.g. `./build.sh --test -run TestFoo ./internal/admin/...`.
run_go_tests() {
    print_section "Running Go tests"

    # The tokenizer panics instead of counting 0 when the cl100k table is
    # missing, so provision it before the tests, as CI does. Non-fatal: without
    # it only the token-counting tests fail, and they say why.
    _ensure_cl100k_table || true

    cd "$PROJECT_ROOT"
    setup_cgo_env

    if [ "$#" -eq 0 ]; then
        set -- ./...
    fi
    GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} CGO_ENABLED=1 \
        CGO_CFLAGS="$CGO_CFLAGS" CGO_LDFLAGS="$CGO_LDFLAGS" \
        go test -tags "cgo,static${EXTRA_TAGS_COMMA}" -count=1 "$@"

    run_native_tests
}

# Run the unit tests of the native package (DeepDoc det/DLA/TSR/OCR-rec Go
# ports), now a regular package under the MAIN module gated by the `cgo` build
# tag (same isolation as office_oxide/pdfium). It used to be a nested Go module
# (own go.mod) that the root `./...` never descended into; after the merge it is
# covered by the normal module tests. The build is pure-Go geometry plus the
# cgo onnxruntime binding, so it runs whenever CGO_ENABLED=1 (the same gate the
# rest of the native C libs use). Model-backed integration tests are handled by
# run_native_integration_tests.
run_native_tests() {
    print_section "Running native unit tests"
    ( cd "$PROJECT_ROOT" && \
      GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} \
      CGO_ENABLED=1 \
      go test -tags "cgo,static${EXTRA_TAGS_COMMA}" -count=1 ./internal/deepdoc/native/... )
}

# Run the model-backed integration tests of the native package and the
# native_analyzer package (the DocAnalyzer the PDF parser consumes). These
# require MODEL_DIR at runtime; ONNX Runtime is statically linked and resolved
# via dlopen(NULL), so no ORT_LIB is needed. The tests self-skip when MODEL_DIR
# is unset or the binary lacks static ORT (see native_integration_test.go
# skipIfNoModels / native_analyzer_test.go analyzerWithModels). Run under the
# `cgo integration` tier.
#
# Why -race is split instead of applied to the whole suite:
# - The DLA/TSR/OCR-rec/Det golden-comparison tests are single-threaded and
#   deterministic. -race multiplies their (model-heavy) heap ~10x for ZERO
#   race-detection value, and that is what used to OOM-kill the runner
#   (SIGTERM) under `cgo integration`. So they run WITHOUT -race.
# - Only the concurrency-correctness tests (TestInferenceConcurrency* in the
#   native package, and native_analyzer's TestAnalyzerConcurrentBatchAndSingle)
#   actually benefit from -race: they hammer the shared session pools from many
#   goroutines, and the detector catches data races on the pools / per-session
#   in-out tensors / cross-call batch state. Those run WITH -race.
# ORT's C internals are not instrumented, but all our shared mutable state lives
# in Go, which the detector covers. CGO_ENABLED=1 is required for both the build
# and the race runtime.
run_native_integration_tests() {
    # Optional isolation: NATIVE_TEST_RUN forwards a -run filter and
    # NATIVE_TEST_V adds -v so a single native test can be exercised in
    # isolation (e.g. `NATIVE_TEST_RUN='TestNativeLoadsOrtModels$' NATIVE_TEST_V=1`).
    local native_run_filter=()
    if [ -n "${NATIVE_TEST_RUN:-}" ]; then
        native_run_filter+=(-run "${NATIVE_TEST_RUN}")
    fi
    if [ -n "${NATIVE_TEST_V:-}" ]; then
        native_run_filter+=(-v)
    fi
    print_section "Running native integration tests (golden/comparison, no race)"
    ( cd "$PROJECT_ROOT" && \
      GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} \
      CGO_ENABLED=1 \
      go test -tags "cgo static integration fetch_testdata${EXTRA_TAGS_SPACE}" -count=1 "${native_run_filter[@]}" ./internal/deepdoc/native/... )

    print_section "Running native integration concurrency tests (race detector on)"
    ( cd "$PROJECT_ROOT" && \
      GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} \
      CGO_ENABLED=1 \
      go test -tags "cgo static integration fetch_testdata${EXTRA_TAGS_SPACE}" -race -count=1 \
      -run 'TestInferenceConcurrency' ./internal/deepdoc/native/... )

    print_section "Running native_analyzer race tests (race detector on)"
    ( cd "$PROJECT_ROOT" && \
      GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} \
      CGO_ENABLED=1 \
      go test -tags "cgo static integration fetch_testdata${EXTRA_TAGS_SPACE}" -race -count=1 \
      ./internal/deepdoc/parser/pdf/inference/native_analyzer/... )

    # EE build extensions hook (build_ee.sh, sourced near the top of this
    # script). Guarded so an empty/absent build_ee.sh is a safe no-op on the
    # OSS path; the EE build defines run_ee_native_integration_tests to run its
    # own native integration/parity tests at this point.
    if declare -F run_ee_native_integration_tests >/dev/null 2>&1; then
        run_ee_native_integration_tests
    fi
}

# Run Go tests gated behind a build tag (or space-separated tag list), e.g.
# `./build.sh --test-integration -run TestFoo ./internal/engine/...`.
# See "Go Test Tiers" in AGENTS.md for the tier definitions.
run_go_tests_tagged() {
    local tags="$1"; shift
    print_section "Running Go tests (tags: ${tags})"

    # Same cl100k table requirement as the unit tier (integration/e2e runs count
    # tokens too); non-fatal, see _ensure_cl100k_table.
    _ensure_cl100k_table || true

    cd "$PROJECT_ROOT"
    setup_cgo_env

    if [ "$#" -eq 0 ]; then
        set -- ./...
    fi
    GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} CGO_ENABLED=1 \
        CGO_CFLAGS="$CGO_CFLAGS" CGO_LDFLAGS="$CGO_LDFLAGS" \
        go test -tags "${tags} static${EXTRA_TAGS_SPACE}" -count=1 "$@"
}

# Clean build artifacts
clean() {
    print_section "Cleaning build artifacts"

    rm -rf "$BUILD_DIR"
    rm -f "$RAGFLOW_SERVER_BINARY"
    rm -f "$RAGFLOW_CLI_BINARY"

    echo -e "${GREEN}✓ Build artifacts cleaned${NC}"
}

# Run the server
run() {
    if [ ! -f "$RAGFLOW_SERVER_BINARY" ]; then
        echo -e "${RED}Error: $RAGFLOW_SERVER_BINARY not found. Build first with --all or --go${NC}"
        exit 1
    fi

    cd "$PROJECT_ROOT"

    # admin_server must be running before ragflow_server, otherwise ragflow_server's
    # heartbeats to admin will error out (see internal/development.md).
    print_section "Starting admin server (background)"
    "$RAGFLOW_SERVER_BINARY" --admin &
    ADMIN_PID=$!
    # One trap for both background services: a second `trap ... EXIT INT TERM`
    # would replace this one rather than add to it, leaving admin_server holding
    # port 9381 after the foreground server exits. INGESTOR_PID is cleared first
    # so a value inherited from the environment cannot be signalled during the
    # window before the ingestor starts.
    INGESTOR_PID=""
    trap 'kill "$ADMIN_PID" ${INGESTOR_PID:+"$INGESTOR_PID"} 2>/dev/null || true' EXIT INT TERM

    # Give admin_server a moment to bind its listening port (9381) before
    # ragflow_server starts sending heartbeats to it.
    sleep 1

    print_section "Starting ingestor (background)"
    "$RAGFLOW_SERVER_BINARY" --ingestor &
    INGESTOR_PID=$!
    sleep 1

    print_section "Starting RAGFlow server (foreground)"
    "$RAGFLOW_SERVER_BINARY" --api
}

# Show help
show_help() {
    # Quoted delimiter so backticks, `$var`, and `\$` in the help text are
    # printed literally instead of being interpreted as command substitution.
    cat << 'EOF'
Usage: $0 [OPTIONS]

Build script for RAGFlow Go server with C++ bindings.

OPTIONS:
    --all, -a       Build everything (C++ library + Go server) [default]
    --cpp, -c       Build only C++ static library
    --cpp-test      Build C++ test executable (builds the C++ library if needed)
    --go, -g        Build only Go server (requires C++ library to be built)
    --test, -t      Run Go unit tests (no build tag). Sets up the CGO env and
                    native static libs (office_oxide/pdfium/pdf_oxide) needed to
                    build (same contract as the Go tier table in AGENTS.md).
                    Extra args are forwarded to `go test`, e.g.
                    `$0 --test -run TestFoo ./internal/admin/...`
    --test-integration   Run Go tests tagged 'integration' (need real services,
                    e.g. MySQL/MinIO/ES/Infinity/LLM). e.g.
                    `$0 --test-integration ./internal/engine/...`
    --test-integration-go  Run ONLY the generic 'integration' tier (MySQL/MinIO/
                    Redis/NATS/Infinity/ES) without the model-backed native
                    DeepDoc tests. Use this for the CI integration job, which
                    brings up the service stack separately from the native
                    backend job. e.g. `$0 --test-integration-go ./internal/storage/...`
    --test-e2e           Run Go tests tagged 'e2e' (full-pipeline, heavy).
    --test-manual        Run Go tests tagged 'manual' (very slow; local opt-in
                    ONLY, never run in CI).
    --test-all           Run 'integration' + 'e2e' tests (excludes 'manual').
    --test-native        Run the in-process (Go) DeepDoc backend tests tagged
                    'cgo integration' (needs libonnxruntime + the
                    InfiniFlow/deepdoc model snapshot; self-skip otherwise).
                    e.g. `$0 --test-native`
    --clean, -C     Clean all build artifacts
    --check-ort-version  Verify the ONNX Runtime native version is declared
                    consistently across all sources (exit 1 on mismatch).
    --run, -r       Build and run the server
    --strip, -s     Strip debug symbols from Go binaries (-ldflags="-s -w")
                    (disabled by default, useful for smaller production binaries)
    --help, -h      Show this help message

EXAMPLES:
    $0              # Build everything
    $0 --cpp        # Build only C++ library
    $0 --go         # Build only Go server
    $0 --cpp-test   # Build C++ test executable
    $0 --test       # Run all Go tests (unit tier, no build tag)
    $0 --test -run TestFoo ./internal/admin/...      # Targeted Go tests
    $0 --test-integration ./internal/engine/...      # integration + native tiers
    $0 --test-integration-go ./internal/storage/...  # generic integration tier only
    $0 --test-e2e                                 # e2e tier
    $0 --test-manual                             # manual tier (very slow)
    $0 --test-all                                # integration + e2e (no manual)
    $0 --run        # Build and run
    $0 --clean      # Clean build artifacts

DEPENDENCIES:
    - cmake >= 4.0
    - go >= 1.26.4
    - clang++ with C++20 support
    - office_oxide native library (download with: uv run tools/scripts/download_dev_deps.py)
    - lld (Linux only): sudo apt install lld-20 && sudo ln -s /usr/bin/ld.lld-20 /usr/bin/ld.lld
    - pcre2 development files
        - Debian/Ubuntu: libpcre2-dev
        - openSUSE/RHEL/Fedora: pcre2-devel
        - macOS (Homebrew): pcre2
EOF
}

# Main function
main() {
    # Parse --strip / -s before other arguments
    local args=()
    for arg in "$@"; do
        case "$arg" in
            --strip|-s) STRIP_SYMBOLS="1" ;;
            *) args+=("$arg") ;;
        esac
    done

    case "${args[0]:-}" in
        --cpp|-c)
            check_cpp_deps
            build_cpp
            ;;
        --cpp-test)
            check_cpp_deps
            build_cpp_test
            ;;
        --go|-g)
            check_go_deps
            build_go
            ;;
        --test|-t)
            check_go_deps
            if [ "${args[1]:-}" = "--" ]; then
                run_go_tests "${args[@]:2}"
            else
                run_go_tests "${args[@]:1}"
            fi
            ;;
        --test-integration)
            check_go_deps
            if [ "${args[1]:-}" = "--" ]; then
                run_go_tests_tagged integration "${args[@]:2}"
            else
                run_go_tests_tagged integration "${args[@]:1}"
            fi
            run_native_integration_tests
            ;;
        --test-integration-go)
            # Runs only the generic `integration` test tier (MySQL/MinIO/Redis/
            # NATS/Infinity/ES/etc.) WITHOUT the model-backed native DeepDoc
            # tests. The native tests need MODEL_DIR + the InfiniFlow/deepdoc
            # snapshot and are covered separately by `--test-native` /
            # `ragflow_native_backend` in CI; running them here would also force
            # the `fetch_testdata` tag's init-time network fetch of testdata,
            # which is undesirable in a generic CI run. Use this target for the
            # CI integration job. e.g. `$0 --test-integration-go ./internal/storage/...`
            check_go_deps
            if [ "${args[1]:-}" = "--" ]; then
                run_go_tests_tagged integration "${args[@]:2}"
            else
                run_go_tests_tagged integration "${args[@]:1}"
            fi
            ;;
        --test-e2e)
            check_go_deps
            if [ "${args[1]:-}" = "--" ]; then
                run_go_tests_tagged e2e "${args[@]:2}"
            else
                run_go_tests_tagged e2e "${args[@]:1}"
            fi
            ;;
        --test-manual)
            check_go_deps
            if [ "${args[1]:-}" = "--" ]; then
                run_go_tests_tagged manual "${args[@]:2}"
            else
                run_go_tests_tagged manual "${args[@]:1}"
            fi
            ;;
        --test-all)
            check_go_deps
            if [ "${args[1]:-}" = "--" ]; then
                run_go_tests_tagged "integration e2e" "${args[@]:2}"
            else
                run_go_tests_tagged "integration e2e" "${args[@]:1}"
            fi
            ;;
        --test-native)
            check_go_deps
            if [ "${args[1]:-}" = "--" ]; then
                pkgs=("${args[@]:2}")
            else
                pkgs=("${args[@]:1}")
            fi
            if [ "${#pkgs[@]}" -eq 0 ]; then
                pkgs=(./internal/deepdoc/parser/pdf/inference/native_analyzer/...)
            fi
            # The in-process (Go) DeepDoc backend needs the .ort weights. Default
            # MODEL_DIR to the canonical repo model dir (internal/rag/res/deepdoc) so a
            # local run needs no MODEL_DIR export after `download_deps.py`.
            REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
            export MODEL_DIR="${MODEL_DIR:-$REPO_ROOT/internal/rag/res/deepdoc}"
            run_go_tests_tagged "cgo integration" "${pkgs[@]}"
            run_native_integration_tests
            ;;
        --clean|-C)
            clean
            ;;
        --check-ort-version)
            check_ort_version_consistency
            ;;
        --run|-r)
            check_cpp_deps
            check_go_deps
            build_cpp
            build_go
            run
            ;;
        --help|-h)
            show_help
            ;;
        --all|-a|"")
            check_cpp_deps
            check_go_deps
            build_cpp
            build_go
            echo -e "\n${GREEN}=== Build completed successfully! ===${NC}"
            echo "Binary: $RAGFLOW_SERVER_BINARY, $RAGFLOW_CLI_BINARY"
            ;;
        *)
            echo -e "${RED}Unknown option: ${args[0]}${NC}"
            show_help
            exit 1
            ;;
    esac
}

# Only run the build when executed directly. When sourced (e.g. by tests),
# skip main so the version-gate helpers can be unit-tested in isolation.
if [ "${BASH_SOURCE[0]}" = "$0" ]; then
    main "$@"
fi
