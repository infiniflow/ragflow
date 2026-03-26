#!/bin/bash
set -e

PACKAGES=(
    "./internal/admin/..."
#    "./internal/binding/..."
    "./internal/cache/..."
    "./internal/cli/..."
    "./internal/common/..."
    "./internal/dao/..."
    "./internal/engine/..."
    "./internal/handler/..."
    "./internal/logger/..."
    "./internal/model/..."
    "./internal/router/..."
    "./internal/server/..."
#    "./internal/service/..."
    "./internal/storage/..."
    "./internal/tokenizer/..."
#    "./internal/utility/..."
)

echo "Running tests for specific packages..."
for pkg in "${PACKAGES[@]}"; do
    echo "=== Testing $pkg ==="
    go test $pkg -v -cover
    echo ""
done

#echo "Running all tests except failed packages..."
#go test $(go list ./internal/... | grep -v -E '(cli|service|binding)$') -v