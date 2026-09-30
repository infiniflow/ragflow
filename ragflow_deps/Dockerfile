# Build from the repository root after running download_deps.py:
# docker build -f ragflow_deps/Dockerfile -t infiniflow/ragflow_deps:latest .
FROM scratch

COPY ragflow_deps/cl100k_base.tiktoken ragflow_deps/stagehand-server-v3-linux-x64 ragflow_deps/stagehand-server-v3-linux-arm64 /

COPY rag/res/deepdoc/det.ort rag/res/deepdoc/layout.ort rag/res/deepdoc/tsr.ort rag/res/deepdoc/rec.ort rag/res/deepdoc/ocr.res /huggingface.co/InfiniFlow/deepdoc/

COPY ragflow_deps/huggingface.co/BAAI/bge-m3/sentencepiece.bpe.model /huggingface.co/BAAI/bge-m3/
COPY ragflow_deps/huggingface.co/BAAI/bge-large-en-v1.5/vocab.txt /huggingface.co/BAAI/bge-large-en-v1.5/
COPY ragflow_deps/huggingface.co/Qwen/Qwen3-Embedding-0.6B/tokenizer.json /huggingface.co/Qwen/Qwen3-Embedding-0.6B/
COPY ragflow_deps/huggingface.co/intfloat/e5-mistral-7b-instruct/tokenizer.json /huggingface.co/intfloat/e5-mistral-7b-instruct/
