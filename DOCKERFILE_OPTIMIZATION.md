# Dockerfile Optimization for Pre-installing Dependencies

## Problem
The original Dockerfile was downloading and installing Python dependencies (`docling` and `mineru[core]`) at every container startup via the `entrypoint.sh` script. This caused:

1. Slow container startup times
2. Network dependency during container runtime
3. Unnecessary repeated downloads of the same packages
4. Potential failures if package repositories are unavailable at runtime

## Solution
Modified the Dockerfile to pre-install these dependencies during the image build process:

### Changes Made

#### 1. Dockerfile Modifications

**Added to builder stage:**
```dockerfile
# Pre-install optional dependencies that are normally installed at runtime
# This prevents downloading dependencies on every container startup
RUN --mount=type=cache,id=ragflow_uv,target=/root/.cache/uv,sharing=locked \
    if [ "$NEED_MIRROR" == "1" ]; then \
        uv pip install -i https://pypi.tuna.tsinghua.edu.cn/simple --extra-index-url https://pypi.org/simple --no-cache-dir "docling==2.58.0"; \
    else \
        uv pip install --no-cache-dir "docling==2.58.0"; \
    fi

# Pre-install mineru in a separate directory that can be used at runtime
RUN --mount=type=cache,id=ragflow_uv,target=/root/.cache/uv,sharing=locked \
    mkdir -p /ragflow/uv_tools && \
    uv venv /ragflow/uv_tools/.venv && \
    if [ "$NEED_MIRROR" == "1" ]; then \
        /ragflow/uv_tools/.venv/bin/uv pip install -U "mineru[core]" -i https://mirrors.aliyun.com/pypi/simple --extra-index-url https://pypi.org/simple; \
    else \
        /ragflow/uv_tools/.venv/bin/uv pip install -U "mineru[core]"; \
    fi
```

**Added to production stage:**
```dockerfile
# Copy pre-installed mineru environment
COPY --from=builder /ragflow/uv_tools /ragflow/uv_tools
```

#### 2. Entrypoint Script Optimizations

Modified the `ensure_docling()` and `ensure_mineru()` functions in `docker/entrypoint.sh` to:

1. **Check for pre-installed packages first** - Look for already installed dependencies before attempting to install
2. **Fallback to runtime installation** - Only install at runtime if the pre-installed packages are not found or not working
3. **Better error handling** - Verify that installed packages actually work before proceeding

## Benefits

1. **Faster startup times** - No dependency downloads during container startup in normal cases
2. **Improved reliability** - Less dependency on external package repositories at runtime
3. **Better caching** - Docker build cache ensures dependencies are only downloaded when the Dockerfile changes
4. **Offline capability** - Containers can start even without internet access (assuming pre-built image)
5. **Predictable deployments** - Dependencies are locked at build time, reducing runtime variability

## Backward Compatibility

The changes maintain backward compatibility:
- Environment variables `USE_DOCLING` and `USE_MINERU` still control whether these packages are used
- If pre-installed packages are missing or broken, the system falls back to runtime installation
- All existing functionality is preserved

## Build Size Impact

- **docling**: Adds ~100-200MB to the image size
- **mineru[core]**: Adds ~200-400MB to the image size (in separate venv)
- **Total**: Approximately 300-600MB increase in image size

This trade-off is generally worthwhile for production deployments where fast startup times are more important than image size.

## Usage

After rebuilding the Docker image with these changes:

1. Containers will start much faster when `USE_DOCLING=true` and/or `USE_MINERU=true`
2. No internet access is required at container startup for these dependencies
3. The system will automatically fall back to runtime installation if needed

## Environment Variables

The optimization respects existing environment variables:
- `USE_DOCLING=true/false` - Controls docling usage
- `USE_MINERU=true/false` - Controls mineru usage
- `DOCLING_VERSION` - Controls docling version (defaults to ==2.58.0)
- `NEED_MIRROR=1` - Uses Chinese mirrors for package downloads