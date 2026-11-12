# Build from deps/ after running `go run ./cmd/ragflow-deps --image` from the repository root:
# docker build -f Dockerfile -t infiniflow/ragflow_deps:latest .
FROM scratch

COPY cl100k_base.tiktoken NotoSansCJKsc-VF.ttf /

COPY huggingface.co /huggingface.co
