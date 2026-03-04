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
import logging
import os
import re
import tempfile
from abc import ABC
from copy import deepcopy
from io import BytesIO
from pathlib import Path
from urllib.parse import urljoin

import requests
from openai import OpenAI, AsyncOpenAI
from openai.lib.azure import AzureOpenAI, AsyncAzureOpenAI

from common.token_utils import num_tokens_from_string, total_token_count_from_response
from rag.nlp import is_english
from rag.prompts.generator import vision_llm_describe_prompt




from common.misc_utils import thread_pool_exec

class Base(ABC):
    def __init__(self, **kwargs):
        # Configure retry parameters
        self.max_retries = kwargs.get("max_retries", int(os.environ.get("LLM_MAX_RETRIES", 5)))
        self.base_delay = kwargs.get("retry_interval", float(os.environ.get("LLM_BASE_DELAY", 2.0)))
        self.max_rounds = kwargs.get("max_rounds", 5)
        self.is_tools = False
        self.tools = []
        self.toolcall_sessions = {}
        self.extra_body = None

    def describe(self, image):
        raise NotImplementedError("Please implement encode method!")

    def describe_with_prompt(self, image, prompt=None):
        raise NotImplementedError("Please implement encode method!")

    def _form_history(self, system, history, images=None):
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
            pmpt.append({"type": "image_url", "image_url": {"url": img if isinstance(img, str) and img.startswith("data:") else f"data:image/png;base64,{img}"}})
        return pmpt

    async def async_chat(self, system, history, gen_conf, images=None, **kwargs):
        try:
            response = await self.async_client.chat.completions.create(
                model=self.model_name,
                messages=self._form_history(system, history, images),
                extra_body=self.extra_body,
            )
            return response.choices[0].message.content.strip(), response.usage.total_tokens
        except Exception as e:
            return "**ERROR**: " + str(e), 0

    async def async_chat_streamly(self, system, history, gen_conf, images=None, **kwargs):
        ans = ""
        tk_count = 0
        try:
            response = await self.async_client.chat.completions.create(
                model=self.model_name,
                messages=self._form_history(system, history, images),
                stream=True,
                extra_body=self.extra_body,
            )
            async for resp in response:
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
    def image2base64_rawvalue(self, image):
        # Return a base64 string without data URL header
        if isinstance(image, bytes):
            b64 = base64.b64encode(image).decode("utf-8")
            return b64
        if isinstance(image, BytesIO):
            data = image.getvalue()
            b64 = base64.b64encode(data).decode("utf-8")
            return b64
        with BytesIO() as buffered:
            try:
                image.save(buffered, format="JPEG")
            except Exception:
                # reset buffer before saving PNG
                buffered.seek(0)
                buffered.truncate()
                image.save(buffered, format="PNG")
            data = buffered.getvalue()
            b64 = base64.b64encode(data).decode("utf-8")
        return b64

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
        with BytesIO() as buffered:
            fmt = "jpeg"
            try:
                image.save(buffered, format="JPEG")
            except Exception:
                # reset buffer before saving PNG
                buffered.seek(0)
                buffered.truncate()
                image.save(buffered, format="PNG")
                fmt = "png"
            data = buffered.getvalue()
            b64 = base64.b64encode(data).decode("utf-8")
            mime = f"image/{fmt}"
        return f"data:{mime};base64,{b64}"

    def prompt(self, b64):
        return [
            {
                "role": "user",
                "content": self._image_prompt(
                    "请用中文详细描述一下图中的内容，比如时间，地点，人物，事情，人物心情等，如果有数据请提取出数据。"
                    if self.lang.lower() == "chinese"
                    else "Please describe the content of this picture, like where, when, who, what happen. If it has number data, please extract them out.",
                    b64,
                ),
            }
        ]

    def vision_llm_prompt(self, b64, prompt=None):
        return [{"role": "user", "content": self._image_prompt(prompt if prompt else vision_llm_describe_prompt(), b64)}]


class GptV4(Base):
    _FACTORY_NAME = "OpenAI"

    def __init__(self, key, model_name="gpt-4-vision-preview", lang="Chinese", base_url="https://api.openai.com/v1", **kwargs):
        if not base_url:
            base_url = "https://api.openai.com/v1"
        self.api_key = key
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.async_client = AsyncOpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name
        self.lang = lang
        super().__init__(**kwargs)

    def describe(self, image):
        b64 = self.image2base64(image)
        res = self.client.chat.completions.create(
            model=self.model_name,
            messages=self.prompt(b64),
            extra_body=self.extra_body
        )
        return res.choices[0].message.content.strip(), total_token_count_from_response(res)

    def describe_with_prompt(self, image, prompt=None):
        b64 = self.image2base64(image)
        res = self.client.chat.completions.create(
            model=self.model_name,
            messages=self.vision_llm_prompt(b64, prompt),
            extra_body=self.extra_body,
        )
        return res.choices[0].message.content.strip(), total_token_count_from_response(res)


class AzureGptV4(GptV4):
    _FACTORY_NAME = "Azure-OpenAI"

    def __init__(self, key, model_name, lang="Chinese", **kwargs):
        api_key = json.loads(key).get("api_key", "")
        api_version = json.loads(key).get("api_version", "2024-02-01")
        self.client = AzureOpenAI(api_key=api_key, azure_endpoint=kwargs["base_url"], api_version=api_version)
        self.async_client = AsyncAzureOpenAI(api_key=api_key, azure_endpoint=kwargs["base_url"], api_version=api_version)
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

    async def async_chat(self, system, history, gen_conf, images=None, video_bytes=None, filename="", **kwargs):
        if video_bytes:
            try:
                summary, summary_num_tokens = self._process_video(video_bytes, filename)
                return summary, summary_num_tokens
            except Exception as e:
                return "**ERROR**: " + str(e), 0

        return "**ERROR**: Method chat not supported yet.", 0

    def _process_video(self, video_bytes, filename):
        from dashscope import MultiModalConversation

        video_suffix = Path(filename).suffix or ".mp4"
        with tempfile.NamedTemporaryFile(delete=False, suffix=video_suffix) as tmp:
            tmp.write(video_bytes)
            tmp_path = tmp.name

            video_path = f"file://{tmp_path}"
            messages = [
                {
                    "role": "user",
                    "content": [
                        {
                            "video": video_path,
                            "fps": 2,
                        },
                        {
                            "text": "Please summarize this video in proper sentences.",
                        },
                    ],
                }
            ]

            def call_api():
                response = MultiModalConversation.call(
                    api_key=self.api_key,
                    model=self.model_name,
                    messages=messages,
                )
                if response.get("message"):
                    raise Exception(response["message"])
                summary = response["output"]["choices"][0]["message"].content[0]["text"]
                return summary, num_tokens_from_string(summary)

            try:
                return call_api()
            except Exception as e1:
                import dashscope

                dashscope.base_http_api_url = "https://dashscope-intl.aliyuncs.com/api/v1"
                try:
                    return call_api()
                except Exception as e2:
                    raise RuntimeError(f"Both default and intl endpoint failed.\nFirst error: {e1}\nSecond error: {e2}")


class HunyuanCV(GptV4):
    _FACTORY_NAME = "Tencent Hunyuan"

    def __init__(self, key, model_name, lang="Chinese", base_url=None, **kwargs):
        if not base_url:
            base_url = "https://api.hunyuan.cloud.tencent.com/v1"
        super().__init__(key, model_name, lang=lang, base_url=base_url, **kwargs)


class Zhipu4V(GptV4):
    _FACTORY_NAME = "ZHIPU-AI"

    def __init__(self, key, model_name="glm-4v", lang="Chinese", **kwargs):
        self.client = OpenAI(api_key=key, base_url="https://open.bigmodel.cn/api/paas/v4/")
        self.async_client = AsyncOpenAI(api_key=key, base_url="https://open.bigmodel.cn/api/paas/v4/")
        self.model_name = model_name
        self.lang = lang
        Base.__init__(self, **kwargs)

    def _clean_conf(self, gen_conf):
        if "max_tokens" in gen_conf:
            del gen_conf["max_tokens"]
        gen_conf = self._clean_conf_plealty(gen_conf)
        return gen_conf

    def _clean_conf_plealty(self, gen_conf):
        if "presence_penalty" in gen_conf:
            del gen_conf["presence_penalty"]
        if "frequency_penalty" in gen_conf:
            del gen_conf["frequency_penalty"]
        return gen_conf

    def _request(self, msg, stream, gen_conf={}):
        response = requests.post(
            self.base_url,
            json={"model": self.model_name, "messages": msg, "stream": stream, **gen_conf},
            headers={
                "Authorization": f"Bearer {self.api_key}",
                "Content-Type": "application/json",
            },
        )
        return response.json()

    async def async_chat(self, system, history, gen_conf, images=None, **kwargs):
        if system and history and history[0].get("role") != "system":
            history.insert(0, {"role": "system", "content": system})

        gen_conf = self._clean_conf(gen_conf)

        logging.info(json.dumps(history, ensure_ascii=False, indent=2))
        response = await self.async_client.chat.completions.create(model=self.model_name, messages=self._form_history(system, history, images), stream=False, **gen_conf)
        content = response.choices[0].message.content.strip()

        cleaned = re.sub(r"<\|(begin_of_box|end_of_box)\|>", "", content).strip()
        return cleaned, total_token_count_from_response(response)

    async def async_chat_streamly(self, system, history, gen_conf, images=None, **kwargs):
        from rag.llm.chat_model import LENGTH_NOTIFICATION_CN, LENGTH_NOTIFICATION_EN
        from rag.nlp import is_chinese

        if system and history and history[0].get("role") != "system":
            history.insert(0, {"role": "system", "content": system})
        gen_conf = self._clean_conf(gen_conf)
        ans = ""
        tk_count = 0
        try:
            logging.info(json.dumps(history, ensure_ascii=False, indent=2))
            response = await self.async_client.chat.completions.create(model=self.model_name, messages=self._form_history(system, history, images), stream=True, **gen_conf)
            async for resp in response:
                if not resp.choices[0].delta.content:
                    continue
                delta = resp.choices[0].delta.content
                ans = delta
                if resp.choices[0].finish_reason == "length":
                    if is_chinese(ans):
                        ans += LENGTH_NOTIFICATION_CN
                    else:
                        ans += LENGTH_NOTIFICATION_EN
                    tk_count = total_token_count_from_response(resp)
                if resp.choices[0].finish_reason == "stop":
                    tk_count = total_token_count_from_response(resp)
                yield ans
        except Exception as e:
            yield ans + "\n**ERROR**: " + str(e)

        yield tk_count

    def describe(self, image):
        return self.describe_with_prompt(image)

    def describe_with_prompt(self, image, prompt=None):
        b64 = self.image2base64(image)
        if prompt is None:
            prompt = "Describe this image."

        # Chat messages
        messages = [{"role": "user", "content": [{"type": "image_url", "image_url": {"url": b64}}, {"type": "text", "text": prompt}]}]

        resp = self.client.chat.completions.create(model=self.model_name, messages=messages, stream=False)

        content = resp.choices[0].message.content.strip()
        cleaned = re.sub(r"<\|(begin_of_box|end_of_box)\|>", "", content).strip()

        return cleaned, num_tokens_from_string(cleaned)


class StepFunCV(GptV4):
    _FACTORY_NAME = "StepFun"

    def __init__(self, key, model_name="step-1v-8k", lang="Chinese", base_url="https://api.stepfun.com/v1", **kwargs):
        if not base_url:
            base_url = "https://api.stepfun.com/v1"
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.async_client = AsyncOpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name
        self.lang = lang
        Base.__init__(self, **kwargs)


class VolcEngineCV(GptV4):
    _FACTORY_NAME = "VolcEngine"

    def __init__(self, key, model_name, lang="Chinese", base_url="https://ark.cn-beijing.volces.com/api/v3", **kwargs):
        if not base_url:
            base_url = "https://ark.cn-beijing.volces.com/api/v3"
        ark_api_key = json.loads(key).get("ark_api_key", "")
        self.client = OpenAI(api_key=ark_api_key, base_url=base_url)
        self.async_client = AsyncOpenAI(api_key=ark_api_key, base_url=base_url)
        self.model_name = json.loads(key).get("ep_id", "") + json.loads(key).get("endpoint_id", "")
        self.lang = lang
        Base.__init__(self, **kwargs)


class LmStudioCV(GptV4):
    _FACTORY_NAME = "LM-Studio"

    def __init__(self, key, model_name, lang="Chinese", base_url="", **kwargs):
        if not base_url:
            raise ValueError("Local llm url cannot be None")
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key="lm-studio", base_url=base_url)
        self.async_client = AsyncOpenAI(api_key="lm-studio", base_url=base_url)
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
        self.async_client = AsyncOpenAI(api_key=key, base_url=base_url)
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

    def __init__(self, key, model_name, lang="Chinese", base_url="https://api.lingyiwanwu.com/v1", **kwargs):
        if not base_url:
            base_url = "https://api.lingyiwanwu.com/v1"
        super().__init__(key, model_name, lang, base_url, **kwargs)


class SILICONFLOWCV(GptV4):
    _FACTORY_NAME = "SILICONFLOW"

    def __init__(self, key, model_name, lang="Chinese", base_url="https://api.siliconflow.cn/v1", **kwargs):
        if not base_url:
            base_url = "https://api.siliconflow.cn/v1"
        super().__init__(key, model_name, lang, base_url, **kwargs)


class OpenRouterCV(GptV4):
    _FACTORY_NAME = "OpenRouter"

    def __init__(self, key, model_name, lang="Chinese", base_url="https://openrouter.ai/api/v1", **kwargs):
        if not base_url:
            base_url = "https://openrouter.ai/api/v1"
        api_key = json.loads(key).get("api_key", "")
        self.client = OpenAI(api_key=api_key, base_url=base_url)
        self.async_client = AsyncOpenAI(api_key=api_key, base_url=base_url)
        self.model_name = model_name
        self.lang = lang
        Base.__init__(self, **kwargs)
        provider_order = json.loads(key).get("provider_order", "")
        self.extra_body = {}
        if provider_order:

            def _to_order_list(x):
                if x is None:
                    return []
                if isinstance(x, str):
                    return [s.strip() for s in x.split(",") if s.strip()]
                if isinstance(x, (list, tuple)):
                    return [str(s).strip() for s in x if str(s).strip()]
                return []

            provider_cfg = {}
            provider_order = _to_order_list(provider_order)
            provider_cfg["order"] = provider_order
            provider_cfg["allow_fallbacks"] = False
            self.extra_body["provider"] = provider_cfg


class LocalAICV(GptV4):
    _FACTORY_NAME = "LocalAI"

    def __init__(self, key, model_name, base_url, lang="Chinese", **kwargs):
        if not base_url:
            raise ValueError("Local cv model url cannot be None")
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key="empty", base_url=base_url)
        self.async_client = AsyncOpenAI(api_key="empty", base_url=base_url)
        self.model_name = model_name.split("___")[0]
        self.lang = lang
        Base.__init__(self, **kwargs)


class XinferenceCV(GptV4):
    _FACTORY_NAME = "Xinference"

    def __init__(self, key, model_name="", lang="Chinese", base_url="", **kwargs):
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.async_client = AsyncOpenAI(api_key=key, base_url=base_url)
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
        self.async_client = AsyncOpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name
        self.lang = lang
        Base.__init__(self, **kwargs)


class LocalCV(Base):
    _FACTORY_NAME = "Local"

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

        # remove the header like "data/*;base64,"
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

    def _form_history(self, system, history, images=None):
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
                prompt=prompt[0]["content"],
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
                prompt=vision_prompt[0]["content"],
                images=[image],
            )
            ans = response["response"].strip()
            return ans, 128
        except Exception as e:
            return "**ERROR**: " + str(e), 0

    async def async_chat(self, system, history, gen_conf, images=None, **kwargs):
        try:
            response = await thread_pool_exec(self.client.chat, model=self.model_name, messages=self._form_history(system, history, images), options=self._clean_conf(gen_conf), keep_alive=self.keep_alive)

            ans = response["message"]["content"].strip()
            return ans, response["eval_count"] + response.get("prompt_eval_count", 0)
        except Exception as e:
            return "**ERROR**: " + str(e), 0

    async def async_chat_streamly(self, system, history, gen_conf, images=None, **kwargs):
        ans = ""
        try:
            response = await thread_pool_exec(self.client.chat, model=self.model_name, messages=self._form_history(system, history, images), stream=True, options=self._clean_conf(gen_conf), keep_alive=self.keep_alive)
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
        from google import genai

        self.api_key = key
        self.model_name = model_name
        self.client = genai.Client(api_key=key)
        self.lang = lang
        Base.__init__(self, **kwargs)
        logging.info(f"[GeminiCV] Initialized with model={self.model_name} lang={self.lang}")

    def _image_to_part(self, image):
        from google.genai import types

        if isinstance(image, str) and image.startswith("data:") and ";base64," in image:
            header, b64data = image.split(",", 1)
            mime = header.split(":", 1)[1].split(";", 1)[0]
            data = base64.b64decode(b64data)
        else:
            data_url = self.image2base64(image)
            header, b64data = data_url.split(",", 1)
            mime = header.split(":", 1)[1].split(";", 1)[0]
            data = base64.b64decode(b64data)

        return types.Part(
            inline_data=types.Blob(
                mime_type=mime,
                data=data,
            )
        )

    def _form_history(self, system, history, images=None):
        from google.genai import types

        contents = []
        images = images or []
        system_len = len(system) if isinstance(system, str) else 0
        history_len = len(history) if history else 0
        images_len = len(images)
        logging.info(f"[GeminiCV] _form_history called: system_len={system_len} history_len={history_len} images_len={images_len}")

        image_parts = []
        for img in images:
            try:
                image_parts.append(self._image_to_part(img))
            except Exception:
                continue

        remaining_history = history or []
        if system or remaining_history:
            parts = []
            if system:
                parts.append(types.Part(text=system))
            if remaining_history:
                first = remaining_history[0]
                parts.append(types.Part(text=first.get("content", "")))
                remaining_history = remaining_history[1:]
            parts.extend(image_parts)
            contents.append(types.Content(role="user", parts=parts))
        elif image_parts:
            contents.append(types.Content(role="user", parts=image_parts))

        role_map = {"user": "user", "assistant": "model", "system": "user"}
        for h in remaining_history:
            role = role_map.get(h.get("role"), "user")
            contents.append(
                types.Content(
                    role=role,
                    parts=[types.Part(text=h.get("content", ""))],
                )
            )

        return contents

    def describe(self, image):
        from google.genai import types

        prompt = (
            "请用中文详细描述一下图中的内容，比如时间，地点，人物，事情，人物心情等，如果有数据请提取出数据。"
            if self.lang.lower() == "chinese"
            else "Please describe the content of this picture, like where, when, who, what happen. If it has number data, please extract them out."
        )

        contents = [
            types.Content(
                role="user",
                parts=[
                    types.Part(text=prompt),
                    self._image_to_part(image),
                ],
            )
        ]

        res = self.client.models.generate_content(
            model=self.model_name,
            contents=contents,
        )
        return res.text, total_token_count_from_response(res)

    def describe_with_prompt(self, image, prompt=None):
        from google.genai import types

        vision_prompt = prompt if prompt else vision_llm_describe_prompt()

        contents = [
            types.Content(
                role="user",
                parts=[
                    types.Part(text=vision_prompt),
                    self._image_to_part(image),
                ],
            )
        ]

        res = self.client.models.generate_content(
            model=self.model_name,
            contents=contents,
        )
        return res.text, total_token_count_from_response(res)

    async def async_chat(self, system, history, gen_conf, images=None, video_bytes=None, filename="", **kwargs):
        if video_bytes:
            try:
                size = len(video_bytes) if video_bytes else 0
                logging.info(f"[GeminiCV] async_chat called with video: filename={filename} size={size}")
                summary, summary_num_tokens = await thread_pool_exec(self._process_video, video_bytes, filename)
                return summary, summary_num_tokens
            except Exception as e:
                logging.info(f"[GeminiCV] async_chat video error: {e}")
                return "**ERROR**: " + str(e), 0

        from google.genai import types

        history_len = len(history) if history else 0
        images_len = len(images) if images else 0
        logging.info(f"[GeminiCV] async_chat called: history_len={history_len} images_len={images_len} gen_conf={gen_conf}")

        generation_config = types.GenerateContentConfig(
            temperature=gen_conf.get("temperature", 0.3),
            top_p=gen_conf.get("top_p", 0.7),
        )
        try:
            response = await self.client.aio.models.generate_content(
                model=self.model_name,
                contents=self._form_history(system, history, images),
                config=generation_config,
            )
            ans = response.text
            logging.info("[GeminiCV] async_chat completed")
            return ans, total_token_count_from_response(response)
        except Exception as e:
            logging.warning(f"[GeminiCV] async_chat error: {e}")
            return "**ERROR**: " + str(e), 0

    async def async_chat_streamly(self, system, history, gen_conf, images=None, **kwargs):
        ans = ""
        response = None
        try:
            from google.genai import types

            generation_config = types.GenerateContentConfig(
                temperature=gen_conf.get("temperature", 0.3),
                top_p=gen_conf.get("top_p", 0.7),
            )
            history_len = len(history) if history else 0
            images_len = len(images) if images else 0
            logging.info(f"[GeminiCV] async_chat_streamly called: history_len={history_len} images_len={images_len} gen_conf={gen_conf}")

            response_stream = await self.client.aio.models.generate_content_stream(
                model=self.model_name,
                contents=self._form_history(system, history, images),
                config=generation_config,
            )

            async for chunk in response_stream:
                if chunk.text:
                    ans += chunk.text
                    yield chunk.text
            logging.info("[GeminiCV] chat_streamly completed")
        except Exception as e:
            logging.warning(f"[GeminiCV] chat_streamly error: {e}")
            yield ans + "\n**ERROR**: " + str(e)

        yield total_token_count_from_response(response)

    def _process_video(self, video_bytes, filename):
        from google import genai
        from google.genai import types

        video_size_mb = len(video_bytes) / (1024 * 1024)
        client = self.client if hasattr(self, "client") else genai.Client(api_key=self.api_key)
        logging.info(f"[GeminiCV] _process_video called: filename={filename} size_mb={video_size_mb:.2f}")

        tmp_path = None
        try:
            if video_size_mb <= 20:
                response = client.models.generate_content(
                    model="models/gemini-2.5-flash",
                    contents=types.Content(parts=[types.Part(inline_data=types.Blob(data=video_bytes, mime_type="video/mp4")), types.Part(text="Please summarize the video in proper sentences.")]),
                )
            else:
                logging.info(f"Video size {video_size_mb:.2f}MB exceeds 20MB. Using Files API...")
                video_suffix = Path(filename).suffix or ".mp4"
                with tempfile.NamedTemporaryFile(delete=False, suffix=video_suffix) as tmp:
                    tmp.write(video_bytes)
                    tmp_path = Path(tmp.name)
                uploaded_file = client.files.upload(file=tmp_path)

                response = client.models.generate_content(model="gemini-2.5-flash", contents=[uploaded_file, "Please summarize this video in proper sentences."])

            summary = response.text or ""
            logging.info(f"[GeminiCV] Video summarized: {summary[:32]}...")
            return summary, num_tokens_from_string(summary)
        except Exception as e:
            logging.warning(f"[GeminiCV] Video processing failed: {e}")
            raise
        finally:
            if tmp_path and tmp_path.exists():
                tmp_path.unlink()


class NvidiaCV(Base):
    _FACTORY_NAME = "NVIDIA"

    def __init__(self, key, model_name, lang="Chinese", base_url="https://ai.api.nvidia.com/v1/vlm", **kwargs):
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
            total_token_count_from_response(response),
        )

    def _request(self, msg, gen_conf={}):
        response = requests.post(
            url=self.base_url,
            headers={
                "accept": "application/json",
                "content-type": "application/json",
                "Authorization": f"Bearer {self.key}",
            },
            json={"messages": msg, **gen_conf},
        )
        return response.json()

    def describe_with_prompt(self, image, prompt=None):
        b64 = self.image2base64(image)
        vision_prompt = self.vision_llm_prompt(b64, prompt) if prompt else self.vision_llm_prompt(b64)
        response = self._request(vision_prompt)
        return (response["choices"][0]["message"]["content"].strip(), total_token_count_from_response(response))

    async def async_chat(self, system, history, gen_conf, images=None, **kwargs):
        try:
            response = await thread_pool_exec(self._request, self._form_history(system, history, images), gen_conf)
            return (response["choices"][0]["message"]["content"].strip(), total_token_count_from_response(response))
        except Exception as e:
            return "**ERROR**: " + str(e), 0

    async def async_chat_streamly(self, system, history, gen_conf, images=None, **kwargs):
        total_tokens = 0
        try:
            response = await thread_pool_exec(self._request, self._form_history(system, history, images), gen_conf)
            cnt = response["choices"][0]["message"]["content"]
            total_tokens += total_token_count_from_response(response)
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
        self.async_client = anthropic.AsyncAnthropic(api_key=key)
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
            pmpt.append(
                {
                    "type": "image",
                    "source": {
                        "type": "base64",
                        "media_type": (img.split(":")[1].split(";")[0] if isinstance(img, str) and img[:4] == "data" else "image/png"),
                        "data": (img.split(",")[1] if isinstance(img, str) and img[:4] == "data" else img),
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
        return response["content"][0]["text"].strip(), total_token_count_from_response(response)

    def _clean_conf(self, gen_conf):
        if "presence_penalty" in gen_conf:
            del gen_conf["presence_penalty"]
        if "frequency_penalty" in gen_conf:
            del gen_conf["frequency_penalty"]
        if "max_token" in gen_conf:
            gen_conf["max_tokens"] = self.max_tokens
        return gen_conf

    async def async_chat(self, system, history, gen_conf, images=None, **kwargs):
        gen_conf = self._clean_conf(gen_conf)
        ans = ""
        try:
            response = await self.async_client.messages.create(
                model=self.model_name,
                messages=self._form_history(system, history, images),
                system=system,
                stream=False,
                **gen_conf,
            )
            response = response.to_dict()
            ans = response["content"][0]["text"]
            if response["stop_reason"] == "max_tokens":
                ans += "...\nFor the content length reason, it stopped, continue?" if is_english([ans]) else "······\n由于长度的原因，回答被截断了，要继续吗？"
            return (
                ans,
                total_token_count_from_response(response),
            )
        except Exception as e:
            return ans + "\n**ERROR**: " + str(e), 0

    async def async_chat_streamly(self, system, history, gen_conf, images=None, **kwargs):
        gen_conf = self._clean_conf(gen_conf)
        total_tokens = 0
        try:
            response = self.async_client.messages.create(
                model=self.model_name,
                messages=self._form_history(system, history, images),
                system=system,
                stream=True,
                **gen_conf,
            )
            think = False
            async for res in response:
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

    async def async_chat(self, system, history, gen_conf, images=None, **kwargs):
        if "claude" in self.model_name:
            return await AnthropicCV.async_chat(self, system, history, gen_conf, images)
        else:
            return await GeminiCV.async_chat(self, system, history, gen_conf, images)

    async def async_chat_streamly(self, system, history, gen_conf, images=None, **kwargs):
        if "claude" in self.model_name:
            async for ans in AnthropicCV.async_chat_streamly(self, system, history, gen_conf, images):
                yield ans
        else:
            async for ans in GeminiCV.async_chat_streamly(self, system, history, gen_conf, images):
                yield ans


class MoonshotCV(GptV4):
    _FACTORY_NAME = "Moonshot"

    def __init__(self, key, model_name="moonshot-v1-8k-vision-preview", lang="Chinese", base_url="https://api.moonshot.cn/v1", **kwargs):
        if not base_url:
            base_url = "https://api.moonshot.cn/v1"
        super().__init__(key, model_name, lang=lang, base_url=base_url, **kwargs)
