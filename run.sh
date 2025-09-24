#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
# 若有虚拟环境就用，没有也不报错
[ -f .venv/bin/activate ] && source .venv/bin/activate
python -m rag.app.local_edit "${1:-$(xdg-user-dir DESKTOP)/default.pdf}"
