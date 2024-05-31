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
from typing import Optional

import requests
from huggingface_hub import snapshot_download
from zhipuai import ZhipuAI
import os
from abc import ABC
from ollama import Client
import dashscope
from openai import OpenAI
from FlagEmbedding import FlagModel
import torch
import numpy as np

from api.utils.file_utils import get_home_cache_dir
from rag.utils import num_tokens_from_string, truncate


class Base(ABC):
    def __init__(self, key, model_name):
        pass

    def encode(self, texts: list, batch_size=32):
        raise NotImplementedError("Please implement encode method!")

    def encode_queries(self, text: str):
        raise NotImplementedError("Please implement encode method!")


class DefaultEmbedding(Base):
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
        if not DefaultEmbedding._model:
            try:
                self._model = FlagModel(os.path.join(get_home_cache_dir(), re.sub(r"^[a-zA-Z]+/", "", model_name)),
                                        query_instruction_for_retrieval="为这个句子生成表示以用于检索相关文章：",
                                        use_fp16=torch.cuda.is_available())
            except Exception as e:
                model_dir = snapshot_download(repo_id="BAAI/bge-large-zh-v1.5",
                                                local_dir=os.path.join(get_home_cache_dir(), re.sub(r"^[a-zA-Z]+/", "", model_name)),
                                                local_dir_use_symlinks=False)
                self._model = FlagModel(model_dir,
                                        query_instruction_for_retrieval="为这个句子生成表示以用于检索相关文章：",
                                        use_fp16=torch.cuda.is_available())

    def encode(self, texts: list, batch_size=32):
        texts = [truncate(t, 2048) for t in texts]
        token_count = 0
        for t in texts:
            token_count += num_tokens_from_string(t)
        res = []
        for i in range(0, len(texts), batch_size):
            res.extend(self._model.encode(texts[i:i + batch_size]).tolist())
        return np.array(res), token_count

    def encode_queries(self, text: str):
        token_count = num_tokens_from_string(text)
        return self._model.encode_queries([text]).tolist()[0], token_count


class OpenAIEmbed(Base):
    def __init__(self, key, model_name="text-embedding-ada-002",
                 base_url="https://api.openai.com/v1"):
        if not base_url:
            base_url = "https://api.openai.com/v1"
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name

    def encode(self, texts: list, batch_size=32):
        texts = [truncate(t, 8196) for t in texts]
        res = self.client.embeddings.create(input=texts,
                                            model=self.model_name)
        return np.array([d.embedding for d in res.data]
                        ), res.usage.total_tokens

    def encode_queries(self, text):
        res = self.client.embeddings.create(input=[truncate(text, 8196)],
                                            model=self.model_name)
        return np.array(res.data[0].embedding), res.usage.total_tokens


class BaiChuanEmbed(OpenAIEmbed):
    def __init__(self, key,
                 model_name='Baichuan-Text-Embedding',
                 base_url='https://api.baichuan-ai.com/v1'):
        if not base_url:
            base_url = "https://api.baichuan-ai.com/v1"
        super().__init__(key, model_name, base_url)


class QWenEmbed(Base):
    def __init__(self, key, model_name="text_embedding_v2", **kwargs):
        dashscope.api_key = key
        self.model_name = model_name

    def encode(self, texts: list, batch_size=10):
        import dashscope
        try:
            res = []
            token_count = 0
            texts = [truncate(t, 2048) for t in texts]
            for i in range(0, len(texts), batch_size):
                resp = dashscope.TextEmbedding.call(
                    model=self.model_name,
                    input=texts[i:i + batch_size],
                    text_type="document"
                )
                embds = [[] for _ in range(len(resp["output"]["embeddings"]))]
                for e in resp["output"]["embeddings"]:
                    embds[e["text_index"]] = e["embedding"]
                res.extend(embds)
                token_count += resp["usage"]["total_tokens"]
            return np.array(res), token_count
        except Exception as e:
            raise Exception("Account abnormal. Please ensure it's on good standing.")
        return np.array([]), 0

    def encode_queries(self, text):
        try:
            resp = dashscope.TextEmbedding.call(
                model=self.model_name,
                input=text[:2048],
                text_type="query"
            )
            return np.array(resp["output"]["embeddings"][0]
                            ["embedding"]), resp["usage"]["total_tokens"]
        except Exception as e:
            raise Exception("Account abnormal. Please ensure it's on good standing.")
        return np.array([]), 0


class ZhipuEmbed(Base):
    def __init__(self, key, model_name="embedding-2", **kwargs):
        self.client = ZhipuAI(api_key=key)
        self.model_name = model_name

    def encode(self, texts: list, batch_size=32):
        arr = []
        tks_num = 0
        for txt in texts:
            res = self.client.embeddings.create(input=txt,
                                                model=self.model_name)
            arr.append(res.data[0].embedding)
            tks_num += res.usage.total_tokens
        return np.array(arr), tks_num

    def encode_queries(self, text):
        res = self.client.embeddings.create(input=text,
                                            model=self.model_name)
        return np.array(res.data[0].embedding), res.usage.total_tokens


class OllamaEmbed(Base):
    def __init__(self, key, model_name, **kwargs):
        self.client = Client(host=kwargs["base_url"])
        self.model_name = model_name

    def encode(self, texts: list, batch_size=32):
        arr = []
        tks_num = 0
        for txt in texts:
            res = self.client.embeddings(prompt=txt,
                                         model=self.model_name)
            arr.append(res["embedding"])
            tks_num += 128
        return np.array(arr), tks_num

    def encode_queries(self, text):
        res = self.client.embeddings(prompt=text,
                                     model=self.model_name)
        return np.array(res["embedding"]), 128


class FastEmbed(Base):
    _model = None

    def __init__(
            self,
            key: Optional[str] = None,
            model_name: str = "BAAI/bge-small-en-v1.5",
            cache_dir: Optional[str] = None,
            threads: Optional[int] = None,
            **kwargs,
    ):
        from fastembed import TextEmbedding
        if not FastEmbed._model:
            self._model = TextEmbedding(model_name, cache_dir, threads, **kwargs)

    def encode(self, texts: list, batch_size=32):
        # Using the internal tokenizer to encode the texts and get the total
        # number of tokens
        encodings = self._model.model.tokenizer.encode_batch(texts)
        total_tokens = sum(len(e) for e in encodings)

        embeddings = [e.tolist() for e in self._model.embed(texts, batch_size)]

        return np.array(embeddings), total_tokens

    def encode_queries(self, text: str):
        # Using the internal tokenizer to encode the texts and get the total
        # number of tokens
        encoding = self._model.model.tokenizer.encode(text)
        embedding = next(self._model.query_embed(text)).tolist()

        return np.array(embedding), len(encoding.ids)


class XinferenceEmbed(Base):
    def __init__(self, key, model_name="", base_url=""):
        self.client = OpenAI(api_key="xxx", base_url=base_url)
        self.model_name = model_name

    def encode(self, texts: list, batch_size=32):
        res = self.client.embeddings.create(input=texts,
                                            model=self.model_name)
        return np.array([d.embedding for d in res.data]
                        ), res.usage.total_tokens

    def encode_queries(self, text):
        res = self.client.embeddings.create(input=[text],
                                            model=self.model_name)
        return np.array(res.data[0].embedding), res.usage.total_tokens


class YoudaoEmbed(Base):
    _client = None

    def __init__(self, key=None, model_name="maidalun1020/bce-embedding-base_v1", **kwargs):
        from BCEmbedding import EmbeddingModel as qanthing
        if not YoudaoEmbed._client:
            try:
                print("LOADING BCE...")
                YoudaoEmbed._client = qanthing(model_name_or_path=os.path.join(
                    get_home_cache_dir(),
                    "bce-embedding-base_v1"))
            except Exception as e:
                YoudaoEmbed._client = qanthing(
                    model_name_or_path=model_name.replace(
                        "maidalun1020", "InfiniFlow"))

    def encode(self, texts: list, batch_size=10):
        res = []
        token_count = 0
        for t in texts:
            token_count += num_tokens_from_string(t)
        for i in range(0, len(texts), batch_size):
            embds = YoudaoEmbed._client.encode(texts[i:i + batch_size])
            res.extend(embds)
        return np.array(res), token_count

    def encode_queries(self, text):
        embds = YoudaoEmbed._client.encode([text])
        return np.array(embds[0]), num_tokens_from_string(text)


class JinaEmbed(Base):
    def __init__(self, key, model_name="jina-embeddings-v2-base-zh",
                 base_url="https://api.jina.ai/v1/embeddings"):

        self.base_url = "https://api.jina.ai/v1/embeddings"
        self.headers = {
            "Content-Type": "application/json",
            "Authorization": f"Bearer {key}"
        }
        self.model_name = model_name

    def encode(self, texts: list, batch_size=None):
        texts = [truncate(t, 8196) for t in texts]
        data = {
            "model": self.model_name,
            "input": texts,
            'encoding_type': 'float'
        }
        res = requests.post(self.base_url, headers=self.headers, json=data).json()
        return np.array([d["embedding"] for d in res["data"]]), res["usage"]["total_tokens"]

    def encode_queries(self, text):
        embds, cnt = self.encode([text])
        return np.array(embds[0]), cnt