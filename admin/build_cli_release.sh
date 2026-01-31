#!/bin/bash

set -e

echo "🚀 Start building..."
echo "================================"

PROJECT_NAME="ragflow-cli"

RELEASE_DIR="release"
BUILD_DIR="dist"
SOURCE_DIR="src"
PACKAGE_DIR="ragflow_cli"

echo "🧹 Clean old build folder..."
rm -rf release/

echo "📁 Prepare source code..."
mkdir release/$PROJECT_NAME/$SOURCE_DIR -p
cp pyproject.toml release/$PROJECT_NAME/pyproject.toml
cp README.md release/$PROJECT_NAME/README.md

mkdir release/$PROJECT_NAME/$SOURCE_DIR/$PACKAGE_DIR -p
cp admin_client.py release/$PROJECT_NAME/$SOURCE_DIR/$PACKAGE_DIR/admin_client.py

if [ -d "release/$PROJECT_NAME/$SOURCE_DIR" ]; then
    echo "✅ source dir: release/$PROJECT_NAME/$SOURCE_DIR"
else
    echo "❌ source dir not exist: release/$PROJECT_NAME/$SOURCE_DIR"
    exit 1
fi

echo "🔨 Make build file..."
cd release/$PROJECT_NAME
export PYTHONPATH=$(pwd)
python -m build

echo "✅ check build result..."
if [ -d "$BUILD_DIR" ]; then
    echo "📦 Package generated:"
    ls -la $BUILD_DIR/
else
    echo "❌ Build Failed: $BUILD_DIR not exist."
    exit 1
fi

echo "🎉 Build finished successfully!"