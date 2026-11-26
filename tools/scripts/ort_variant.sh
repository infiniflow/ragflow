#!/usr/bin/env bash
# Pure helper for build.sh: map the requested ONNX Runtime build variant to a
# directory-name suffix so the archive glob selects exactly one variant.
#
# Why this exists: build.sh's archive glob was
#   ${dir_prefix}${version}*/lib/*.a
# which matches BOTH the minimal (-glibc2_28) and full (-full) archives when a
# cache holds both. Which one became the link "primary" depended on `find`
# traversal order — nondeterministic, and the wrong (minimal) choice makes a
# CUDA-EP build fail at runtime with "RegisterExecutionProviderLibrary is not
# supported in a minimal build".
#
# The variant suffix narrows the glob:
#   unset / cpu  -> "*"       (legacy behaviour: match any variant dir)
#   gpu / full   -> "-full"   (only the full/CUDA-capable archive)
#   empty / other -> error    (fail fast; never silently reintroduce ambiguity)
#
# Defaulting to "*" keeps OSS/CPU builds byte-for-byte unchanged (additive).

ort_variant_suffix() {
    # Distinguish "unset" (legacy default) from "set to empty" (a typo/config
    # error). ${VAR:-cpu} would collapse both; ${VAR+set} keeps them apart.
    if [ "${DEEPDOC_ORT_VARIANT+set}" != set ]; then
        printf '%s' '*'
        return 0
    fi
    case "${DEEPDOC_ORT_VARIANT}" in
        cpu)
            printf '%s' '*'
            ;;
        gpu|full)
            printf '%s' '-full'
            ;;
        *)
            # An unknown or empty, explicitly-set variant must NOT silently
            # fall back to the wildcard: that would reintroduce the minimal/full
            # ambiguity this helper exists to remove. Fail loudly at build time.
            printf 'unknown DEEPDOC_ORT_VARIANT=%s (expected cpu, gpu or full)\n' \
                "${DEEPDOC_ORT_VARIANT}" >&2
            return 1
            ;;
    esac
}
