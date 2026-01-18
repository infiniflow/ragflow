#!/bin/bash

# Exit on error
set -e

# Default values
TARGET_PLATFORM="linux/amd64"
DEFAULT_TAG="dev"
DEFAULT_IMAGE="ragflow-local"
DEFAULT_CONTEXT="unraid"

# Parse arguments
POSITIONAL_ARGS=()
PUSH_FLAG="--load"     # Default to load (local)
LOCAL_MODE=true        # Default to local
CONTEXT="$DEFAULT_CONTEXT"

while [[ $# -gt 0 ]]; do
  case $1 in
    -h|--help)
      echo "Usage: $0 [options] [image_name] [tag]"
      echo ""
      echo "Arguments:"
      echo "  image_name   Image name (default: $DEFAULT_IMAGE)"
      echo "  tag          Tag (default: $DEFAULT_TAG)"
      echo ""
      echo "Options:"
      echo "  --local      Build and load into Docker daemon (default: enabled)"
      echo "  --push       Push to registry (disables --local)"
      echo "  --platform   Target platform (default: $TARGET_PLATFORM)"
      echo "  --context    Docker context to use (default: $DEFAULT_CONTEXT)"
      echo ""
      exit 0
      ;;
    --local)
      LOCAL_MODE=true
      PUSH_FLAG="--load"
      shift
      ;;
    --push)
      LOCAL_MODE=false
      PUSH_FLAG="--push"
      shift
      ;;
    --platform)
      if [[ -z "$2" || "$2" == -* ]]; then
        echo "Error: Argument for --platform is missing or invalid."
        exit 1
      fi
      TARGET_PLATFORM="$2"
      shift 2
      ;;
    --context)
      if [[ -z "$2" || "$2" == -* ]]; then
        echo "Error: Argument for --context is missing or invalid."
        exit 1
      fi
      CONTEXT="$2"
      shift 2
      ;;
    *)
      POSITIONAL_ARGS+=("$1")
      shift
      ;;
  esac
done

set -- "${POSITIONAL_ARGS[@]}" # restore positional parameters

IMAGE_NAME="${1:-$DEFAULT_IMAGE}"
TAG="${2:-$DEFAULT_TAG}"
FULL_IMAGE="$IMAGE_NAME:$TAG"

# Get project root (one level up from scripts/)
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "=============================================="
echo "RAGFlow Docker Publisher"
echo "=============================================="
echo "Image:    $FULL_IMAGE"
echo "Platform: $TARGET_PLATFORM"
echo "Mode:     $([ "$LOCAL_MODE" = true ] && echo "Local (Load)" || echo "Registry (Push)")"
echo "Root:     $PROJECT_ROOT"
echo "Context:  ${CONTEXT:-$(docker context show)}"
echo "=============================================="

# Handle Docker Context
if [ -n "$CONTEXT" ]; then
    ORIGINAL_CONTEXT=$(docker context show)
    if [ "$ORIGINAL_CONTEXT" != "$CONTEXT" ]; then
        echo "🔄 Switching Docker context to: $CONTEXT"
        if ! docker context use "$CONTEXT" > /dev/null; then
            echo "Error: Failed to switch to context '$CONTEXT'"
            exit 1
        fi
        
        # Ensure we switch back on exit
        # We use a function to handle cleanup to avoid overwriting other traps if we add them later
        cleanup_context() {
            echo "🔄 Restoring Docker context to: $ORIGINAL_CONTEXT"
            docker context use "$ORIGINAL_CONTEXT" > /dev/null
        }
        trap cleanup_context EXIT
    else
        echo "✅ Already using Docker context: $CONTEXT"
    fi
fi

# Check if docker is running
if ! docker info > /dev/null 2>&1; then
    echo "Error: Docker is not running or you don't have permissions."
    exit 1
fi

# Check if buildx is available (needed for cross-platform builds or --load/--push separation)
if docker buildx version > /dev/null 2>&1; then
    echo "✅ Docker Buildx detected."
    
    # If using --local, we might not need multi-platform, usually implies native or explicit load
    BUILD_CMD="docker buildx build --platform $TARGET_PLATFORM $PUSH_FLAG"
    
    # Check if a builder instance exists
    # For --local (loading to docker daemon), we generally need the 'docker' driver or explicit load support
    # If we are using the default 'desktop-linux' or similar, --load works.
    
    if ! docker buildx inspect > /dev/null 2>&1; then
        echo "Creating new buildx builder..."
        docker buildx create --use --name ragflow-builder --driver docker-container
        docker buildx inspect --bootstrap
    fi
else
    echo "⚠️ Docker Buildx not found. Falling back to standard build."
    if [ "$LOCAL_MODE" = true ]; then
        # Standard build always loads to daemon
        BUILD_CMD="docker build"
        if [ -n "$TARGET_PLATFORM" ]; then
            echo "⚠️ Warning: Using standard build with --platform. This might fail or be ignored if cross-compilation is not supported by the daemon."
            BUILD_CMD="$BUILD_CMD --platform $TARGET_PLATFORM"
        fi
    else
        echo "Error: Cannot push with standard build script logic. Use 'docker push' manually after build."
        exit 1
    fi
fi

echo "🚀 Building..."

# Run the build
cd "$PROJECT_ROOT"
$BUILD_CMD \
    --build-arg NEED_MIRROR=0 \
    -t "$FULL_IMAGE" \
    -f Dockerfile \
    .

echo ""
echo "✅ Success! Image published to: $FULL_IMAGE"
echo "You can now update your Unraid docker-compose.yml:"
echo "---------------------------------------------------"
echo "services:"
echo "  ragflow:"
echo "    image: $FULL_IMAGE"
echo "---------------------------------------------------"
