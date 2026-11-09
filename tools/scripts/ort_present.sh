#!/usr/bin/env bash
# Pure helper for build.sh: decide whether the required ONNX Runtime static lib
# is already present in the version+platform-keyed user cache.
#
#   $1 = ONNXRUNTIME_STATIC_PREFIX  root holding the extracted ORT dirs
#   $2 = ort_dir_prefix             platform top-level dir prefix
#                                     (see build.sh ort_static_lib_prefix)
#   $3 = ort_version                DeepDocORTVersion (internal/common/environments.go)
#
# Exits 0 (present) when at least one real base libonnxruntime*.a lives under a
# dir matching BOTH the target platform and the required version —
# */${ort_dir_prefix}${ort_version}*/lib/*.a — and 1 (missing) otherwise. This is
# the SAME dir check_onnxruntime_deps links, so a shared cache holding only an
# OLD version or a DIFFERENT platform's ORT is reported missing (and the
# controlled auto-downloader runs) instead of being silently treated as present
# and failing only at link time.
#
# The lib/ segment is required so a stray .a sitting in the prefix root (not
# under a versioned .../lib/ dir) is ignored. Provider-specific archives
# (*cuda*/*tensorrt*/*coreml*/*dml*/*migraphx*) are excluded, matching the
# link-time selection in check_onnxruntime_deps.
ragflow_ort_present() {
    local prefix="$1" ort_dir_prefix="$2" ort_version="$3"
    local f
    while IFS= read -r f; do
        case "$(basename "$f")" in
            *cuda*|*tensorrt*|*coreml*|*dml*|*migraphx*) continue ;;
        esac
        case "$f" in
            */${ort_dir_prefix}${ort_version}*/lib/*.a) return 0 ;;
        esac
    done < <(find "$prefix" -type f -name '*.a' 2>/dev/null)
    return 1
}
