#!/usr/bin/env bash
# Pure helper for build.sh: decide how to version-stamp the ORT static archive
# with a symlink. Echoes "<needs_write>|<relative_target>".
#
#   $1 = versioned_dir  the v<version>-<sha16> stamp dir (under the native cache)
#   $2 = archive_file   absolute path to the real libonnxruntime.a
#
# The stamp target is RELATIVE (never absolute) so it resolves under ANY mount
# point of the cache (build host, runner, CI builder container). `needs_write`
# is "no" when an existing symlink already resolves to the same archive, so a
# read-only cache mount is a no-op instead of a fatal `ln` on a read-only fs.
#
# Both paths are absolute and share the static_lib root, but nothing here may
# depend on GNU coreutils: `realpath -m --relative-to` does not exist on macOS
# (BSD realpath has neither flag) and silently yields an empty target there.

# Echo <to_path> expressed relative to <from_dir>; both must be absolute.
ragflow_relative_path() {
    local from="$1" to="$2" rel=""
    while [ "${to#"$from"/}" = "$to" ] && [ "$from" != "/" ]; do
        from="$(dirname "$from")"
        rel="../$rel"
    done
    printf '%s%s\n' "$rel" "${to#"$from"/}"
}

ragflow_ort_stamp_target() {
    local versioned_dir="$1" archive_file="$2"
    local link_name link_target want
    link_name="$(basename "$archive_file")"
    link_target="${versioned_dir}/${link_name}"
    want="$(ragflow_relative_path "$versioned_dir" "$archive_file")"
    local needs_write="yes"
    # Same spelling means the link already points at this archive; no write, so
    # a read-only cache mount stays a no-op.
    if [ -L "$link_target" ] && [ "$(readlink "$link_target")" = "$want" ]; then
        needs_write="no"
    fi
    printf '%s|%s\n' "$needs_write" "$want"
}
