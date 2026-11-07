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
ragflow_ort_stamp_target() {
    local versioned_dir="$1" archive_file="$2"
    local link_name link_target want
    link_name="$(basename "$archive_file")"
    link_target="${versioned_dir}/${link_name}"
    # -m: compute the relative path without requiring either path to exist yet.
    want="$(realpath -m --relative-to="$versioned_dir" "$archive_file")"
    local needs_write="yes"
    if [ -L "$link_target" ] && \
       [ "$(readlink -f "$link_target" 2>/dev/null)" = "$(readlink -f "$archive_file" 2>/dev/null)" ]; then
        needs_write="no"
    fi
    printf '%s|%s\n' "$needs_write" "$want"
}
