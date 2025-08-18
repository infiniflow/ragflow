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
import json
import os
from abc import ABC
from copy import deepcopy
from io import BytesIO
from urllib.parse import urljoin
import requests
from openai import OpenAI
from openai.lib.azure import AzureOpenAI
from zhipuai import ZhipuAI
from rag.nlp import is_english
from rag.prompts import vision_llm_describe_prompt
from rag.utils import num_tokens_from_string


class Base(ABC):
    def __init__(self, **kwargs):
        # Configure retry parameters
        self.max_retries = kwargs.get("max_retries", int(os.environ.get("LLM_MAX_RETRIES", 5)))
        self.base_delay = kwargs.get("retry_interval", float(os.environ.get("LLM_BASE_DELAY", 2.0)))
        self.max_rounds = kwargs.get("max_rounds", 5)
        self.is_tools = False
        self.tools = []
        self.toolcall_sessions = {}

    def describe(self, image):
        raise NotImplementedError("Please implement encode method!")

    def describe_with_prompt(self, image, prompt=None):
        raise NotImplementedError("Please implement encode method!")

    def _form_history(self, system, history, images=[]):
        hist = []
        if system:
            hist.append({"role": "system", "content": system})
        for h in history:
            if images and h["role"] == "user":
                h["content"] = self._image_prompt(h["content"], images)
                images = []
            hist.append(h)
        return hist

    def _image_prompt(self, text, images):
        if not images:
            return text

        if isinstance(images, str) or "bytes" in type(images).__name__:
            images = [images]

        pmpt = [{"type": "text", "text": text}]
        for img in images:
            pmpt.append({
                "type": "image_url",
                "image_url": {
                    "url": img if isinstance(img, str) and img.startswith("data:") else f"data:image/png;base64,{img}"
                }
            })
        return pmpt

    def chat(self, system, history, gen_conf, images=[], **kwargs):
        try:
            response = self.client.chat.completions.create(
                model=self.model_name,
                messages=self._form_history(system, history, images)
            )
            return response.choices[0].message.content.strip(), response.usage.total_tokens
        except Exception as e:
            return "**ERROR**: " + str(e), 0

    def chat_streamly(self, system, history, gen_conf, images=[], **kwargs):
        ans = ""
        tk_count = 0
        try:
            response = self.client.chat.completions.create(
                model=self.model_name,
                messages=self._form_history(system, history, images),
                stream=True
            )
            for resp in response:
                if not resp.choices[0].delta.content:
                    continue
                delta = resp.choices[0].delta.content
                ans = delta
                if resp.choices[0].finish_reason == "length":
                    ans += "...\nFor the content length reason, it stopped, continue?" if is_english([ans]) else "······\n由于长度的原因，回答被截断了，要继续吗？"
                if resp.choices[0].finish_reason == "stop":
                    tk_count += resp.usage.total_tokens
                yield ans
        except Exception as e:
            yield ans + "\n**ERROR**: " + str(e)

        yield tk_count

    @staticmethod
    def image2base64(image):
        # Return a data URL with the correct MIME to avoid provider mismatches
        if isinstance(image, bytes):
            # Best-effort magic number sniffing
            mime = "image/png"
            if len(image) >= 2 and image[0] == 0xFF and image[1] == 0xD8:
                mime = "image/jpeg"
            b64 = base64.b64encode(image).decode("utf-8")
            return f"data:{mime};base64,{b64}"
        if isinstance(image, BytesIO):
            data = image.getvalue()
            mime = "image/png"
            if len(data) >= 2 and data[0] == 0xFF and data[1] == 0xD8:
                mime = "image/jpeg"
            b64 = base64.b64encode(data).decode("utf-8")
            return f"data:{mime};base64,{b64}"
        buffered = BytesIO()
        fmt = "JPEG"
        try:
            image.save(buffered, format="JPEG")
        except Exception:
            buffered = BytesIO()  # reset buffer before saving PNG
            image.save(buffered, format="PNG")
            fmt = "PNG"
        data = buffered.getvalue()
        b64 = base64.b64encode(data).decode("utf-8")
        mime = f"image/{fmt.lower()}"
        return f"data:{mime};base64,{b64}"

    def prompt(self, b64):
        return [
            {
                "role": "user",
                "content": self._image_prompt(
                    "请用中文详细描述一下图中的内容，比如时间，地点，人物，事情，人物心情等，如果有数据请提取出数据。"
                    if self.lang.lower() == "chinese"
                    else "Please describe the content of this picture, like where, when, who, what happen. If it has number data, please extract them out.",
                    b64
                )
            }
        ]

    def vision_llm_prompt(self, b64, prompt=None):
        return [
            {
                "role": "user",
                "content": self._image_prompt(prompt if prompt else vision_llm_describe_prompt(), b64)
            }
        ]


class GptV4(Base):
    _FACTORY_NAME = "OpenAI"

    def __init__(self, key, model_name="gpt-4-vision-preview", lang="Chinese", base_url="https://api.openai.com/v1", **kwargs):
        if not base_url:
            base_url = "https://api.openai.com/v1"
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name
        self.lang = lang
        super().__init__(**kwargs)

    def describe(self, image):
        b64 = self.image2base64(image)
        res = self.client.chat.completions.create(
            model=self.model_name,
            messages=self.prompt(b64),
        )
        return res.choices[0].message.content.strip(), res.usage.total_tokens

    def describe_with_prompt(self, image, prompt=None):
        b64 = self.image2base64(image)
        res = self.client.chat.completions.create(
            model=self.model_name,
            messages=self.vision_llm_prompt(b64, prompt),
        )
        return res.choices[0].message.content.strip(), res.usage.total_tokens


class AzureGptV4(GptV4):
    _FACTORY_NAME = "Azure-OpenAI"

    def __init__(self, key, model_name, lang="Chinese", **kwargs):
        api_key = json.loads(key).get("api_key", "")
        api_version = json.loads(key).get("api_version", "2024-02-01")
        self.client = AzureOpenAI(api_key=api_key, azure_endpoint=kwargs["base_url"], api_version=api_version)
        self.model_name = model_name
        self.lang = lang
        Base.__init__(self, **kwargs)


class xAICV(GptV4):
    _FACTORY_NAME = "xAI"

    def __init__(self, key, model_name="grok-3", lang="Chinese", base_url=None, **kwargs):
        if not base_url:
            base_url = "https://api.x.ai/v1"
        super().__init__(key, model_name, lang=lang, base_url=base_url, **kwargs)


class QWenCV(GptV4):
    _FACTORY_NAME = "Tongyi-Qianwen"

    def __init__(self, key, model_name="qwen-vl-chat-v1", lang="Chinese", base_url=None, **kwargs):
        if not base_url:
            base_url = "https://dashscope.aliyuncs.com/compatible-mode/v1"
        super().__init__(key, model_name, lang=lang, base_url=base_url, **kwargs)


class HunyuanCV(GptV4):
    _FACTORY_NAME = "Tencent Hunyuan"

    def __init__(self, key, model_name, lang="Chinese", base_url=None, **kwargs):
        if not base_url:
            base_url = "https://api.hunyuan.cloud.tencent.com/v1"
        super().__init__(key, model_name, lang=lang, base_url=base_url, **kwargs)


class Zhipu4V(GptV4):
    _FACTORY_NAME = "ZHIPU-AI"

    def __init__(self, key, model_name="glm-4v", lang="Chinese", **kwargs):
        self.client = ZhipuAI(api_key=key)
        self.model_name = model_name
        self.lang = lang
        Base.__init__(self, **kwargs)


class StepFunCV(GptV4):
    _FACTORY_NAME = "StepFun"

    def __init__(self, key, model_name="step-1v-8k", lang="Chinese", base_url="https://api.stepfun.com/v1", **kwargs):
        if not base_url:
            base_url = "https://api.stepfun.com/v1"
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name
        self.lang = lang
        Base.__init__(self, **kwargs)


class LmStudioCV(GptV4):
    _FACTORY_NAME = "LM-Studio"

    def __init__(self, key, model_name, lang="Chinese", base_url="", **kwargs):
        if not base_url:
            raise ValueError("Local llm url cannot be None")
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key="lm-studio", base_url=base_url)
        self.model_name = model_name
        self.lang = lang
        Base.__init__(self, **kwargs)


class OpenAI_APICV(GptV4):
    _FACTORY_NAME = ["VLLM", "OpenAI-API-Compatible"]

    def __init__(self, key, model_name, lang="Chinese", base_url="", **kwargs):
        if not base_url:
            raise ValueError("url cannot be None")
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name.split("___")[0]
        self.lang = lang
        Base.__init__(self, **kwargs)


class TogetherAICV(GptV4):
    _FACTORY_NAME = "TogetherAI"

    def __init__(self, key, model_name, lang="Chinese", base_url="https://api.together.xyz/v1", **kwargs):
        if not base_url:
            base_url = "https://api.together.xyz/v1"
        super().__init__(key, model_name, lang, base_url, **kwargs)


class YiCV(GptV4):
    _FACTORY_NAME = "01.AI"

    def __init__(
            self,
            key,
            model_name,
            lang="Chinese",
            base_url="https://api.lingyiwanwu.com/v1", **kwargs
    ):
        if not base_url:
            base_url = "https://api.lingyiwanwu.com/v1"
        super().__init__(key, model_name, lang, base_url, **kwargs)


class SILICONFLOWCV(GptV4):
    _FACTORY_NAME = "SILICONFLOW"

    def __init__(
            self,
            key,
            model_name,
            lang="Chinese",
            base_url="https://api.siliconflow.cn/v1", **kwargs
    ):
        if not base_url:
            base_url = "https://api.siliconflow.cn/v1"
        super().__init__(key, model_name, lang, base_url, **kwargs)


class OpenRouterCV(GptV4):
    _FACTORY_NAME = "OpenRouter"

    def __init__(
            self,
            key,
            model_name,
            lang="Chinese",
            base_url="https://openrouter.ai/api/v1", **kwargs
    ):
        if not base_url:
            base_url = "https://openrouter.ai/api/v1"
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name
        self.lang = lang
        Base.__init__(self, **kwargs)


class LocalAICV(GptV4):
    _FACTORY_NAME = "LocalAI"

    def __init__(self, key, model_name, base_url, lang="Chinese", **kwargs):
        if not base_url:
            raise ValueError("Local cv model url cannot be None")
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key="empty", base_url=base_url)
        self.model_name = model_name.split("___")[0]
        self.lang = lang
        Base.__init__(self, **kwargs)


class XinferenceCV(GptV4):
    _FACTORY_NAME = "Xinference"

    def __init__(self, key, model_name="", lang="Chinese", base_url="", **kwargs):
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name
        self.lang = lang
        Base.__init__(self, **kwargs)


class GPUStackCV(GptV4):
    _FACTORY_NAME = "GPUStack"

    def __init__(self, key, model_name, lang="Chinese", base_url="", **kwargs):
        if not base_url:
            raise ValueError("Local llm url cannot be None")
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name
        self.lang = lang
        Base.__init__(self, **kwargs)


class LocalCV(Base):
    _FACTORY_NAME = "Moonshot"

    def __init__(self, key, model_name="glm-4v", lang="Chinese", **kwargs):
        pass

    def describe(self, image):
        return "", 0


class OllamaCV(Base):
    _FACTORY_NAME = "Ollama"

    def __init__(self, key, model_name, lang="Chinese", **kwargs):
        from ollama import Client
        self.client = Client(host=kwargs["base_url"])
        self.model_name = model_name
        self.lang = lang
        self.keep_alive = kwargs.get("ollama_keep_alive", int(os.environ.get("OLLAMA_KEEP_ALIVE", -1)))
        Base.__init__(self, **kwargs)


    def _clean_img(self, img):
        if not isinstance(img, str):
            return img

        #remove the header like "data/*;base64,"
        if img.startswith("data:") and ";base64," in img:
            img = img.split(";base64,")[1]
        return img

    def _clean_conf(self, gen_conf):
        options = {}
        if "temperature" in gen_conf:
            options["temperature"] = gen_conf["temperature"]
        if "top_p" in gen_conf:
            options["top_k"] = gen_conf["top_p"]
        if "presence_penalty" in gen_conf:
            options["presence_penalty"] = gen_conf["presence_penalty"]
        if "frequency_penalty" in gen_conf:
            options["frequency_penalty"] = gen_conf["frequency_penalty"]
        return options

    def _form_history(self, system, history, images=[]):
        hist = deepcopy(history)
        if system and hist[0]["role"] == "user":
            hist.insert(0, {"role": "system", "content": system})
        if not images:
            return hist
        temp_images = []
        for img in images:
            temp_images.append(self._clean_img(img))
        for his in hist:
            if his["role"] == "user":
                his["images"] = temp_images
                break
        return hist

    def describe(self, image):
        prompt = self.prompt("")
        try:
            response = self.client.generate(
                model=self.model_name,
                prompt=prompt[0]["content"][0]["text"],
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
                prompt=vision_prompt[0]["content"][0]["text"],
                images=[image],
            )
            ans = response["response"].strip()
            return ans, 128
        except Exception as e:
            return "**ERROR**: " + str(e), 0

    def chat(self, system, history, gen_conf, images=[]):
        try:
            response = self.client.chat(
                model=self.model_name,
                messages=self._form_history(system, history, images),
                options=self._clean_conf(gen_conf),
                keep_alive=self.keep_alive
            )

            ans = response["message"]["content"].strip()
            return ans, response["eval_count"] + response.get("prompt_eval_count", 0)
        except Exception as e:
            return "**ERROR**: " + str(e), 0

    def chat_streamly(self, system, history, gen_conf, images=[]):
        ans = ""
        try:
            response = self.client.chat(
                model=self.model_name,
                messages=self._form_history(system, history, images),
                stream=True,
                options=self._clean_conf(gen_conf),
                keep_alive=self.keep_alive
            )
            for resp in response:
                if resp["done"]:
                    yield resp.get("prompt_eval_count", 0) + resp.get("eval_count", 0)
                ans = resp["message"]["content"]
                yield ans
        except Exception as e:
            yield ans + "\n**ERROR**: " + str(e)
        yield 0


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
        Base.__init__(self, **kwargs)

    def _form_history(self, system, history, images=[]):
        hist = []
        if system:
            hist.append({"role": "user", "parts": [system, history[0]["content"]]})
        for img in images:
            hist[0]["parts"].append(("data:image/jpeg;base64," + img) if img[:4]!="data" else img)
        for h in history[1:]:
            hist.append({"role": "user" if h["role"]=="user" else "model", "parts": [h["content"]]})
        return hist

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
        img.close()
        return res.text, res.usage_metadata.total_token_count

    def describe_with_prompt(self, image, prompt=None):
        from PIL.Image import open

        b64 = self.image2base64(image)
        vision_prompt = prompt if prompt else vision_llm_describe_prompt()
        img = open(BytesIO(base64.b64decode(b64)))
        input = [vision_prompt, img]
        res = self.model.generate_content(
            input,
        )
        img.close()
        return res.text, res.usage_metadata.total_token_count

    def chat(self, system, history, gen_conf, images=[]):
        generation_config = dict(temperature=gen_conf.get("temperature", 0.3), top_p=gen_conf.get("top_p", 0.7))
        try:
            response = self.model.generate_content(
                self._form_history(system, history, images),
                generation_config=generation_config)
            ans = response.text
            return ans, response.usage_metadata.total_token_count
        except Exception as e:
            return "**ERROR**: " + str(e), 0

    def chat_streamly(self, system, history, gen_conf, images=[]):
        ans = ""
        response = None
        try:
            generation_config = dict(temperature=gen_conf.get("temperature", 0.3), top_p=gen_conf.get("top_p", 0.7))
            response = self.model.generate_content(
                self._form_history(system, history, images),
                generation_config=generation_config,
                stream=True,
            )

            for resp in response:
                if not resp.text:
                    continue
                ans = resp.text
                yield ans
        except Exception as e:
            yield ans + "\n**ERROR**: " + str(e)

        if response and hasattr(response, "usage_metadata") and hasattr(response.usage_metadata, "total_token_count"):
            yield response.usage_metadata.total_token_count
        else:
            yield 0


class NvidiaCV(Base):
    _FACTORY_NAME = "NVIDIA"

    def __init__(
        self,
        key,
        model_name,
        lang="Chinese",
        base_url="https://ai.api.nvidia.com/v1/vlm", **kwargs
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
        Base.__init__(self, **kwargs)

    def _image_prompt(self, text, images):
        if not images:
            return text
        htmls = ""
        for img in images:
            htmls += ' <img src="{}"/>'.format(f"data:image/jpeg;base64,{img}" if img[:4] != "data" else img)
        return text + htmls

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

    def _request(self, msg, gen_conf={}):
        response = requests.post(
            url=self.base_url,
            headers={
                "accept": "application/json",
                "content-type": "application/json",
                "Authorization": f"Bearer {self.key}",
            },
            json={
                "messages": msg, **gen_conf
            },
        )
        return response.json()

    def describe_with_prompt(self, image, prompt=None):
        b64 = self.image2base64(image)
        vision_prompt = self.vision_llm_prompt(b64, prompt) if prompt else self.vision_llm_prompt(b64)
        response = self._request(vision_prompt)
        return (
            response["choices"][0]["message"]["content"].strip(),
            response["usage"]["total_tokens"],
        )

    def chat(self, system, history, gen_conf, images=[], **kwargs):
        try:
            response = self._request(self._form_history(system, history, images), gen_conf)
            return (
                response["choices"][0]["message"]["content"].strip(),
                response["usage"]["total_tokens"],
            )
        except Exception as e:
            return "**ERROR**: " + str(e), 0

    def chat_streamly(self, system, history, gen_conf, images=[], **kwargs):
        total_tokens = 0
        try:
            response = self._request(self._form_history(system, history, images), gen_conf)
            cnt = response["choices"][0]["message"]["content"]
            if "usage" in response and "total_tokens" in response["usage"]:
                total_tokens += response["usage"]["total_tokens"]
            for resp in cnt:
                yield resp
        except Exception as e:
            yield "\n**ERROR**: " + str(e)

        yield total_tokens


class AnthropicCV(Base):
    _FACTORY_NAME = "Anthropic"

    def __init__(self, key, model_name, base_url=None, **kwargs):
        import anthropic

        self.client = anthropic.Anthropic(api_key=key)
        self.model_name = model_name
        self.system = ""
        self.max_tokens = 8192
        if "haiku" in self.model_name or "opus" in self.model_name:
            self.max_tokens = 4096
        Base.__init__(self, **kwargs)

    def _image_prompt(self, text, images):
        if not images:
            return text
        pmpt = [{"type": "text", "text": text}]
        for img in images:
            pmpt.append({
                        "type": "image",
                        "source": {
                            "type": "base64",
                            "media_type": (img.split(":")[1].split(";")[0] if isinstance(img, str) and img[:4] == "data" else "image/png"),
                            "data": (img.split(",")[1] if isinstance(img, str) and img[:4] == "data" else img)
                        },
                    }
            )
        return pmpt

    def describe(self, image):
        b64 = self.image2base64(image)
        response = self.client.messages.create(model=self.model_name, max_tokens=self.max_tokens, messages=self.prompt(b64))
        return response["content"][0]["text"].strip(), response["usage"]["input_tokens"] + response["usage"]["output_tokens"]

    def describe_with_prompt(self, image, prompt=None):
        b64 = self.image2base64(image)
        prompt = self.prompt(b64, prompt if prompt else vision_llm_describe_prompt())

        response = self.client.messages.create(model=self.model_name, max_tokens=self.max_tokens, messages=prompt)
        return response["content"][0]["text"].strip(), response["usage"]["input_tokens"] + response["usage"]["output_tokens"]

    def _clean_conf(self, gen_conf):
        if "presence_penalty" in gen_conf:
            del gen_conf["presence_penalty"]
        if "frequency_penalty" in gen_conf:
            del gen_conf["frequency_penalty"]
        if "max_token" in gen_conf:
            gen_conf["max_tokens"] = self.max_tokens
        return gen_conf

    def chat(self, system, history, gen_conf, images=[]):
        gen_conf = self._clean_conf(gen_conf)
        ans = ""
        try:
            response = self.client.messages.create(
                model=self.model_name,
                messages=self._form_history(system, history, images),
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

    def chat_streamly(self, system, history, gen_conf, images=[]):
        gen_conf = self._clean_conf(gen_conf)
        total_tokens = 0
        try:
            response = self.client.messages.create(
                model=self.model_name,
                messages=self._form_history(system, history, images),
                system=system,
                stream=True,
                **gen_conf,
            )
            think = False
            for res in response:
                if res.type == "content_block_delta":
                    if res.delta.type == "thinking_delta" and res.delta.thinking:
                        if not think:
                            yield "<think>"
                            think = True
                        yield res.delta.thinking
                        total_tokens += num_tokens_from_string(res.delta.thinking)
                    elif think:
                        yield "</think>"
                    else:
                        yield res.delta.text
                        total_tokens += num_tokens_from_string(res.delta.text)
        except Exception as e:
            yield "\n**ERROR**: " + str(e)

        yield total_tokens


class GoogleCV(AnthropicCV, GeminiCV):
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
        Base.__init__(self, **kwargs)

    def describe(self, image):
        if "claude" in self.model_name:
            return AnthropicCV.describe(self, image)
        else:
            return GeminiCV.describe(self, image)

    def describe_with_prompt(self, image, prompt=None):
        if "claude" in self.model_name:
            return AnthropicCV.describe_with_prompt(self, image, prompt)
        else:
            return GeminiCV.describe_with_prompt(self, image, prompt)

    def chat(self, system, history, gen_conf, images=[]):
        if "claude" in self.model_name:
            return AnthropicCV.chat(self, system, history, gen_conf, images)
        else:
            return GeminiCV.chat(self, system, history, gen_conf, images)

    def chat_streamly(self, system, history, gen_conf, images=[]):
        if "claude" in self.model_name:
            for ans in AnthropicCV.chat_streamly(self, system, history, gen_conf, images):
                yield ans
        else:
            for ans in GeminiCV.chat_streamly(self, system, history, gen_conf, images):
                yield ans
