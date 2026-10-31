# Build from deps/ after running download_deps.py:
# docker build -f Dockerfile -t infiniflow/ragflow_deps:latest .
FROM scratch

COPY cl100k_base.tiktoken NotoSansCJKsc-VF.ttf /

COPY huggingface.co /huggingface.co
