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
from openai.lib.azure import AzureOpenAI
from zhipuai import ZhipuAI
import io
from abc import ABC
from ollama import Client
from PIL import Image
from openai import OpenAI
import os
import base64
from io import BytesIO
import json
import requests

from rag.nlp import is_english
from api.utils import get_uuid
from api.utils.file_utils import get_project_base_directory


class Base(ABC):
    def __init__(self, key, model_name):
        pass

    def describe(self, image, max_tokens=300):
        raise NotImplementedError("Please implement encode method!")
        
    def chat(self, system, history, gen_conf):
        raise NotImplementedError("Please implement encode method!")

    def chat_streamly(self, system, history, gen_conf):
        raise NotImplementedError("Please implement encode method!")
        
    def image2base64(self, image):
        if isinstance(image, bytes):
            return base64.b64encode(image).decode("utf-8")
        if isinstance(image, BytesIO):
            return base64.b64encode(image.getvalue()).decode("utf-8")
        buffered = BytesIO()
        try:
            image.save(buffered, format="JPEG")
        except Exception as e:
            image.save(buffered, format="PNG")
        return base64.b64encode(buffered.getvalue()).decode("utf-8")

    def prompt(self, b64):
        return [
            {
                "role": "user",
                "content": [
                    {
                        "type": "image_url",
                        "image_url": {
                            "url": f"data:image/jpeg;base64,{b64}"
                        },
                    },
                    {
                        "text": "请用中文详细描述一下图中的内容，比如时间，地点，人物，事情，人物心情等，如果有数据请提取出数据。" if self.lang.lower() == "chinese" else
                        "Please describe the content of this picture, like where, when, who, what happen. If it has number data, please extract them out.",
                    },
                ],
            }
        ]

    def chat_prompt(self, text, b64):
        return [
            {
                "type": "image_url",
                "image_url": f"{b64}"
            },
            {
                "type": "text",
                "text": text
            },
        ]



class GptV4(Base):
    def __init__(self, key, model_name="gpt-4-vision-preview", lang="Chinese", base_url="https://api.openai.com/v1"):
        if not base_url: base_url="https://api.openai.com/v1"
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name
        self.lang = lang

    def describe(self, image, max_tokens=300):
        b64 = self.image2base64(image)
        prompt = self.prompt(b64)
        for i in range(len(prompt)):
            for c in prompt[i]["content"]:
                if "text" in c: c["type"] = "text"

        res = self.client.chat.completions.create(
            model=self.model_name,
            messages=prompt,
            max_tokens=max_tokens,
        )
        return res.choices[0].message.content.strip(), res.usage.total_tokens

class AzureGptV4(Base):
    def __init__(self, key, model_name, lang="Chinese", **kwargs):
        self.client = AzureOpenAI(api_key=key, azure_endpoint=kwargs["base_url"], api_version="2024-02-01")
        self.model_name = model_name
        self.lang = lang

    def describe(self, image, max_tokens=300):
        b64 = self.image2base64(image)
        prompt = self.prompt(b64)
        for i in range(len(prompt)):
            for c in prompt[i]["content"]:
                if "text" in c: c["type"] = "text"

        res = self.client.chat.completions.create(
            model=self.model_name,
            messages=prompt,
            max_tokens=max_tokens,
        )
        return res.choices[0].message.content.strip(), res.usage.total_tokens


class QWenCV(Base):
    def __init__(self, key, model_name="qwen-vl-chat-v1", lang="Chinese", **kwargs):
        import dashscope
        dashscope.api_key = key
        self.model_name = model_name
        self.lang = lang

    def prompt(self, binary):
        # stupid as hell
        tmp_dir = get_project_base_directory("tmp")
        if not os.path.exists(tmp_dir):
            os.mkdir(tmp_dir)
        path = os.path.join(tmp_dir, "%s.jpg" % get_uuid())
        Image.open(io.BytesIO(binary)).save(path)
        return [
            {
                "role": "user",
                "content": [
                    {
                        "image": f"file://{path}"
                    },
                    {
                        "text": "请用中文详细描述一下图中的内容，比如时间，地点，人物，事情，人物心情等，如果有数据请提取出数据。" if self.lang.lower() == "chinese" else
                        "Please describe the content of this picture, like where, when, who, what happen. If it has number data, please extract them out.",
                    },
                ],
            }
        ]

    def describe(self, image, max_tokens=300):
        from http import HTTPStatus
        from dashscope import MultiModalConversation
        response = MultiModalConversation.call(model=self.model_name,
                                               messages=self.prompt(image))
        if response.status_code == HTTPStatus.OK:
            return response.output.choices[0]['message']['content'][0]["text"], response.usage.output_tokens
        return response.message, 0


class Zhipu4V(Base):
    def __init__(self, key, model_name="glm-4v", lang="Chinese", **kwargs):
        self.client = ZhipuAI(api_key=key)
        self.model_name = model_name
        self.lang = lang

    def describe(self, image, max_tokens=1024):
        b64 = self.image2base64(image)

        res = self.client.chat.completions.create(
            model=self.model_name,
            messages=self.prompt(b64),
            max_tokens=max_tokens,
        )
        return res.choices[0].message.content.strip(), res.usage.total_tokens

    def chat_prompt(self, text, b64):
        return [
            {
                "type": "image_url",
                "image_url": {
                    "url": f"{b64}"
                },
            },
            {
                "type": "text",
                "text": text
            },
        ]

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
                max_tokens=gen_conf.get("max_tokens", 1000),
                temperature=gen_conf.get("temperature", 0.3),
                top_p=gen_conf.get("top_p", 0.7)
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
                max_tokens=gen_conf.get("max_tokens", 1000),
                temperature=gen_conf.get("temperature", 0.3),
                top_p=gen_conf.get("top_p", 0.7),
                stream=True
            )
            for resp in response:
                if not resp.choices[0].delta.content: continue
                delta = resp.choices[0].delta.content
                ans += delta
                if resp.choices[0].finish_reason == "length":
                    ans += "...\nFor the content length reason, it stopped, continue?" if is_english(
                        [ans]) else "······\n由于长度的原因，回答被截断了，要继续吗？"
                    tk_count = resp.usage.total_tokens
                if resp.choices[0].finish_reason == "stop": tk_count = resp.usage.total_tokens
                yield ans
        except Exception as e:
            yield ans + "\n**ERROR**: " + str(e)

        yield tk_count


class OllamaCV(Base):
    def __init__(self, key, model_name, lang="Chinese", **kwargs):
        self.client = Client(host=kwargs["base_url"])
        self.model_name = model_name
        self.lang = lang

    def describe(self, image, max_tokens=1024):
        prompt = self.prompt("")
        try:
            options = {"num_predict": max_tokens}
            response = self.client.generate(
                model=self.model_name,
                prompt=prompt[0]["content"][1]["text"],
                images=[image],
                options=options
            )
            ans = response["response"].strip()
            return ans, 128
        except Exception as e:
            return "**ERROR**: " + str(e), 0


class XinferenceCV(Base):
    def __init__(self, key, model_name="", lang="Chinese", base_url=""):
        self.client = OpenAI(api_key="xxx", base_url=base_url)
        self.model_name = model_name
        self.lang = lang

    def describe(self, image, max_tokens=300):
        b64 = self.image2base64(image)

        res = self.client.chat.completions.create(
            model=self.model_name,
            messages=self.prompt(b64),
            max_tokens=max_tokens,
        )
        return res.choices[0].message.content.strip(), res.usage.total_tokens

class GeminiCV(Base):
    def __init__(self, key, model_name="gemini-1.0-pro-vision-latest", lang="Chinese", **kwargs):
        from google.generativeai import client,GenerativeModel 
        client.configure(api_key=key)
        _client = client.get_default_generative_client()
        self.model_name = model_name
        self.model = GenerativeModel(model_name=self.model_name)
        self.model._client = _client
        self.lang = lang 

    def describe(self, image, max_tokens=2048):
        from PIL.Image import open
        gen_config = {'max_output_tokens':max_tokens}
        prompt = "请用中文详细描述一下图中的内容，比如时间，地点，人物，事情，人物心情等，如果有数据请提取出数据。" if self.lang.lower() == "chinese" else \
            "Please describe the content of this picture, like where, when, who, what happen. If it has number data, please extract them out."
        b64 = self.image2base64(image) 
        img = open(BytesIO(base64.b64decode(b64))) 
        input = [prompt,img]
        res = self.model.generate_content(
            input,
            generation_config=gen_config,
        )
        return res.text,res.usage_metadata.total_token_count


class OpenRouterCV(Base):
    def __init__(
        self,
        key,
        model_name,
        lang="Chinese",
        base_url="https://openrouter.ai/api/v1/chat/completions",
    ):
        self.model_name = model_name
        self.lang = lang
        self.base_url = "https://openrouter.ai/api/v1/chat/completions"
        self.key = key

    def describe(self, image, max_tokens=300):
        b64 = self.image2base64(image)
        response = requests.post(
            url=self.base_url,
            headers={
                "Authorization": f"Bearer {self.key}",
            },
            data=json.dumps(
                {
                    "model": self.model_name,
                    "messages": self.prompt(b64),
                    "max_tokens": max_tokens,
                }
            ),
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
                "content": [
                    {
                        "type": "image_url",
                        "image_url": {"url": f"data:image/jpeg;base64,{b64}"},
                    },
                    {
                        "type": "text",
                        "text": (
                            "请用中文详细描述一下图中的内容，比如时间，地点，人物，事情，人物心情等，如果有数据请提取出数据。"
                            if self.lang.lower() == "chinese"
                            else "Please describe the content of this picture, like where, when, who, what happen. If it has number data, please extract them out."
                        ),
                    },
                ],
            }
        ]


class LocalCV(Base):
    def __init__(self, key, model_name="glm-4v", lang="Chinese", **kwargs):
        pass

    def describe(self, image, max_tokens=1024):
        return "", 0
