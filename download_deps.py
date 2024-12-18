#!/usr/bin/env python3
#
# Install this script's dependencies with pip3:
# pip3 install huggingface-hub nltk


from huggingface_hub import snapshot_download
import nltk
import os
import urllib.request

urls = [
    "http://archive.ubuntu.com/ubuntu/pool/main/o/openssl/libssl1.1_1.1.1f-1ubuntu2_amd64.deb",
    "http://ports.ubuntu.com/pool/main/o/openssl/libssl1.1_1.1.1f-1ubuntu2_arm64.deb",
    "https://repo1.maven.org/maven2/org/apache/tika/tika-server-standard/3.0.0/tika-server-standard-3.0.0.jar",
    "https://repo1.maven.org/maven2/org/apache/tika/tika-server-standard/3.0.0/tika-server-standard-3.0.0.jar.md5",
    "https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken",
    "https://bit.ly/chrome-linux64-121-0-6167-85",
    "https://bit.ly/chromedriver-linux64-121-0-6167-85",
]

repos = [
    "InfiniFlow/text_concat_xgb_v1.0",
    "InfiniFlow/deepdoc",
    "InfiniFlow/huqie",
    "BAAI/bge-large-zh-v1.5",
    "BAAI/bge-reranker-v2-m3",
    "maidalun1020/bce-embedding-base_v1",
    "maidalun1020/bce-reranker-base_v1",
]

def download_model(repo_id):
    local_dir = os.path.abspath(os.path.join("huggingface.co", repo_id))
    os.makedirs(local_dir, exist_ok=True)
    snapshot_download(repo_id=repo_id, local_dir=local_dir, local_dir_use_symlinks=False)


if __name__ == "__main__":
    for url in urls:
        filename = url.split("/")[-1]
        print(f"Downloading {url}...")
        if not os.path.exists(filename):
            urllib.request.urlretrieve(url, filename)

    local_dir = os.path.abspath('nltk_data')
    for data in ['wordnet', 'punkt', 'punkt_tab']:
        print(f"Downloading nltk {data}...")
        nltk.download(data, download_dir=local_dir)

    for repo_id in repos:
        print(f"Downloading huggingface repo {repo_id}...")
        download_model(repo_id)
