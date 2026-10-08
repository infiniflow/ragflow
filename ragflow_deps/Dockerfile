# Build from ragflow_deps/ after running download_deps.py:
# docker build -f Dockerfile -t infiniflow/ragflow_deps:latest .
FROM scratch

COPY cl100k_base.tiktoken stagehand-server-v3-linux-x64 stagehand-server-v3-linux-arm64 /

COPY huggingface.co /huggingface.co
