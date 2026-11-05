#!/usr/bin/env bash
# Resolve the on-disk directory of a native static library for a pinned version.
#
# A single CI runner image may now bake several versions of a library at once
# (e.g. office_oxide v0.1.12 and v0.1.16) under version-keyed subdirs, so PRs
# pinned to different go.mod versions all build offline. select_native_lib_version
# picks the directory build.sh should seed from.
#
# Source this file from build.sh; it defines only functions (no top-level side
# effects) so it is safe to source and unit-testable in isolation.
#
# select_native_lib_version <dep_name> <version> <system_root>
#   Echoes the path to seed from, preferring a version-keyed subdir:
#     <system_root>/<dep_name>/v<version>
#   and falling back to the legacy flat layout:
#     <system_root>/<dep_name>
#   Returns 0 on success (path printed) and 1 when neither exists locally — the
#   caller must then fall back to a network download.

select_native_lib_version() {
    local dep_name="$1" version="$2" system_root="$3"
    local versioned="${system_root}/${dep_name}/v${version}"
    if [ -d "$versioned" ]; then
        printf '%s\n' "$versioned"
        return 0
    fi
    local flat="${system_root}/${dep_name}"
    if [ -d "$flat" ]; then
        printf '%s\n' "$flat"
        return 0
    fi
    return 1
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

# reseed_dep_from_system <dep_name> <version> <system_root> <user_cache_dir>
# Overwrites a (possibly stale) user-cache directory with the pinned version
# baked into the CI runner image at <system_root>. This is the recovery path for
# self-hosted runners where $HOME persists across jobs: an older image may have
# left a wrong-version native lib under ~/ragflow-native-libs/<dep>, which would
# otherwise shadow the correct /opt pre-bake and link a stale .a.
#
# The source directory is resolved by select_native_lib_version (version-keyed
# subdir first, then flat), so only the requested version is ever copied — never
# a different (newer) sibling. Copies with cp -rf so a correct version fully
# replaces a stale one (no leftover files). Returns 0 on success, 1 when /opt has
# no copy of the requested version (caller must then fail or download).
#
# <user_cache_dir> is the dependency ROOT (e.g. ~/ragflow-native-libs/onnxruntime
# for the onnxruntime case, whose lib actually lives under static_lib/), matching
# the layout select_native_lib_version returns.
reseed_dep_from_system() {
    local dep_name="$1" version="$2" system_root="$3" user_cache_dir="$4"
    local src
    src="$(select_native_lib_version "$dep_name" "$version" "$system_root")" || return 1
    [ -n "$src" ] || return 1
    # Purge the destination first so a stale .a that the new version no longer
    # ships cannot linger and get linked (the exact silent-wrong-link failure
    # this path exists to prevent). The destination is a controlled cache dir
    # under ~/ragflow-native-libs, not an arbitrary path.
    rm -rf "$user_cache_dir"
    mkdir -p "$user_cache_dir"
    cp -rf "$src/." "$user_cache_dir/"
    return 0
}
