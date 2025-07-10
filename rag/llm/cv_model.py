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
import base64
import io
import json
import os
from abc import ABC
from io import BytesIO
from urllib.parse import urljoin

import requests
from ollama import Client
from openai import OpenAI
from openai.lib.azure import AzureOpenAI
from PIL import Image
from zhipuai import ZhipuAI

from api.utils import get_uuid
from api.utils.file_utils import get_project_base_directory
from rag.nlp import is_english
from rag.prompts import vision_llm_describe_prompt
from rag.utils import num_tokens_from_string


class Base(ABC):
    def __init__(self, key, model_name):
        pass

    def describe(self, image):
        raise NotImplementedError("Please implement encode method!")

    def describe_with_prompt(self, image, prompt=None):
        raise NotImplementedError("Please implement encode method!")

    def chat(self, system, history, gen_conf, image=""):
        if system:
            history[-1]["content"] = system + history[-1]["content"] + "user query: " + history[-1]["content"]
        try:
            for his in history:
                if his["role"] == "user":
                    his["content"] = self.chat_prompt(his["content"], image)

            response = self.client.chat.completions.create(
                model=self.model_name,
                messages=history,
                temperature=gen_conf.get("temperature", 0.3),
                top_p=gen_conf.get("top_p", 0.7),
            )
            return response.choices[0].message.content.strip(), response.usage.total_tokens
        except Exception as e:
            return "**ERROR**: " + str(e), 0

    def chat_streamly(self, system, history, gen_conf, image=""):
        if system:
            history[-1]["content"] = system + history[-1]["content"] + "user query: " + history[-1]["content"]

        ans = ""
        tk_count = 0
        try:
            for his in history:
                if his["role"] == "user":
                    his["content"] = self.chat_prompt(his["content"], image)

            response = self.client.chat.completions.create(
                model=self.model_name,
                messages=history,
                temperature=gen_conf.get("temperature", 0.3),
                top_p=gen_conf.get("top_p", 0.7),
                stream=True,
            )
            for resp in response:
                if not resp.choices[0].delta.content:
                    continue
                delta = resp.choices[0].delta.content
                ans += delta
                if resp.choices[0].finish_reason == "length":
                    ans += "...\nFor the content length reason, it stopped, continue?" if is_english([ans]) else "······\n由于长度的原因，回答被截断了，要继续吗？"
                    tk_count = resp.usage.total_tokens
                if resp.choices[0].finish_reason == "stop":
                    tk_count = resp.usage.total_tokens
                yield ans
        except Exception as e:
            yield ans + "\n**ERROR**: " + str(e)

        yield tk_count

    def image2base64(self, image):
        if isinstance(image, bytes):
            return base64.b64encode(image).decode("utf-8")
        if isinstance(image, BytesIO):
            return base64.b64encode(image.getvalue()).decode("utf-8")
        buffered = BytesIO()
        try:
            image.save(buffered, format="JPEG")
        except Exception:
            image.save(buffered, format="PNG")
        return base64.b64encode(buffered.getvalue()).decode("utf-8")

    def prompt(self, b64):
        return [
            {
                "role": "user",
                "content": [
                    {
                        "type": "image_url",
                        "image_url": {"url": f"data:image/jpeg;base64,{b64}"},
                    },
                    {
                        "text": "请用中文详细描述一下图中的内容，比如时间，地点，人物，事情，人物心情等，如果有数据请提取出数据。"
                        if self.lang.lower() == "chinese"
                        else "Please describe the content of this picture, like where, when, who, what happen. If it has number data, please extract them out.",
                    },
                ],
            }
        ]

    def vision_llm_prompt(self, b64, prompt=None):
        return [
            {
                "role": "user",
                "content": [
                    {
                        "type": "image_url",
                        "image_url": {"url": f"data:image/jpeg;base64,{b64}"},
                    },
                    {
                        "type": "text",
                        "text": prompt if prompt else vision_llm_describe_prompt(),
                    },
                ],
            }
        ]

    def chat_prompt(self, text, b64):
        return [
            {
                "type": "image_url",
                "image_url": {
                    "url": f"data:image/jpeg;base64,{b64}",
                },
            },
            {"type": "text", "text": text},
        ]


class GptV4(Base):
    _FACTORY_NAME = "OpenAI"

    def __init__(self, key, model_name="gpt-4-vision-preview", lang="Chinese", base_url="https://api.openai.com/v1"):
        if not base_url:
            base_url = "https://api.openai.com/v1"
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name
        self.lang = lang

    def describe(self, image):
        b64 = self.image2base64(image)
        prompt = self.prompt(b64)
        for i in range(len(prompt)):
            for c in prompt[i]["content"]:
                if "text" in c:
                    c["type"] = "text"

        res = self.client.chat.completions.create(
            model=self.model_name,
            messages=prompt,
        )
        return res.choices[0].message.content.strip(), res.usage.total_tokens

    def describe_with_prompt(self, image, prompt=None):
        b64 = self.image2base64(image)
        vision_prompt = self.vision_llm_prompt(b64, prompt) if prompt else self.vision_llm_prompt(b64)

        res = self.client.chat.completions.create(
            model=self.model_name,
            messages=vision_prompt,
        )
        return res.choices[0].message.content.strip(), res.usage.total_tokens


class AzureGptV4(Base):
    _FACTORY_NAME = "Azure-OpenAI"

    def __init__(self, key, model_name, lang="Chinese", **kwargs):
        api_key = json.loads(key).get("api_key", "")
        api_version = json.loads(key).get("api_version", "2024-02-01")
        self.client = AzureOpenAI(api_key=api_key, azure_endpoint=kwargs["base_url"], api_version=api_version)
        self.model_name = model_name
        self.lang = lang

    def describe(self, image):
        b64 = self.image2base64(image)
        prompt = self.prompt(b64)
        for i in range(len(prompt)):
            for c in prompt[i]["content"]:
                if "text" in c:
                    c["type"] = "text"

        res = self.client.chat.completions.create(model=self.model_name, messages=prompt)
        return res.choices[0].message.content.strip(), res.usage.total_tokens

    def describe_with_prompt(self, image, prompt=None):
        b64 = self.image2base64(image)
        vision_prompt = self.vision_llm_prompt(b64, prompt) if prompt else self.vision_llm_prompt(b64)

        res = self.client.chat.completions.create(
            model=self.model_name,
            messages=vision_prompt,
        )
        return res.choices[0].message.content.strip(), res.usage.total_tokens


class QWenCV(Base):
    _FACTORY_NAME = "Tongyi-Qianwen"

    def __init__(self, key, model_name="qwen-vl-chat-v1", lang="Chinese", **kwargs):
        import dashscope

        dashscope.api_key = key
        self.model_name = model_name
        self.lang = lang

    def prompt(self, binary):
        # stupid as hell
        tmp_dir = get_project_base_directory("tmp")
        if not os.path.exists(tmp_dir):
            os.makedirs(tmp_dir, exist_ok=True)
        path = os.path.join(tmp_dir, "%s.jpg" % get_uuid())
        Image.open(io.BytesIO(binary)).save(path)
        return [
            {
                "role": "user",
                "content": [
                    {"image": f"file://{path}"},
                    {
                        "text": "请用中文详细描述一下图中的内容，比如时间，地点，人物，事情，人物心情等，如果有数据请提取出数据。"
                        if self.lang.lower() == "chinese"
                        else "Please describe the content of this picture, like where, when, who, what happen. If it has number data, please extract them out.",
                    },
                ],
            }
        ]

    def vision_llm_prompt(self, binary, prompt=None):
        # stupid as hell
        tmp_dir = get_project_base_directory("tmp")
        if not os.path.exists(tmp_dir):
            os.makedirs(tmp_dir, exist_ok=True)
        path = os.path.join(tmp_dir, "%s.jpg" % get_uuid())
        Image.open(io.BytesIO(binary)).save(path)

        return [
            {
                "role": "user",
                "content": [
                    {"image": f"file://{path}"},
                    {
                        "text": prompt if prompt else vision_llm_describe_prompt(),
                    },
                ],
            }
        ]

    def chat_prompt(self, text, b64):
        return [
            {"image": f"{b64}"},
            {"text": text},
        ]

    def describe(self, image):
        from http import HTTPStatus

        from dashscope import MultiModalConversation

        response = MultiModalConversation.call(model=self.model_name, messages=self.prompt(image))
        if response.status_code == HTTPStatus.OK:
            return response.output.choices[0]["message"]["content"][0]["text"], response.usage.output_tokens
        return response.message, 0

    def describe_with_prompt(self, image, prompt=None):
        from http import HTTPStatus

        from dashscope import MultiModalConversation

        vision_prompt = self.vision_llm_prompt(image, prompt) if prompt else self.vision_llm_prompt(image)
        response = MultiModalConversation.call(model=self.model_name, messages=vision_prompt)
        if response.status_code == HTTPStatus.OK:
            return response.output.choices[0]["message"]["content"][0]["text"], response.usage.output_tokens
        return response.message, 0

    def chat(self, system, history, gen_conf, image=""):
        from http import HTTPStatus

        from dashscope import MultiModalConversation

        if system:
            history[-1]["content"] = system + history[-1]["content"] + "user query: " + history[-1]["content"]

        for his in history:
            if his["role"] == "user":
                his["content"] = self.chat_prompt(his["content"], image)
        response = MultiModalConversation.call(
            model=self.model_name,
            messages=history,
            temperature=gen_conf.get("temperature", 0.3),
            top_p=gen_conf.get("top_p", 0.7),
        )

        ans = ""
        tk_count = 0
        if response.status_code == HTTPStatus.OK:
            ans = response.output.choices[0]["message"]["content"]
            if isinstance(ans, list):
                ans = ans[0]["text"] if ans else ""
            tk_count += response.usage.total_tokens
            if response.output.choices[0].get("finish_reason", "") == "length":
                ans += "...\nFor the content length reason, it stopped, continue?" if is_english([ans]) else "······\n由于长度的原因，回答被截断了，要继续吗？"
            return ans, tk_count

        return "**ERROR**: " + response.message, tk_count

    def chat_streamly(self, system, history, gen_conf, image=""):
        from http import HTTPStatus

        from dashscope import MultiModalConversation

        if system:
            history[-1]["content"] = system + history[-1]["content"] + "user query: " + history[-1]["content"]

        for his in history:
            if his["role"] == "user":
                his["content"] = self.chat_prompt(his["content"], image)

        ans = ""
        tk_count = 0
        try:
            response = MultiModalConversation.call(
                model=self.model_name,
                messages=history,
                temperature=gen_conf.get("temperature", 0.3),
                top_p=gen_conf.get("top_p", 0.7),
                stream=True,
            )
            for resp in response:
                if resp.status_code == HTTPStatus.OK:
                    cnt = resp.output.choices[0]["message"]["content"]
                    if isinstance(cnt, list):
                        cnt = cnt[0]["text"] if ans else ""
                    ans += cnt
                    tk_count = resp.usage.total_tokens
                    if resp.output.choices[0].get("finish_reason", "") == "length":
                        ans += "...\nFor the content length reason, it stopped, continue?" if is_english([ans]) else "······\n由于长度的原因，回答被截断了，要继续吗？"
                    yield ans
                else:
                    yield ans + "\n**ERROR**: " + resp.message if str(resp.message).find("Access") < 0 else "Out of credit. Please set the API key in **settings > Model providers.**"
        except Exception as e:
            yield ans + "\n**ERROR**: " + str(e)

        yield tk_count


class Zhipu4V(Base):
    _FACTORY_NAME = "ZHIPU-AI"

    def __init__(self, key, model_name="glm-4v", lang="Chinese", **kwargs):
        self.client = ZhipuAI(api_key=key)
        self.model_name = model_name
        self.lang = lang

    def describe(self, image):
        b64 = self.image2base64(image)

        prompt = self.prompt(b64)
        prompt[0]["content"][1]["type"] = "text"

        res = self.client.chat.completions.create(
            model=self.model_name,
            messages=prompt,
        )
        return res.choices[0].message.content.strip(), res.usage.total_tokens

    def describe_with_prompt(self, image, prompt=None):
        b64 = self.image2base64(image)
        vision_prompt = self.vision_llm_prompt(b64, prompt) if prompt else self.vision_llm_prompt(b64)

        res = self.client.chat.completions.create(model=self.model_name, messages=vision_prompt)
        return res.choices[0].message.content.strip(), res.usage.total_tokens

    def chat(self, system, history, gen_conf, image=""):
        if system:
            history[-1]["content"] = system + history[-1]["content"] + "user query: " + history[-1]["content"]
        try:
            for his in history:
                if his["role"] == "user":
                    his["content"] = self.chat_prompt(his["content"], image)

            response = self.client.chat.completions.create(
                model=self.model_name,
                messages=history,
                temperature=gen_conf.get("temperature", 0.3),
                top_p=gen_conf.get("top_p", 0.7),
            )
            return response.choices[0].message.content.strip(), response.usage.total_tokens
        except Exception as e:
            return "**ERROR**: " + str(e), 0

    def chat_streamly(self, system, history, gen_conf, image=""):
        if system:
            history[-1]["content"] = system + history[-1]["content"] + "user query: " + history[-1]["content"]

        ans = ""
        tk_count = 0
        try:
            for his in history:
                if his["role"] == "user":
                    his["content"] = self.chat_prompt(his["content"], image)

            response = self.client.chat.completions.create(
                model=self.model_name,
                messages=history,
                temperature=gen_conf.get("temperature", 0.3),
                top_p=gen_conf.get("top_p", 0.7),
                stream=True,
            )
            for resp in response:
                if not resp.choices[0].delta.content:
                    continue
                delta = resp.choices[0].delta.content
                ans += delta
                if resp.choices[0].finish_reason == "length":
                    ans += "...\nFor the content length reason, it stopped, continue?" if is_english([ans]) else "······\n由于长度的原因，回答被截断了，要继续吗？"
                    tk_count = resp.usage.total_tokens
                if resp.choices[0].finish_reason == "stop":
                    tk_count = resp.usage.total_tokens
                yield ans
        except Exception as e:
            yield ans + "\n**ERROR**: " + str(e)

        yield tk_count


class OllamaCV(Base):
    _FACTORY_NAME = "Ollama"

    def __init__(self, key, model_name, lang="Chinese", **kwargs):
        self.client = Client(host=kwargs["base_url"])
        self.model_name = model_name
        self.lang = lang

    def describe(self, image):
        prompt = self.prompt("")
        try:
            response = self.client.generate(
                model=self.model_name,
                prompt=prompt[0]["content"][1]["text"],
                images=[image],
            )
            ans = response["response"].strip()
            return ans, 128
        except Exception as e:
            return "**ERROR**: " + str(e), 0

    def describe_with_prompt(self, image, prompt=None):
        vision_prompt = self.vision_llm_prompt("", prompt) if prompt else self.vision_llm_prompt("")
        try:
            response = self.client.generate(
                model=self.model_name,
                prompt=vision_prompt[0]["content"][1]["text"],
                images=[image],
            )
            ans = response["response"].strip()
            return ans, 128
        except Exception as e:
            return "**ERROR**: " + str(e), 0

    def chat(self, system, history, gen_conf, image=""):
        if system:
            history[-1]["content"] = system + history[-1]["content"] + "user query: " + history[-1]["content"]

        try:
            for his in history:
                if his["role"] == "user":
                    his["images"] = [image]
            options = {}
            if "temperature" in gen_conf:
                options["temperature"] = gen_conf["temperature"]
            if "top_p" in gen_conf:
                options["top_k"] = gen_conf["top_p"]
            if "presence_penalty" in gen_conf:
                options["presence_penalty"] = gen_conf["presence_penalty"]
            if "frequency_penalty" in gen_conf:
                options["frequency_penalty"] = gen_conf["frequency_penalty"]
            response = self.client.chat(
                model=self.model_name,
                messages=history,
                options=options,
                keep_alive=-1,
            )

            ans = response["message"]["content"].strip()
            return ans, response["eval_count"] + response.get("prompt_eval_count", 0)
        except Exception as e:
            return "**ERROR**: " + str(e), 0

    def chat_streamly(self, system, history, gen_conf, image=""):
        if system:
            history[-1]["content"] = system + history[-1]["content"] + "user query: " + history[-1]["content"]

        for his in history:
            if his["role"] == "user":
                his["images"] = [image]
        options = {}
        if "temperature" in gen_conf:
            options["temperature"] = gen_conf["temperature"]
        if "top_p" in gen_conf:
            options["top_k"] = gen_conf["top_p"]
        if "presence_penalty" in gen_conf:
            options["presence_penalty"] = gen_conf["presence_penalty"]
        if "frequency_penalty" in gen_conf:
            options["frequency_penalty"] = gen_conf["frequency_penalty"]
        ans = ""
        try:
            response = self.client.chat(
                model=self.model_name,
                messages=history,
                stream=True,
                options=options,
                keep_alive=-1,
            )
            for resp in response:
                if resp["done"]:
                    yield resp.get("prompt_eval_count", 0) + resp.get("eval_count", 0)
                ans += resp["message"]["content"]
                yield ans
        except Exception as e:
            yield ans + "\n**ERROR**: " + str(e)
        yield 0


class LocalAICV(GptV4):
    _FACTORY_NAME = "LocalAI"

    def __init__(self, key, model_name, base_url, lang="Chinese"):
        if not base_url:
            raise ValueError("Local cv model url cannot be None")
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key="empty", base_url=base_url)
        self.model_name = model_name.split("___")[0]
        self.lang = lang


class XinferenceCV(Base):
    _FACTORY_NAME = "Xinference"

    def __init__(self, key, model_name="", lang="Chinese", base_url=""):
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name
        self.lang = lang

    def describe(self, image):
        b64 = self.image2base64(image)

        res = self.client.chat.completions.create(model=self.model_name, messages=self.prompt(b64))
        return res.choices[0].message.content.strip(), res.usage.total_tokens

    def describe_with_prompt(self, image, prompt=None):
        b64 = self.image2base64(image)
        vision_prompt = self.vision_llm_prompt(b64, prompt) if prompt else self.vision_llm_prompt(b64)

        res = self.client.chat.completions.create(
            model=self.model_name,
            messages=vision_prompt,
        )
        return res.choices[0].message.content.strip(), res.usage.total_tokens


class GeminiCV(Base):
    _FACTORY_NAME = "Gemini"

    def __init__(self, key, model_name="gemini-1.0-pro-vision-latest", lang="Chinese", **kwargs):
        from google.generativeai import GenerativeModel, client

        client.configure(api_key=key)
        _client = client.get_default_generative_client()
        self.model_name = model_name
        self.model = GenerativeModel(model_name=self.model_name)
        self.model._client = _client
        self.lang = lang

    def describe(self, image):
        from PIL.Image import open

        prompt = (
            "请用中文详细描述一下图中的内容，比如时间，地点，人物，事情，人物心情等，如果有数据请提取出数据。"
            if self.lang.lower() == "chinese"
            else "Please describe the content of this picture, like where, when, who, what happen. If it has number data, please extract them out."
        )
        b64 = self.image2base64(image)
        img = open(BytesIO(base64.b64decode(b64)))
        input = [prompt, img]
        res = self.model.generate_content(input)
        return res.text, res.usage_metadata.total_token_count

    def describe_with_prompt(self, image, prompt=None):
        from PIL.Image import open

        b64 = self.image2base64(image)
        vision_prompt = self.vision_llm_prompt(b64, prompt) if prompt else self.vision_llm_prompt(b64)
        img = open(BytesIO(base64.b64decode(b64)))
        input = [vision_prompt, img]
        res = self.model.generate_content(
            input,
        )
        return res.text, res.usage_metadata.total_token_count

    def chat(self, system, history, gen_conf, image=""):
        from transformers import GenerationConfig

        if system:
            history[-1]["content"] = system + history[-1]["content"] + "user query: " + history[-1]["content"]
        try:
            for his in history:
                if his["role"] == "assistant":
                    his["role"] = "model"
                    his["parts"] = [his["content"]]
                    his.pop("content")
                if his["role"] == "user":
                    his["parts"] = [his["content"]]
                    his.pop("content")
            history[-1]["parts"].append("data:image/jpeg;base64," + image)

            response = self.model.generate_content(history, generation_config=GenerationConfig(temperature=gen_conf.get("temperature", 0.3), top_p=gen_conf.get("top_p", 0.7)))

            ans = response.text
            return ans, response.usage_metadata.total_token_count
        except Exception as e:
            return "**ERROR**: " + str(e), 0

    def chat_streamly(self, system, history, gen_conf, image=""):
        from transformers import GenerationConfig

        if system:
            history[-1]["content"] = system + history[-1]["content"] + "user query: " + history[-1]["content"]

        ans = ""
        try:
            for his in history:
                if his["role"] == "assistant":
                    his["role"] = "model"
                    his["parts"] = [his["content"]]
                    his.pop("content")
                if his["role"] == "user":
                    his["parts"] = [his["content"]]
                    his.pop("content")
            history[-1]["parts"].append("data:image/jpeg;base64," + image)

            response = self.model.generate_content(
                history,
                generation_config=GenerationConfig(temperature=gen_conf.get("temperature", 0.3), top_p=gen_conf.get("top_p", 0.7)),
                stream=True,
            )

            for resp in response:
                if not resp.text:
                    continue
                ans += resp.text
                yield ans
        except Exception as e:
            yield ans + "\n**ERROR**: " + str(e)

        yield response._chunks[-1].usage_metadata.total_token_count


class OpenRouterCV(GptV4):
    _FACTORY_NAME = "OpenRouter"

    def __init__(
        self,
        key,
        model_name,
        lang="Chinese",
        base_url="https://openrouter.ai/api/v1",
    ):
        if not base_url:
            base_url = "https://openrouter.ai/api/v1"
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name
        self.lang = lang


class LocalCV(Base):
    _FACTORY_NAME = "Moonshot"

    def __init__(self, key, model_name="glm-4v", lang="Chinese", **kwargs):
        pass

    def describe(self, image):
        return "", 0


class NvidiaCV(Base):
    _FACTORY_NAME = "NVIDIA"

    def __init__(
        self,
        key,
        model_name,
        lang="Chinese",
        base_url="https://ai.api.nvidia.com/v1/vlm",
    ):
        if not base_url:
            base_url = ("https://ai.api.nvidia.com/v1/vlm",)
        self.lang = lang
        factory, llm_name = model_name.split("/")
        if factory != "liuhaotian":
            self.base_url = urljoin(base_url, f"{factory}/{llm_name}")
        else:
            self.base_url = urljoin(f"{base_url}/community", llm_name.replace("-v1.6", "16"))
        self.key = key

    def describe(self, image):
        b64 = self.image2base64(image)
        response = requests.post(
            url=self.base_url,
            headers={
                "accept": "application/json",
                "content-type": "application/json",
                "Authorization": f"Bearer {self.key}",
            },
            json={"messages": self.prompt(b64)},
        )
        response = response.json()
        return (
            response["choices"][0]["message"]["content"].strip(),
            response["usage"]["total_tokens"],
        )

    def describe_with_prompt(self, image, prompt=None):
        b64 = self.image2base64(image)
        vision_prompt = self.vision_llm_prompt(b64, prompt) if prompt else self.vision_llm_prompt(b64)

        response = requests.post(
            url=self.base_url,
            headers={
                "accept": "application/json",
                "content-type": "application/json",
                "Authorization": f"Bearer {self.key}",
            },
            json={
                "messages": vision_prompt,
            },
        )
        response = response.json()
        return (
            response["choices"][0]["message"]["content"].strip(),
            response["usage"]["total_tokens"],
        )

    def prompt(self, b64):
        return [
            {
                "role": "user",
                "content": (
                    "请用中文详细描述一下图中的内容，比如时间，地点，人物，事情，人物心情等，如果有数据请提取出数据。"
                    if self.lang.lower() == "chinese"
                    else "Please describe the content of this picture, like where, when, who, what happen. If it has number data, please extract them out."
                )
                + f' <img src="data:image/jpeg;base64,{b64}"/>',
            }
        ]

    def vision_llm_prompt(self, b64, prompt=None):
        return [
            {
                "role": "user",
                "content": (prompt if prompt else vision_llm_describe_prompt()) + f' <img src="data:image/jpeg;base64,{b64}"/>',
            }
        ]

    def chat_prompt(self, text, b64):
        return [
            {
                "role": "user",
                "content": text + f' <img src="data:image/jpeg;base64,{b64}"/>',
            }
        ]


class StepFunCV(GptV4):
    _FACTORY_NAME = "StepFun"

    def __init__(self, key, model_name="step-1v-8k", lang="Chinese", base_url="https://api.stepfun.com/v1"):
        if not base_url:
            base_url = "https://api.stepfun.com/v1"
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name
        self.lang = lang


class LmStudioCV(GptV4):
    _FACTORY_NAME = "LM-Studio"

    def __init__(self, key, model_name, lang="Chinese", base_url=""):
        if not base_url:
            raise ValueError("Local llm url cannot be None")
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key="lm-studio", base_url=base_url)
        self.model_name = model_name
        self.lang = lang


class OpenAI_APICV(GptV4):
    _FACTORY_NAME = ["VLLM", "OpenAI-API-Compatible"]

    def __init__(self, key, model_name, lang="Chinese", base_url=""):
        if not base_url:
            raise ValueError("url cannot be None")
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name.split("___")[0]
        self.lang = lang


class TogetherAICV(GptV4):
    _FACTORY_NAME = "TogetherAI"

    def __init__(self, key, model_name, lang="Chinese", base_url="https://api.together.xyz/v1"):
        if not base_url:
            base_url = "https://api.together.xyz/v1"
        super().__init__(key, model_name, lang, base_url)


class YiCV(GptV4):
    _FACTORY_NAME = "01.AI"

    def __init__(
        self,
        key,
        model_name,
        lang="Chinese",
        base_url="https://api.lingyiwanwu.com/v1",
    ):
        if not base_url:
            base_url = "https://api.lingyiwanwu.com/v1"
        super().__init__(key, model_name, lang, base_url)


class SILICONFLOWCV(GptV4):
    _FACTORY_NAME = "SILICONFLOW"

    def __init__(
        self,
        key,
        model_name,
        lang="Chinese",
        base_url="https://api.siliconflow.cn/v1",
    ):
        if not base_url:
            base_url = "https://api.siliconflow.cn/v1"
        super().__init__(key, model_name, lang, base_url)


class HunyuanCV(Base):
    _FACTORY_NAME = "Tencent Hunyuan"

    def __init__(self, key, model_name, lang="Chinese", base_url=None):
        from tencentcloud.common import credential
        from tencentcloud.hunyuan.v20230901 import hunyuan_client

        key = json.loads(key)
        sid = key.get("hunyuan_sid", "")
        sk = key.get("hunyuan_sk", "")
        cred = credential.Credential(sid, sk)
        self.model_name = model_name
        self.client = hunyuan_client.HunyuanClient(cred, "")
        self.lang = lang

    def describe(self, image):
        from tencentcloud.common.exception.tencent_cloud_sdk_exception import (
            TencentCloudSDKException,
        )
        from tencentcloud.hunyuan.v20230901 import models

        b64 = self.image2base64(image)
        req = models.ChatCompletionsRequest()
        params = {"Model": self.model_name, "Messages": self.prompt(b64)}
        req.from_json_string(json.dumps(params))
        ans = ""
        try:
            response = self.client.ChatCompletions(req)
            ans = response.Choices[0].Message.Content
            return ans, response.Usage.TotalTokens
        except TencentCloudSDKException as e:
            return ans + "\n**ERROR**: " + str(e), 0

    def describe_with_prompt(self, image, prompt=None):
        from tencentcloud.common.exception.tencent_cloud_sdk_exception import TencentCloudSDKException
        from tencentcloud.hunyuan.v20230901 import models

        b64 = self.image2base64(image)
        vision_prompt = self.vision_llm_prompt(b64, prompt) if prompt else self.vision_llm_prompt(b64)
        req = models.ChatCompletionsRequest()
        params = {"Model": self.model_name, "Messages": vision_prompt}
        req.from_json_string(json.dumps(params))
        ans = ""
        try:
            response = self.client.ChatCompletions(req)
            ans = response.Choices[0].Message.Content
            return ans, response.Usage.TotalTokens
        except TencentCloudSDKException as e:
            return ans + "\n**ERROR**: " + str(e), 0

    def prompt(self, b64):
        return [
            {
                "Role": "user",
                "Contents": [
                    {
                        "Type": "image_url",
                        "ImageUrl": {"Url": f"data:image/jpeg;base64,{b64}"},
                    },
                    {
                        "Type": "text",
                        "Text": "请用中文详细描述一下图中的内容，比如时间，地点，人物，事情，人物心情等，如果有数据请提取出数据。"
                        if self.lang.lower() == "chinese"
                        else "Please describe the content of this picture, like where, when, who, what happen. If it has number data, please extract them out.",
                    },
                ],
            }
        ]


class AnthropicCV(Base):
    _FACTORY_NAME = "Anthropic"

    def __init__(self, key, model_name, base_url=None):
        import anthropic

        self.client = anthropic.Anthropic(api_key=key)
        self.model_name = model_name
        self.system = ""
        self.max_tokens = 8192
        if "haiku" in self.model_name or "opus" in self.model_name:
            self.max_tokens = 4096

    def prompt(self, b64, prompt):
        return [
            {
                "role": "user",
                "content": [
                    {
                        "type": "image",
                        "source": {
                            "type": "base64",
                            "media_type": "image/jpeg",
                            "data": b64,
                        },
                    },
                    {"type": "text", "text": prompt},
                ],
            }
        ]

    def describe(self, image):
        b64 = self.image2base64(image)
        prompt = self.prompt(
            b64,
            "请用中文详细描述一下图中的内容，比如时间，地点，人物，事情，人物心情等，如果有数据请提取出数据。"
            if self.lang.lower() == "chinese"
            else "Please describe the content of this picture, like where, when, who, what happen. If it has number data, please extract them out.",
        )

        response = self.client.messages.create(model=self.model_name, max_tokens=self.max_tokens, messages=prompt)
        return response["content"][0]["text"].strip(), response["usage"]["input_tokens"] + response["usage"]["output_tokens"]

    def describe_with_prompt(self, image, prompt=None):
        b64 = self.image2base64(image)
        prompt = self.prompt(b64, prompt if prompt else vision_llm_describe_prompt())

        response = self.client.messages.create(model=self.model_name, max_tokens=self.max_tokens, messages=prompt)
        return response["content"][0]["text"].strip(), response["usage"]["input_tokens"] + response["usage"]["output_tokens"]

    def chat(self, system, history, gen_conf):
        if "presence_penalty" in gen_conf:
            del gen_conf["presence_penalty"]
        if "frequency_penalty" in gen_conf:
            del gen_conf["frequency_penalty"]
        gen_conf["max_tokens"] = self.max_tokens

        ans = ""
        try:
            response = self.client.messages.create(
                model=self.model_name,
                messages=history,
                system=system,
                stream=False,
                **gen_conf,
            ).to_dict()
            ans = response["content"][0]["text"]
            if response["stop_reason"] == "max_tokens":
                ans += "...\nFor the content length reason, it stopped, continue?" if is_english([ans]) else "······\n由于长度的原因，回答被截断了，要继续吗？"
            return (
                ans,
                response["usage"]["input_tokens"] + response["usage"]["output_tokens"],
            )
        except Exception as e:
            return ans + "\n**ERROR**: " + str(e), 0

    def chat_streamly(self, system, history, gen_conf):
        if "presence_penalty" in gen_conf:
            del gen_conf["presence_penalty"]
        if "frequency_penalty" in gen_conf:
            del gen_conf["frequency_penalty"]
        gen_conf["max_tokens"] = self.max_tokens

        ans = ""
        total_tokens = 0
        try:
            response = self.client.messages.create(
                model=self.model_name,
                messages=history,
                system=system,
                stream=True,
                **gen_conf,
            )
            for res in response:
                if res.type == "content_block_delta":
                    if res.delta.type == "thinking_delta" and res.delta.thinking:
                        if ans.find("<think>") < 0:
                            ans += "<think>"
                        ans = ans.replace("</think>", "")
                        ans += res.delta.thinking + "</think>"
                    else:
                        text = res.delta.text
                        ans += text
                        total_tokens += num_tokens_from_string(text)
                    yield ans
        except Exception as e:
            yield ans + "\n**ERROR**: " + str(e)

        yield total_tokens


class GPUStackCV(GptV4):
    _FACTORY_NAME = "GPUStack"

    def __init__(self, key, model_name, lang="Chinese", base_url=""):
        if not base_url:
            raise ValueError("Local llm url cannot be None")
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name
        self.lang = lang


class GoogleCV(Base):
    _FACTORY_NAME = "Google Cloud"

    def __init__(self, key, model_name, lang="Chinese", base_url=None, **kwargs):
        import base64

        from google.oauth2 import service_account

        key = json.loads(key)
        access_token = json.loads(base64.b64decode(key.get("google_service_account_key", "")))
        project_id = key.get("google_project_id", "")
        region = key.get("google_region", "")

        scopes = ["https://www.googleapis.com/auth/cloud-platform"]
        self.model_name = model_name
        self.lang = lang

        if "claude" in self.model_name:
            from anthropic import AnthropicVertex
            from google.auth.transport.requests import Request

            if access_token:
                credits = service_account.Credentials.from_service_account_info(access_token, scopes=scopes)
                request = Request()
                credits.refresh(request)
                token = credits.token
                self.client = AnthropicVertex(region=region, project_id=project_id, access_token=token)
            else:
                self.client = AnthropicVertex(region=region, project_id=project_id)
        else:
            import vertexai.generative_models as glm
            from google.cloud import aiplatform

            if access_token:
                credits = service_account.Credentials.from_service_account_info(access_token)
                aiplatform.init(credentials=credits, project=project_id, location=region)
            else:
                aiplatform.init(project=project_id, location=region)
            self.client = glm.GenerativeModel(model_name=self.model_name)

    def describe(self, image):
        prompt = (
            "请用中文详细描述一下图中的内容，比如时间，地点，人物，事情，人物心情等，如果有数据请提取出数据。"
            if self.lang.lower() == "chinese"
            else "Please describe the content of this picture, like where, when, who, what happen. If it has number data, please extract them out."
        )

        if "claude" in self.model_name:
            b64 = self.image2base64(image)
            vision_prompt = [
                {
                    "role": "user",
                    "content": [
                        {
                            "type": "image",
                            "source": {
                                "type": "base64",
                                "media_type": "image/jpeg",
                                "data": b64,
                            },
                        },
                        {"type": "text", "text": prompt},
                    ],
                }
            ]
            response = self.client.messages.create(
                model=self.model_name,
                max_tokens=8192,
                messages=vision_prompt,
            )
            return response.content[0].text.strip(), response.usage.input_tokens + response.usage.output_tokens
        else:
            import vertexai.generative_models as glm

            b64 = self.image2base64(image)
            # Create proper image part for Gemini
            image_part = glm.Part.from_data(data=base64.b64decode(b64), mime_type="image/jpeg")
            input = [prompt, image_part]
            res = self.client.generate_content(input)
            return res.text, res.usage_metadata.total_token_count

    def describe_with_prompt(self, image, prompt=None):
        if "claude" in self.model_name:
            b64 = self.image2base64(image)
            vision_prompt = [
                {
                    "role": "user",
                    "content": [
                        {
                            "type": "image",
                            "source": {
                                "type": "base64",
                                "media_type": "image/jpeg",
                                "data": b64,
                            },
                        },
                        {"type": "text", "text": prompt if prompt else vision_llm_describe_prompt()},
                    ],
                }
            ]
            response = self.client.messages.create(model=self.model_name, max_tokens=8192, messages=vision_prompt)
            return response.content[0].text.strip(), response.usage.input_tokens + response.usage.output_tokens
        else:
            import vertexai.generative_models as glm

            b64 = self.image2base64(image)
            vision_prompt = prompt if prompt else vision_llm_describe_prompt()
            # Create proper image part for Gemini
            image_part = glm.Part.from_data(data=base64.b64decode(b64), mime_type="image/jpeg")
            input = [vision_prompt, image_part]
            res = self.client.generate_content(input)
            return res.text, res.usage_metadata.total_token_count

    def chat(self, system, history, gen_conf, image=""):
        if "claude" in self.model_name:
            if system:
                history[-1]["content"] = system + history[-1]["content"] + "user query: " + history[-1]["content"]
            try:
                for his in history:
                    if his["role"] == "user":
                        his["content"] = [
                            {
                                "type": "image",
                                "source": {
                                    "type": "base64",
                                    "media_type": "image/jpeg",
                                    "data": image,
                                },
                            },
                            {"type": "text", "text": his["content"]},
                        ]

                response = self.client.messages.create(model=self.model_name, max_tokens=8192, messages=history, temperature=gen_conf.get("temperature", 0.3), top_p=gen_conf.get("top_p", 0.7))
                return response.content[0].text.strip(), response.usage.input_tokens + response.usage.output_tokens
            except Exception as e:
                return "**ERROR**: " + str(e), 0
        else:
            import vertexai.generative_models as glm
            from transformers import GenerationConfig

            if system:
                history[-1]["content"] = system + history[-1]["content"] + "user query: " + history[-1]["content"]
            try:
                for his in history:
                    if his["role"] == "assistant":
                        his["role"] = "model"
                        his["parts"] = [his["content"]]
                        his.pop("content")
                    if his["role"] == "user":
                        his["parts"] = [his["content"]]
                        his.pop("content")

                # Create proper image part for Gemini
                img_bytes = base64.b64decode(image)
                image_part = glm.Part.from_data(data=img_bytes, mime_type="image/jpeg")
                history[-1]["parts"].append(image_part)

                response = self.client.generate_content(history, generation_config=GenerationConfig(temperature=gen_conf.get("temperature", 0.3), top_p=gen_conf.get("top_p", 0.7)))

                ans = response.text
                return ans, response.usage_metadata.total_token_count
            except Exception as e:
                return "**ERROR**: " + str(e), 0
