#!/usr/bin/env bash
# Single source of truth for the native static-library versions build.sh and
# tools/scripts/download_dev_deps.py agree on. Source this file from build.sh;
# it defines only functions (no top-level side effects) so it is safe to source.
#
# Each native lib resolves its version from exactly one place:
#   - office_oxide / pdf_oxide  -> go.mod       (go_module_version)
#   - ORT                       -> internal/common/environments.go
#                                   DeepDocORTVersion (read by both build.sh via
#                                   deepdoc_ort_version() below and
#                                   download_dev_deps.deepdoc_ort_version() — it is
#                                   Go-owned, so the const itself stays in Go)
#   - pdfium                    -> pdfium_static_version (below; pdfium is not a
#                                   Go module, so go.mod does not apply)
#
# There is NO /opt pre-bake and NO flat (version-less) cache layout any more: the
# libs land under ~/ragflow-native-libs/<lib>/v<version>/ (see download_dev_deps.py
# and build.sh), so a single mounted cache carries several versions at once.

# ragflow_native_libs_dir
# Root of the version-keyed native static-lib user cache
# (~/ragflow-native-libs by default). Overridable via RAGFLOW_NATIVE_LIBS_DIR so
# CI can mount the cache at a fixed host path (e.g. a RUNNER_WORKSPACE_PREFIX
# bind mount) instead of the build user's $HOME — see sep-tests.yml Slice 3. An
# EMPTY override falls back to the $HOME default (the ${VAR:-default} expansion),
# so a half-configured runner never resolves to an empty cache root. Echoes the
# resolved path and returns 0.
ragflow_native_libs_dir() {
    printf '%s\n' "${RAGFLOW_NATIVE_LIBS_DIR:-${HOME}/ragflow-native-libs}"
}

# go_module_version <module> <repo_root>
# Reads the pinned version (the "vX.Y.Z" after the require line) of a Go module
# from <repo_root>/go.mod so build.sh and tools/scripts/download_dev_deps.py agree
# on the version (single source of truth). The module may appear either as a bare
# require line (`\t<module> vX.Y.Z`) inside a require block or on a single-line
# `require <module> vX.Y.Z`; an optional leading `require` keyword is allowed.
# Echoes the bare version (no "v" prefix) and returns 0, or returns 1 with empty
# output when go.mod is missing or the module is not required.
go_module_version() {
    local module="$1" repo_root="$2"
    local go_mod="${repo_root}/go.mod"
    [ -f "$go_mod" ] || return 1
    local version
    version="$(grep -m1 -E "^[[:space:]]*(require[[:space:]]+)?${module}[[:space:]]+v" "$go_mod" \
        | sed -E 's/.*v([0-9][^[:space:]]*).*/\1/')"
    [ -n "$version" ] || return 1
    printf '%s\n' "$version"
}

# pdfium_static_version
# pdfium (kognitos/pdfium-static) is NOT a Go module, so unlike office_oxide /
# pdf_oxide its version cannot be derived from go.mod. It is pinned here as the
# single shared constant; download_dev_deps.py and build.sh both source this
# function so the bare numeric build number lives in exactly one place. Echoes
# the constant and returns 0.
pdfium_static_version() {
    # Single source of truth for the pdfium static build number. Bump here only.
    printf '%s\n' "7809"
}

# Give cgo immutable archive paths that change when any linked PDFium archive
# changes, even under the same version. Keep these copies in the project's
# writable cache so a pre-provisioned native dependency cache can be read-only.
pdfium_static_link_dir() (
    local source_dir="$1" target_os="$2" cache_dir="$3/archives"
    local -a archives=(libpdfium.a) hash_command=(sha256sum)
    if [ "$target_os" = "linux" ]; then
        archives+=(libc++.a libc++abi.a)
    fi
    command -v sha256sum >/dev/null || hash_command=(shasum -a 256)

    local hashes digest link_dir
    hashes="$(cd "$source_dir" && "${hash_command[@]}" "${archives[@]}")" || return 1
    digest="$(printf '%s\n' "$hashes" | "${hash_command[@]}")" || return 1
    digest="${digest%% *}"
    link_dir="$cache_dir/$digest"
    if [ ! -d "$link_dir" ]; then
        mkdir -p "$cache_dir" || return 1
        local staging staged_dir archive staged_hashes
        staging="$(mktemp -d "$cache_dir/.pdfium.XXXXXX")" || return 1
        trap 'rm -rf -- "$staging"' EXIT
        trap 'rm -rf -- "$staging"; exit 130' INT
        trap 'rm -rf -- "$staging"; exit 143' TERM
        staged_dir="$staging/$digest"
        mkdir "$staged_dir" || return 1
        for archive in "${archives[@]}"; do
            cp "$source_dir/$archive" "$staged_dir/$archive" || return 1
        done
        staged_hashes="$(cd "$staged_dir" && "${hash_command[@]}" "${archives[@]}")" || return 1
        if [ "$staged_hashes" != "$hashes" ]; then
            echo "PDFium archives changed while preparing linker inputs. Retry the build." >&2
            return 1
        fi
        # Rename into the cache parent in one operation. The published directory
        # is outside the attempt's cleanup, and another build may publish first.
        if ! mv -f "$staged_dir" "$cache_dir/" 2>/dev/null && [ ! -d "$link_dir" ]; then
            echo "Could not publish the PDFium linker cache at $link_dir" >&2
            return 1
        fi
    fi
    printf '%s\n' "$link_dir"
)

# deepdoc_ort_version <repo_root>
# Reads the ONNX Runtime version the in-process (Go) DeepDoc backend links from
# <repo_root>/internal/common/environments.go's DeepDocORTVersion const.
#
# ORT is the one native lib whose version is owned by Go source (not go.mod), so
# build.sh and download_dev_deps.deepdoc_ort_version() both parse the SAME const
# through this helper — no second hardcoded copy to drift. Echoes the bare
# version and returns 0, or returns 1 with empty output when environments.go is
# missing or the const is absent (so callers fail loud, mirroring the Python
# RuntimeError). build.sh used to inline this grep at every call site; collapsing
# it here keeps a single bash source of truth alongside the Python one.
deepdoc_ort_version() {
    local repo_root="$1"
    local env_go="${repo_root}/internal/common/environments.go"
    [ -f "$env_go" ] || return 1
    local version
    version="$(grep -m1 -E 'DeepDocORTVersion[[:space:]]*=[[:space:]]*"' "$env_go" \
        | sed -E 's/.*"([^"]+)".*/\1/')"
    [ -n "$version" ] || return 1
    printf '%s\n' "$version"
}
