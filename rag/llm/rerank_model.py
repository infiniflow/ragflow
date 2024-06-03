#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
import re
import requests
import torch
from FlagEmbedding import FlagReranker
from huggingface_hub import snapshot_download
import os
from abc import ABC
import numpy as np
from api.utils.file_utils import get_home_cache_dir
from rag.utils import num_tokens_from_string, truncate

def sigmoid(x):
    return 1 / (1 + np.exp(-x))

class Base(ABC):
    def __init__(self, key, model_name):
        pass

    def similarity(self, query: str, texts: list):
        raise NotImplementedError("Please implement encode method!")


class DefaultRerank(Base):
    _model = None

    def __init__(self, key, model_name, **kwargs):
        """
        If you have trouble downloading HuggingFace models, -_^ this might help!!

        For Linux:
        export HF_ENDPOINT=https://hf-mirror.com

        For Windows:
        Good luck
        ^_-

        """
        if not DefaultRerank._model:
            try:
                self._model = FlagReranker(os.path.join(get_home_cache_dir(), re.sub(r"^[a-zA-Z]+/", "", model_name)),
                                           use_fp16=torch.cuda.is_available())
            except Exception as e:
                self._model = snapshot_download(repo_id=model_name,
                                                local_dir=os.path.join(get_home_cache_dir(),
                                                                       re.sub(r"^[a-zA-Z]+/", "", model_name)),
                                                local_dir_use_symlinks=False)
                self._model = FlagReranker(os.path.join(get_home_cache_dir(), model_name),
                                           use_fp16=torch.cuda.is_available())

    def similarity(self, query: str, texts: list):
        pairs = [(query,truncate(t, 2048)) for t in texts]
        token_count = 0
        for _, t in pairs:
            token_count += num_tokens_from_string(t)
        batch_size = 32
        res = []
        for i in range(0, len(pairs), batch_size):
            scores = self._model.compute_score(pairs[i:i + batch_size], max_length=2048)
            scores = sigmoid(np.array(scores)).tolist()
            res.extend(scores)
        return np.array(res), token_count


class JinaRerank(Base):
    def __init__(self, key, model_name="jina-reranker-v1-base-en",
                 base_url="https://api.jina.ai/v1/rerank"):
        self.base_url = "https://api.jina.ai/v1/rerank"
        self.headers = {
            "Content-Type": "application/json",
            "Authorization": f"Bearer {key}"
        }
        self.model_name = model_name

    def similarity(self, query: str, texts: list):
        texts = [truncate(t, 8196) for t in texts]
        data = {
            "model": self.model_name,
            "query": query,
            "documents": texts,
            "top_n": len(texts)
        }
        res = requests.post(self.base_url, headers=self.headers, json=data).json()
        return np.array([d["relevance_score"] for d in res["results"]]), res["usage"]["total_tokens"]


class YoudaoRerank(DefaultRerank):
    _model = None

    def __init__(self, key=None, model_name="maidalun1020/bce-reranker-base_v1", **kwargs):
        from BCEmbedding import RerankerModel
        if not YoudaoRerank._model:
            try:
                print("LOADING BCE...")
                YoudaoRerank._model = RerankerModel(model_name_or_path=os.path.join(
                    get_home_cache_dir(),
                    re.sub(r"^[a-zA-Z]+/", "", model_name)))
            except Exception as e:
                YoudaoRerank._model = RerankerModel(
                    model_name_or_path=model_name.replace(
                        "maidalun1020", "InfiniFlow"))
    
    def similarity(self, query: str, texts: list):
        pairs = [(query, truncate(t, self._model.max_length)) for t in texts]
        token_count = 0
        for _, t in pairs:
            token_count += num_tokens_from_string(t)
        batch_size = 32
        res = []
        for i in range(0, len(pairs), batch_size):
            scores = self._model.compute_score(pairs[i:i + batch_size], max_length=self._model.max_length)
            scores = sigmoid(np.array(scores)).tolist()
            res.extend(scores)
        return np.array(res), token_count
    

