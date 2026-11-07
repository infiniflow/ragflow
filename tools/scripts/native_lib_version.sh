#!/usr/bin/env bash
# Single source of truth for the native static-library versions build.sh and
# tools/scripts/download_dev_deps.py agree on. Source this file from build.sh;
# it defines only functions (no top-level side effects) so it is safe to source.
#
# Each native lib resolves its version from exactly one place:
#   - office_oxide / pdf_oxide  -> go.mod       (go_module_version)
#   - ORT                       -> internal/common/environments.go
#                                   DeepDocORTVersion (read by both build.sh and
#                                   download_dev_deps.py directly — it is Go-owned)
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
