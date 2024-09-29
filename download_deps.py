#!/usr/bin/env python3

from huggingface_hub import snapshot_download
import os

repos = [
    "InfiniFlow/text_concat_xgb_v1.0",
    "InfiniFlow/deepdoc",
    "BAAI/bge-large-zh-v1.5",
    "BAAI/bge-reranker-v2-m3",
    "maidalun1020/bce-embedding-base_v1",
    "maidalun1020/bce-reranker-base_v1",
]


def download_model(repo_id):
    local_dir = os.path.join("huggingface.co", repo_id)
    os.makedirs(local_dir, exist_ok=True)
    snapshot_download(repo_id=repo_id, local_dir=local_dir)


if __name__ == "__main__":
    for repo_id in repos:
        download_model(repo_id)
