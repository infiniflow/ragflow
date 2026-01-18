# RAGFlow Utility Scripts

This directory contains utility scripts for development and maintenance.

## Scripts

### check_comment_ascii.py
Validates that Python files contain only ASCII characters in comments and docstrings.

**Usage:**
```bash
# Check all Python files in the repository
git ls-files -z -- '*.py' | xargs -0 python3 scripts/check_comment_ascii.py

# Check specific file
python3 scripts/check_comment_ascii.py path/to/file.py
```

### download_deps.py
Downloads external dependencies required by RAGFlow including:
- NLTK data
- Hugging Face models
- Chromedriver and Chrome binaries
- Tika server JAR
- SSL libraries
- TikToken encodings

**Usage:**
```bash
uv run scripts/download_deps.py
```

### dev_setup.sh
Sets up the development environment for RAGFlow backend development.

**Usage:**
```bash
bash scripts/dev_setup.sh
```

### show_env.sh
Displays environment information useful for debugging and support.

**Usage:**
```bash
bash scripts/show_env.sh
```

## Notes

- These scripts are typically run from the repository root directory
- Most scripts use `uv run` for Python execution to ensure correct virtual environment
- Shell scripts should be executed with `bash` for compatibility

### publish_dev_image.sh
Builds and publishes (or loads) the RAGFlow Docker image. Designed for local development flow.

**Usage:**
```bash
# Default (Builds ragflow-local:dev, loads to 'unraid' context)
./scripts/publish_dev_image.sh


# Custom usage
./scripts/publish_dev_image.sh --context unraid --local my-image v2
```

**Options:**
- `--local`: Load image to Docker daemon (default).
- `--push`: Push image to registry.
- `--context <name>`: Switch to specified Docker context during build (default: `unraid`. Update this if your context differs).
- `--platform <plat>`: Specify target platform (default: `linux/amd64`).

**Arguments:**
- `<image-name>`: Name of the Docker image (Optional. Default: `ragflow-local`)
- `<tag>`: Tag for the Docker image (Optional. Default: `dev`)

