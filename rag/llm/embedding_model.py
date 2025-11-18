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
import json
import os
import threading
from abc import ABC
from urllib.parse import urljoin

import dashscope
import google.generativeai as genai
import numpy as np
import requests
from ollama import Client
from openai import OpenAI
from zhipuai import ZhipuAI

from common.log_utils import log_exception
from common.token_utils import num_tokens_from_string, truncate
from common import settings
import logging
import base64


class Base(ABC):
    def __init__(self, key, model_name, **kwargs):
        """
        Constructor for abstract base class.
        Parameters are accepted for interface consistency but are not stored.
        Subclasses should implement their own initialization as needed.
        """
        pass

    def encode(self, texts: list):
        raise NotImplementedError("Please implement encode method!")

    def encode_queries(self, text: str):
        raise NotImplementedError("Please implement encode method!")

    def total_token_count(self, resp):
        try:
            return resp.usage.total_tokens
        except Exception:
            pass
        try:
            return resp["usage"]["total_tokens"]
        except Exception:
            pass
        return 0


class BuiltinEmbed(Base):
    _FACTORY_NAME = "Builtin"
    MAX_TOKENS = {"Qwen/Qwen3-Embedding-0.6B": 30000, "BAAI/bge-m3": 8000, "BAAI/bge-small-en-v1.5": 500}
    _model = None
    _model_name = ""
    _max_tokens = 500
    _model_lock = threading.Lock()

    def __init__(self, key, model_name, **kwargs):
        logging.info(f"Initialize BuiltinEmbed according to settings.EMBEDDING_CFG: {settings.EMBEDDING_CFG}")
        embedding_cfg = settings.EMBEDDING_CFG
        if not BuiltinEmbed._model and "tei-" in os.getenv("COMPOSE_PROFILES", ""):
            with BuiltinEmbed._model_lock:
                BuiltinEmbed._model_name = settings.EMBEDDING_MDL
                BuiltinEmbed._max_tokens = BuiltinEmbed.MAX_TOKENS.get(settings.EMBEDDING_MDL, 500)
                BuiltinEmbed._model = HuggingFaceEmbed(embedding_cfg["api_key"], settings.EMBEDDING_MDL, base_url=embedding_cfg["base_url"])
        self._model = BuiltinEmbed._model
        self._model_name = BuiltinEmbed._model_name
        self._max_tokens = BuiltinEmbed._max_tokens

    def encode(self, texts: list):
        batch_size = 16
        # TEI is able to auto truncate inputs according to https://github.com/huggingface/text-embeddings-inference.
        token_count = 0
        ress = None
        for i in range(0, len(texts), batch_size):
            embeddings, token_count_delta = self._model.encode(texts[i : i + batch_size])
            token_count += token_count_delta
            if ress is None:
                ress = embeddings
            else:
                ress = np.concatenate((ress, embeddings), axis=0)
        return ress, token_count

    def encode_queries(self, text: str):
        return self._model.encode_queries(text)


class OpenAIEmbed(Base):
    _FACTORY_NAME = "OpenAI"

    def __init__(self, key, model_name="text-embedding-ada-002", base_url="https://api.openai.com/v1"):
        if not base_url:
            base_url = "https://api.openai.com/v1"
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name

    def encode(self, texts: list):
        # OpenAI requires batch size <=16
        batch_size = 16
        texts = [truncate(t, 8191) for t in texts]
        ress = []
        total_tokens = 0
        for i in range(0, len(texts), batch_size):
            res = self.client.embeddings.create(input=texts[i : i + batch_size], model=self.model_name, encoding_format="float", extra_body={"drop_params": True})
            try:
                ress.extend([d.embedding for d in res.data])
                total_tokens += self.total_token_count(res)
            except Exception as _e:
                log_exception(_e, res)
        return np.array(ress), total_tokens

    def encode_queries(self, text):
        res = self.client.embeddings.create(input=[truncate(text, 8191)], model=self.model_name, encoding_format="float",extra_body={"drop_params": True})
        return np.array(res.data[0].embedding), self.total_token_count(res)


class LocalAIEmbed(Base):
    _FACTORY_NAME = "LocalAI"

    def __init__(self, key, model_name, base_url):
        if not base_url:
            raise ValueError("Local embedding model url cannot be None")
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key="empty", base_url=base_url)
        self.model_name = model_name.split("___")[0]

    def encode(self, texts: list):
        batch_size = 16
        ress = []
        for i in range(0, len(texts), batch_size):
            res = self.client.embeddings.create(input=texts[i : i + batch_size], model=self.model_name)
            try:
                ress.extend([d.embedding for d in res.data])
            except Exception as _e:
                log_exception(_e, res)
        # local embedding for LmStudio donot count tokens
        return np.array(ress), 1024

    def encode_queries(self, text):
        embds, cnt = self.encode([text])
        return np.array(embds[0]), cnt


class AzureEmbed(OpenAIEmbed):
    _FACTORY_NAME = "Azure-OpenAI"

    def __init__(self, key, model_name, **kwargs):
        from openai.lib.azure import AzureOpenAI

        api_key = json.loads(key).get("api_key", "")
        api_version = json.loads(key).get("api_version", "2024-02-01")
        self.client = AzureOpenAI(api_key=api_key, azure_endpoint=kwargs["base_url"], api_version=api_version)
        self.model_name = model_name


class BaiChuanEmbed(OpenAIEmbed):
    _FACTORY_NAME = "BaiChuan"

    def __init__(self, key, model_name="Baichuan-Text-Embedding", base_url="https://api.baichuan-ai.com/v1"):
        if not base_url:
            base_url = "https://api.baichuan-ai.com/v1"
        super().__init__(key, model_name, base_url)


class QWenEmbed(Base):
    _FACTORY_NAME = "Tongyi-Qianwen"

    def __init__(self, key, model_name="text_embedding_v2", **kwargs):
        self.key = key
        self.model_name = model_name

    def encode(self, texts: list):
        import time

        import dashscope

        batch_size = 4
        res = []
        token_count = 0
        texts = [truncate(t, 2048) for t in texts]
        for i in range(0, len(texts), batch_size):
            retry_max = 5
            resp = dashscope.TextEmbedding.call(model=self.model_name, input=texts[i : i + batch_size], api_key=self.key, text_type="document")
            while (resp["output"] is None or resp["output"].get("embeddings") is None) and retry_max > 0:
                time.sleep(10)
                resp = dashscope.TextEmbedding.call(model=self.model_name, input=texts[i : i + batch_size], api_key=self.key, text_type="document")
                retry_max -= 1
            if retry_max == 0 and (resp["output"] is None or resp["output"].get("embeddings") is None):
                if resp.get("message"):
                    log_exception(ValueError(f"Retry_max reached, calling embedding model failed: {resp['message']}"))
                else:
                    log_exception(ValueError("Retry_max reached, calling embedding model failed"))
                raise
            try:
                embds = [[] for _ in range(len(resp["output"]["embeddings"]))]
                for e in resp["output"]["embeddings"]:
                    embds[e["text_index"]] = e["embedding"]
                res.extend(embds)
                token_count += self.total_token_count(resp)
            except Exception as _e:
                log_exception(_e, resp)
                raise
        return np.array(res), token_count

    def encode_queries(self, text):
        resp = dashscope.TextEmbedding.call(model=self.model_name, input=text[:2048], api_key=self.key, text_type="query")
        try:
            return np.array(resp["output"]["embeddings"][0]["embedding"]), self.total_token_count(resp)
        except Exception as _e:
            log_exception(_e, resp)


class ZhipuEmbed(Base):
    _FACTORY_NAME = "ZHIPU-AI"

    def __init__(self, key, model_name="embedding-2", **kwargs):
        self.client = ZhipuAI(api_key=key)
        self.model_name = model_name

    def encode(self, texts: list):
        arr = []
        tks_num = 0
        MAX_LEN = -1
        if self.model_name.lower() == "embedding-2":
            MAX_LEN = 512
        if self.model_name.lower() == "embedding-3":
            MAX_LEN = 3072
        if MAX_LEN > 0:
            texts = [truncate(t, MAX_LEN) for t in texts]

        for txt in texts:
            res = self.client.embeddings.create(input=txt, model=self.model_name)
            try:
                arr.append(res.data[0].embedding)
                tks_num += self.total_token_count(res)
            except Exception as _e:
                log_exception(_e, res)
        return np.array(arr), tks_num

    def encode_queries(self, text):
        res = self.client.embeddings.create(input=text, model=self.model_name)
        try:
            return np.array(res.data[0].embedding), self.total_token_count(res)
        except Exception as _e:
            log_exception(_e, res)


class OllamaEmbed(Base):
    _FACTORY_NAME = "Ollama"

    _special_tokens = ["<|endoftext|>"]

    def __init__(self, key, model_name, **kwargs):
        self.client = Client(host=kwargs["base_url"]) if not key or key == "x" else Client(host=kwargs["base_url"], headers={"Authorization": f"Bearer {key}"})
        self.model_name = model_name
        self.keep_alive = kwargs.get("ollama_keep_alive", int(os.environ.get("OLLAMA_KEEP_ALIVE", -1)))

    def encode(self, texts: list):
        arr = []
        tks_num = 0
        for txt in texts:
            # remove special tokens if they exist base on regex in one request
            for token in OllamaEmbed._special_tokens:
                txt = txt.replace(token, "")
            res = self.client.embeddings(prompt=txt, model=self.model_name, options={"use_mmap": True}, keep_alive=self.keep_alive)
            try:
                arr.append(res["embedding"])
            except Exception as _e:
                log_exception(_e, res)
            tks_num += 128
        return np.array(arr), tks_num

    def encode_queries(self, text):
        # remove special tokens if they exist
        for token in OllamaEmbed._special_tokens:
            text = text.replace(token, "")
        res = self.client.embeddings(prompt=text, model=self.model_name, options={"use_mmap": True}, keep_alive=self.keep_alive)
        try:
            return np.array(res["embedding"]), 128
        except Exception as _e:
            log_exception(_e, res)


class XinferenceEmbed(Base):
    _FACTORY_NAME = "Xinference"

    def __init__(self, key, model_name="", base_url=""):
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name

    def encode(self, texts: list):
        batch_size = 16
        ress = []
        total_tokens = 0
        for i in range(0, len(texts), batch_size):
            res = None
            try:
                res = self.client.embeddings.create(input=texts[i : i + batch_size], model=self.model_name)
                ress.extend([d.embedding for d in res.data])
                total_tokens += self.total_token_count(res)
            except Exception as _e:
                log_exception(_e, res)
        return np.array(ress), total_tokens

    def encode_queries(self, text):
        res = None
        try:
            res = self.client.embeddings.create(input=[text], model=self.model_name)
            return np.array(res.data[0].embedding), self.total_token_count(res)
        except Exception as _e:
            log_exception(_e, res)


class YoudaoEmbed(Base):
    _FACTORY_NAME = "Youdao"
    _client = None

    def __init__(self, key=None, model_name="maidalun1020/bce-embedding-base_v1", **kwargs):
        pass

    def encode(self, texts: list):
        batch_size = 10
        res = []
        token_count = 0
        for t in texts:
            token_count += num_tokens_from_string(t)
        for i in range(0, len(texts), batch_size):
            embds = YoudaoEmbed._client.encode(texts[i : i + batch_size])
            res.extend(embds)
        return np.array(res), token_count

    def encode_queries(self, text):
        embds = YoudaoEmbed._client.encode([text])
        return np.array(embds[0]), num_tokens_from_string(text)


class JinaEmbed(Base):
    _FACTORY_NAME = "Jina"

    def __init__(self, key, model_name="jina-embeddings-v3", base_url="https://api.jina.ai/v1/embeddings"):
        self.base_url = "https://api.jina.ai/v1/embeddings"
        self.headers = {"Content-Type": "application/json", "Authorization": f"Bearer {key}"}
        self.model_name = model_name

    def encode(self, texts: list):
        texts = [truncate(t, 8196) for t in texts]
        batch_size = 16
        ress = []
        token_count = 0
        for i in range(0, len(texts), batch_size):
            data = {"model": self.model_name, "input": texts[i : i + batch_size], "encoding_type": "float"}
            response = requests.post(self.base_url, headers=self.headers, json=data)
            try:
                res = response.json()
                ress.extend([d["embedding"] for d in res["data"]])
                token_count += self.total_token_count(res)
            except Exception as _e:
                log_exception(_e, response)
        return np.array(ress), token_count

    def encode_queries(self, text):
        embds, cnt = self.encode([text])
        return np.array(embds[0]), cnt


class JinaMultiVecEmbed(Base):
    _FACTORY_NAME = "Jina"

    def __init__(self, key, model_name="jina-embeddings-v4", base_url="https://api.jina.ai/v1/embeddings"):
        self.base_url = "https://api.jina.ai/v1/embeddings"
        self.headers = {"Content-Type": "application/json", "Authorization": f"Bearer {key}"}
        self.model_name = model_name

    def encode(self, texts: list[str|bytes], task="retrieval.passage"):
        batch_size = 16
        ress = []
        token_count = 0
        input = []
        for text in texts:
            if isinstance(text, str):
                input.append({"text": text})
            elif isinstance(text, bytes):
                img_b64s = None
                try:
                    base64.b64decode(text, validate=True)
                    img_b64s = text.decode('utf8')
                except Exception:
                    img_b64s = base64.b64encode(text).decode('utf8')
                input.append({"image": img_b64s})  # base64 encoded image
        for i in range(0, len(texts), batch_size):
            data = {"model": self.model_name, "task": task, "truncate": True, "return_multivector": True, "input": input[i : i + batch_size]}
            response = requests.post(self.base_url, headers=self.headers, json=data)
            try:
                res = response.json()
                ress.extend([d["embeddings"] for d in res["data"]])
                token_count += self.total_token_count(res)
            except Exception as _e:
                log_exception(_e, response)
        return np.array(ress), token_count

    def encode_queries(self, text):
        embds, cnt = self.encode([text], task="retrieval.query")
        return np.array(embds[0]), cnt


class MistralEmbed(Base):
    _FACTORY_NAME = "Mistral"

    def __init__(self, key, model_name="mistral-embed", base_url=None):
        from mistralai.client import MistralClient

        self.client = MistralClient(api_key=key)
        self.model_name = model_name

    def encode(self, texts: list):
        import time
        import random

        texts = [truncate(t, 8196) for t in texts]
        batch_size = 16
        ress = []
        token_count = 0
        for i in range(0, len(texts), batch_size):
            retry_max = 5
            while retry_max > 0:
                try:
                    res = self.client.embeddings(input=texts[i : i + batch_size], model=self.model_name)
                    ress.extend([d.embedding for d in res.data])
                    token_count += self.total_token_count(res)
                    break
                except Exception as _e:
                    if retry_max == 1:
                        log_exception(_e)
                    delay = random.uniform(20, 60)
                    time.sleep(delay)
                    retry_max -= 1
        return np.array(ress), token_count

    def encode_queries(self, text):
        import time
        import random
        retry_max = 5
        while retry_max > 0:
            try:
                res = self.client.embeddings(input=[truncate(text, 8196)], model=self.model_name)
                return np.array(res.data[0].embedding), self.total_token_count(res)
            except Exception as _e:
                if retry_max == 1:
                    log_exception(_e)
                delay = random.randint(20, 60)
                time.sleep(delay)
                retry_max -= 1


class BedrockEmbed(Base):
    _FACTORY_NAME = "Bedrock"

    def __init__(self, key, model_name, **kwargs):
        import boto3

        self.bedrock_ak = json.loads(key).get("bedrock_ak", "")
        self.bedrock_sk = json.loads(key).get("bedrock_sk", "")
        self.bedrock_region = json.loads(key).get("bedrock_region", "")
        self.model_name = model_name
        self.is_amazon = self.model_name.split(".")[0] == "amazon"
        self.is_cohere = self.model_name.split(".")[0] == "cohere"

        if self.bedrock_ak == "" or self.bedrock_sk == "" or self.bedrock_region == "":
            # Try to create a client using the default credentials (AWS_PROFILE, AWS_DEFAULT_REGION, etc.)
            self.client = boto3.client("bedrock-runtime")
        else:
            self.client = boto3.client(service_name="bedrock-runtime", region_name=self.bedrock_region, aws_access_key_id=self.bedrock_ak, aws_secret_access_key=self.bedrock_sk)

    def encode(self, texts: list):
        texts = [truncate(t, 8196) for t in texts]
        embeddings = []
        token_count = 0
        for text in texts:
            if self.is_amazon:
                body = {"inputText": text}
            elif self.is_cohere:
                body = {"texts": [text], "input_type": "search_document"}

            response = self.client.invoke_model(modelId=self.model_name, body=json.dumps(body))
            try:
                model_response = json.loads(response["body"].read())
                embeddings.extend([model_response["embedding"]])
                token_count += num_tokens_from_string(text)
            except Exception as _e:
                log_exception(_e, response)

        return np.array(embeddings), token_count

    def encode_queries(self, text):
        embeddings = []
        token_count = num_tokens_from_string(text)
        if self.is_amazon:
            body = {"inputText": truncate(text, 8196)}
        elif self.is_cohere:
            body = {"texts": [truncate(text, 8196)], "input_type": "search_query"}

        response = self.client.invoke_model(modelId=self.model_name, body=json.dumps(body))
        try:
            model_response = json.loads(response["body"].read())
            embeddings.extend(model_response["embedding"])
        except Exception as _e:
            log_exception(_e, response)

        return np.array(embeddings), token_count


class GeminiEmbed(Base):
    _FACTORY_NAME = "Gemini"

    def __init__(self, key, model_name="models/text-embedding-004", **kwargs):
        self.key = key
        self.model_name = "models/" + model_name

    def encode(self, texts: list):
        texts = [truncate(t, 2048) for t in texts]
        token_count = sum(num_tokens_from_string(text) for text in texts)
        genai.configure(api_key=self.key)
        batch_size = 16
        ress = []
        for i in range(0, len(texts), batch_size):
            result = genai.embed_content(model=self.model_name, content=texts[i : i + batch_size], task_type="retrieval_document", title="Embedding of single string")
            try:
                ress.extend(result["embedding"])
            except Exception as _e:
                log_exception(_e, result)
        return np.array(ress), token_count

    def encode_queries(self, text):
        genai.configure(api_key=self.key)
        result = genai.embed_content(model=self.model_name, content=truncate(text, 2048), task_type="retrieval_document", title="Embedding of single string")
        token_count = num_tokens_from_string(text)
        try:
            return np.array(result["embedding"]), token_count
        except Exception as _e:
            log_exception(_e, result)


class NvidiaEmbed(Base):
    _FACTORY_NAME = "NVIDIA"

    def __init__(self, key, model_name, base_url="https://integrate.api.nvidia.com/v1/embeddings"):
        if not base_url:
            base_url = "https://integrate.api.nvidia.com/v1/embeddings"
        self.api_key = key
        self.base_url = base_url
        self.headers = {
            "accept": "application/json",
            "Content-Type": "application/json",
            "authorization": f"Bearer {self.api_key}",
        }
        self.model_name = model_name
        if model_name == "nvidia/embed-qa-4":
            self.base_url = "https://ai.api.nvidia.com/v1/retrieval/nvidia/embeddings"
            self.model_name = "NV-Embed-QA"
        if model_name == "snowflake/arctic-embed-l":
            self.base_url = "https://ai.api.nvidia.com/v1/retrieval/snowflake/arctic-embed-l/embeddings"

    def encode(self, texts: list):
        batch_size = 16
        ress = []
        token_count = 0
        for i in range(0, len(texts), batch_size):
            payload = {
                "input": texts[i : i + batch_size],
                "input_type": "query",
                "model": self.model_name,
                "encoding_format": "float",
                "truncate": "END",
            }
            response = requests.post(self.base_url, headers=self.headers, json=payload)
            try:
                res = response.json()
            except Exception as _e:
                log_exception(_e, response)
            ress.extend([d["embedding"] for d in res["data"]])
            token_count += self.total_token_count(res)
        return np.array(ress), token_count

    def encode_queries(self, text):
        embds, cnt = self.encode([text])
        return np.array(embds[0]), cnt


class LmStudioEmbed(LocalAIEmbed):
    _FACTORY_NAME = "LM-Studio"

    def __init__(self, key, model_name, base_url):
        if not base_url:
            raise ValueError("Local llm url cannot be None")
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key="lm-studio", base_url=base_url)
        self.model_name = model_name


class OpenAI_APIEmbed(OpenAIEmbed):
    _FACTORY_NAME = ["VLLM", "OpenAI-API-Compatible"]

    def __init__(self, key, model_name, base_url):
        if not base_url:
            raise ValueError("url cannot be None")
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name.split("___")[0]


class CoHereEmbed(Base):
    _FACTORY_NAME = "Cohere"

    def __init__(self, key, model_name, base_url=None):
        from cohere import Client

        self.client = Client(api_key=key)
        self.model_name = model_name

    def encode(self, texts: list):
        batch_size = 16
        ress = []
        token_count = 0
        for i in range(0, len(texts), batch_size):
            res = self.client.embed(
                texts=texts[i : i + batch_size],
                model=self.model_name,
                input_type="search_document",
                embedding_types=["float"],
            )
            try:
                ress.extend([d for d in res.embeddings.float])
                token_count += res.meta.billed_units.input_tokens
            except Exception as _e:
                log_exception(_e, res)
        return np.array(ress), token_count

    def encode_queries(self, text):
        res = self.client.embed(
            texts=[text],
            model=self.model_name,
            input_type="search_query",
            embedding_types=["float"],
        )
        try:
            return np.array(res.embeddings.float[0]), int(res.meta.billed_units.input_tokens)
        except Exception as _e:
            log_exception(_e, res)


class TogetherAIEmbed(OpenAIEmbed):
    _FACTORY_NAME = "TogetherAI"

    def __init__(self, key, model_name, base_url="https://api.together.xyz/v1"):
        if not base_url:
            base_url = "https://api.together.xyz/v1"
        super().__init__(key, model_name, base_url=base_url)


class PerfXCloudEmbed(OpenAIEmbed):
    _FACTORY_NAME = "PerfXCloud"

    def __init__(self, key, model_name, base_url="https://cloud.perfxlab.cn/v1"):
        if not base_url:
            base_url = "https://cloud.perfxlab.cn/v1"
        super().__init__(key, model_name, base_url)


class UpstageEmbed(OpenAIEmbed):
    _FACTORY_NAME = "Upstage"

    def __init__(self, key, model_name, base_url="https://api.upstage.ai/v1/solar"):
        if not base_url:
            base_url = "https://api.upstage.ai/v1/solar"
        super().__init__(key, model_name, base_url)


class SILICONFLOWEmbed(Base):
    _FACTORY_NAME = "SILICONFLOW"

    def __init__(self, key, model_name, base_url="https://api.siliconflow.cn/v1/embeddings"):
        if not base_url:
            base_url = "https://api.siliconflow.cn/v1/embeddings"
        self.headers = {
            "accept": "application/json",
            "content-type": "application/json",
            "authorization": f"Bearer {key}",
        }
        self.base_url = base_url
        self.model_name = model_name

    def encode(self, texts: list):
        batch_size = 16
        ress = []
        token_count = 0
        for i in range(0, len(texts), batch_size):
            texts_batch = texts[i : i + batch_size]
            if self.model_name in ["BAAI/bge-large-zh-v1.5", "BAAI/bge-large-en-v1.5"]:
                # limit 512, 340 is almost safe
                texts_batch = [" " if not text.strip() else truncate(text, 256) for text in texts_batch]
            else:
                texts_batch = [" " if not text.strip() else text for text in texts_batch]

            payload = {
                "model": self.model_name,
                "input": texts_batch,
                "encoding_format": "float",
            }
            response = requests.post(self.base_url, json=payload, headers=self.headers)
            try:
                res = response.json()
                ress.extend([d["embedding"] for d in res["data"]])
                token_count += self.total_token_count(res)
            except Exception as _e:
                log_exception(_e, response)

        return np.array(ress), token_count

    def encode_queries(self, text):
        payload = {
            "model": self.model_name,
            "input": text,
            "encoding_format": "float",
        }
        response = requests.post(self.base_url, json=payload, headers=self.headers)
        try:
            res = response.json()
            return np.array(res["data"][0]["embedding"]), self.total_token_count(res)
        except Exception as _e:
            log_exception(_e, response)


class ReplicateEmbed(Base):
    _FACTORY_NAME = "Replicate"

    def __init__(self, key, model_name, base_url=None):
        from replicate.client import Client

        self.model_name = model_name
        self.client = Client(api_token=key)

    def encode(self, texts: list):
        batch_size = 16
        token_count = sum([num_tokens_from_string(text) for text in texts])
        ress = []
        for i in range(0, len(texts), batch_size):
            res = self.client.run(self.model_name, input={"texts": texts[i : i + batch_size]})
            ress.extend(res)
        return np.array(ress), token_count

    def encode_queries(self, text):
        res = self.client.embed(self.model_name, input={"texts": [text]})
        return np.array(res), num_tokens_from_string(text)


class BaiduYiyanEmbed(Base):
    _FACTORY_NAME = "BaiduYiyan"

    def __init__(self, key, model_name, base_url=None):
        import qianfan

        key = json.loads(key)
        ak = key.get("yiyan_ak", "")
        sk = key.get("yiyan_sk", "")
        self.client = qianfan.Embedding(ak=ak, sk=sk)
        self.model_name = model_name

    def encode(self, texts: list, batch_size=16):
        res = self.client.do(model=self.model_name, texts=texts).body
        try:
            return (
                np.array([r["embedding"] for r in res["data"]]),
                self.total_token_count(res),
            )
        except Exception as _e:
            log_exception(_e, res)

    def encode_queries(self, text):
        res = self.client.do(model=self.model_name, texts=[text]).body
        try:
            return (
                np.array([r["embedding"] for r in res["data"]]),
                self.total_token_count(res),
            )
        except Exception as _e:
            log_exception(_e, res)


class VoyageEmbed(Base):
    _FACTORY_NAME = "Voyage AI"

    def __init__(self, key, model_name, base_url=None):
        import voyageai

        self.client = voyageai.Client(api_key=key)
        self.model_name = model_name

    def encode(self, texts: list):
        batch_size = 16
        ress = []
        token_count = 0
        for i in range(0, len(texts), batch_size):
            res = self.client.embed(texts=texts[i : i + batch_size], model=self.model_name, input_type="document")
            try:
                ress.extend(res.embeddings)
                token_count += res.total_tokens
            except Exception as _e:
                log_exception(_e, res)
        return np.array(ress), token_count

    def encode_queries(self, text):
        res = self.client.embed(texts=text, model=self.model_name, input_type="query")
        try:
            return np.array(res.embeddings)[0], res.total_tokens
        except Exception as _e:
            log_exception(_e, res)


class HuggingFaceEmbed(Base):
    _FACTORY_NAME = "HuggingFace"

    def __init__(self, key, model_name, base_url=None, **kwargs):
        if not model_name:
            raise ValueError("Model name cannot be None")
        self.key = key
        self.model_name = model_name.split("___")[0]
        self.base_url = base_url or "http://127.0.0.1:8080"

    def encode(self, texts: list):
        response = requests.post(f"{self.base_url}/embed", json={"inputs": texts}, headers={"Content-Type": "application/json"})
        if response.status_code == 200:
            embeddings = response.json()
        else:
            raise Exception(f"Error: {response.status_code} - {response.text}")
        return np.array(embeddings), sum([num_tokens_from_string(text) for text in texts])

    def encode_queries(self, text: str):
        response = requests.post(f"{self.base_url}/embed", json={"inputs": text}, headers={"Content-Type": "application/json"})
        if response.status_code == 200:
            embedding = response.json()[0]
            return np.array(embedding), num_tokens_from_string(text)
        else:
            raise Exception(f"Error: {response.status_code} - {response.text}")


class VolcEngineEmbed(OpenAIEmbed):
    _FACTORY_NAME = "VolcEngine"

    def __init__(self, key, model_name, base_url="https://ark.cn-beijing.volces.com/api/v3"):
        if not base_url:
            base_url = "https://ark.cn-beijing.volces.com/api/v3"
        ark_api_key = json.loads(key).get("ark_api_key", "")
        model_name = json.loads(key).get("ep_id", "") + json.loads(key).get("endpoint_id", "")
        super().__init__(ark_api_key, model_name, base_url)


class GPUStackEmbed(OpenAIEmbed):
    _FACTORY_NAME = "GPUStack"

    def __init__(self, key, model_name, base_url):
        if not base_url:
            raise ValueError("url cannot be None")
        base_url = urljoin(base_url, "v1")

        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name


class NovitaEmbed(SILICONFLOWEmbed):
    _FACTORY_NAME = "NovitaAI"

    def __init__(self, key, model_name, base_url="https://api.novita.ai/v3/openai/embeddings"):
        if not base_url:
            base_url = "https://api.novita.ai/v3/openai/embeddings"
        super().__init__(key, model_name, base_url)


class GiteeEmbed(SILICONFLOWEmbed):
    _FACTORY_NAME = "GiteeAI"

    def __init__(self, key, model_name, base_url="https://ai.gitee.com/v1/embeddings"):
        if not base_url:
            base_url = "https://ai.gitee.com/v1/embeddings"
        super().__init__(key, model_name, base_url)

class DeepInfraEmbed(OpenAIEmbed):
    _FACTORY_NAME = "DeepInfra"

    def __init__(self, key, model_name, base_url="https://api.deepinfra.com/v1/openai"):
        if not base_url:
            base_url = "https://api.deepinfra.com/v1/openai"
        super().__init__(key, model_name, base_url)


class Ai302Embed(Base):
    _FACTORY_NAME = "302.AI"

    def __init__(self, key, model_name, base_url="https://api.302.ai/v1/embeddings"):
        if not base_url:
            base_url = "https://api.302.ai/v1/embeddings"
        super().__init__(key, model_name, base_url)


class CometAPIEmbed(OpenAIEmbed):
    _FACTORY_NAME = "CometAPI"

    def __init__(self, key, model_name, base_url="https://api.cometapi.com/v1"):
        if not base_url:
            base_url = "https://api.cometapi.com/v1"
        super().__init__(key, model_name, base_url)

class DeerAPIEmbed(OpenAIEmbed):
    _FACTORY_NAME = "DeerAPI"

    def __init__(self, key, model_name, base_url="https://api.deerapi.com/v1"):
        if not base_url:
            base_url = "https://api.deerapi.com/v1"
        super().__init__(key, model_name, base_url)


class JiekouAIEmbed(OpenAIEmbed):
    _FACTORY_NAME = "Jiekou.AI"

    def __init__(self, key, model_name, base_url="https://api.jiekou.ai/openai/v1/embeddings"):
        if not base_url:
            base_url = "https://api.jiekou.ai/openai/v1/embeddings"
        super().__init__(key, model_name, base_url)
