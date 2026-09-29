#!/usr/bin/env bash
# A normal Go build must provision the BPE table before producing a server.
set -u

source "$(dirname "${BASH_SOURCE[0]}")/../build.sh"
set +e  # build.sh enables errexit; the missing C++ library is intentional here.

test_dir="$(mktemp -d)"
trap 'rm -rf "$test_dir"' EXIT
PROJECT_ROOT="$test_dir/project"
BUILD_DIR="$PROJECT_ROOT/internal/binding/cpp/cmake-build-release"
SYSTEM_DEPS_TOKENIZER="$test_dir/preseed"
mkdir -p "$PROJECT_ROOT" "$SYSTEM_DEPS_TOKENIZER"
printf 'preseeded Go BPE table\n' > "$SYSTEM_DEPS_TOKENIZER/cl100k_base.tiktoken"

output="$(build_go 2>&1)"
status=$?
if [ "$status" -ne 1 ] || [[ "$output" != *"C++ static library not found"* ]]; then
    printf 'Unexpected build result (status %s):\n%s\n' "$status" "$output" >&2
    exit 1
fi
if ! cmp -s "$SYSTEM_DEPS_TOKENIZER/cl100k_base.tiktoken" "$PROJECT_ROOT/ragflow_deps/cl100k_base.tiktoken"; then
    echo "Go build did not provision cl100k_base.tiktoken before checking native libraries" >&2
    exit 1
fi

echo "Go build provisioned cl100k_base.tiktoken"
