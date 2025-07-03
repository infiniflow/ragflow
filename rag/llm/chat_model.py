#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
import asyncio
import json
import logging
import os
import random
import time
from abc import ABC
from copy import deepcopy
from typing import Any, Protocol
from urllib.parse import urljoin

import json_repair
import openai
import requests
from dashscope import Generation
from ollama import Client
from openai import OpenAI
from openai.lib.azure import AzureOpenAI
from zhipuai import ZhipuAI

from rag.nlp import is_chinese, is_english
from rag.utils import num_tokens_from_string

# Error message constants
ERROR_PREFIX = "**ERROR**"
ERROR_RATE_LIMIT = "RATE_LIMIT_EXCEEDED"
ERROR_AUTHENTICATION = "AUTH_ERROR"
ERROR_INVALID_REQUEST = "INVALID_REQUEST"
ERROR_SERVER = "SERVER_ERROR"
ERROR_TIMEOUT = "TIMEOUT"
ERROR_CONNECTION = "CONNECTION_ERROR"
ERROR_MODEL = "MODEL_ERROR"
ERROR_CONTENT_FILTER = "CONTENT_FILTERED"
ERROR_QUOTA = "QUOTA_EXCEEDED"
ERROR_MAX_RETRIES = "MAX_RETRIES_EXCEEDED"
ERROR_GENERIC = "GENERIC_ERROR"

LENGTH_NOTIFICATION_CN = "······\n由于大模型的上下文窗口大小限制，回答已经被大模型截断。"
LENGTH_NOTIFICATION_EN = "...\nThe answer is truncated by your chosen LLM due to its limitation on context length."


class ToolCallSession(Protocol):
    def tool_call(self, name: str, arguments: dict[str, Any]) -> str: ...


class Base(ABC):
    def __init__(self, key, model_name, base_url, **kwargs):
        timeout = int(os.environ.get("LM_TIMEOUT_SECONDS", 600))
        self.client = OpenAI(api_key=key, base_url=base_url, timeout=timeout)
        self.model_name = model_name
        # Configure retry parameters
        self.max_retries = kwargs.get("max_retries", int(os.environ.get("LLM_MAX_RETRIES", 5)))
        self.base_delay = kwargs.get("retry_interval", float(os.environ.get("LLM_BASE_DELAY", 2.0)))
        self.max_rounds = kwargs.get("max_rounds", 5)
        self.is_tools = False
        self.tools = []
        self.toolcall_sessions = {}

    def _get_delay(self):
        """Calculate retry delay time"""
        return self.base_delay + random.uniform(0, 0.5)

    def _classify_error(self, error):
        """Classify error based on error message content"""
        error_str = str(error).lower()

        if "rate limit" in error_str or "429" in error_str or "tpm limit" in error_str or "too many requests" in error_str or "requests per minute" in error_str:
            return ERROR_RATE_LIMIT
        elif "auth" in error_str or "key" in error_str or "apikey" in error_str or "401" in error_str or "forbidden" in error_str or "permission" in error_str:
            return ERROR_AUTHENTICATION
        elif "invalid" in error_str or "bad request" in error_str or "400" in error_str or "format" in error_str or "malformed" in error_str or "parameter" in error_str:
            return ERROR_INVALID_REQUEST
        elif "server" in error_str or "502" in error_str or "503" in error_str or "504" in error_str or "500" in error_str or "unavailable" in error_str:
            return ERROR_SERVER
        elif "timeout" in error_str or "timed out" in error_str:
            return ERROR_TIMEOUT
        elif "connect" in error_str or "network" in error_str or "unreachable" in error_str or "dns" in error_str:
            return ERROR_CONNECTION
        elif "quota" in error_str or "capacity" in error_str or "credit" in error_str or "billing" in error_str or "limit" in error_str and "rate" not in error_str:
            return ERROR_QUOTA
        elif "filter" in error_str or "content" in error_str or "policy" in error_str or "blocked" in error_str or "safety" in error_str or "inappropriate" in error_str:
            return ERROR_CONTENT_FILTER
        elif "model" in error_str or "not found" in error_str or "does not exist" in error_str or "not available" in error_str:
            return ERROR_MODEL
        else:
            return ERROR_GENERIC

    def _clean_conf(self, gen_conf):
        if "max_tokens" in gen_conf:
            del gen_conf["max_tokens"]
        return gen_conf

    def _chat(self, history, gen_conf):
        response = self.client.chat.completions.create(model=self.model_name, messages=history, **gen_conf)

        if any([not response.choices, not response.choices[0].message, not response.choices[0].message.content]):
            return "", 0
        ans = response.choices[0].message.content.strip()
        if response.choices[0].finish_reason == "length":
            if is_chinese(ans):
                ans += LENGTH_NOTIFICATION_CN
            else:
                ans += LENGTH_NOTIFICATION_EN
        return ans, self.total_token_count(response)

    def _length_stop(self, ans):
        if is_chinese([ans]):
            return ans + LENGTH_NOTIFICATION_CN
        return ans + LENGTH_NOTIFICATION_EN

    def _exceptions(self, e, attempt):
        logging.exception("OpenAI cat_with_tools")
        # Classify the error
        error_code = self._classify_error(e)

        # Check if it's a rate limit error or server error and not the last attempt
        should_retry = (error_code == ERROR_RATE_LIMIT or error_code == ERROR_SERVER) and attempt < self.max_retries

        if should_retry:
            delay = self._get_delay()
            logging.warning(f"Error: {error_code}. Retrying in {delay:.2f} seconds... (Attempt {attempt + 1}/{self.max_retries})")
            time.sleep(delay)
        else:
            # For non-rate limit errors or the last attempt, return an error message
            if attempt == self.max_retries:
                error_code = ERROR_MAX_RETRIES
            return f"{ERROR_PREFIX}: {error_code} - {str(e)}"

    def _verbose_tool_use(self, name, args, res):
        return "<tool_call>" + json.dumps({"name": name, "args": args, "result": res}, ensure_ascii=False, indent=2) + "</tool_call>"

    def _append_history(self, hist, tool_call, tool_res):
        hist.append(
            {
                "role": "assistant",
                "tool_calls": [
                    {
                        "index": tool_call.index,
                        "id": tool_call.id,
                        "function": {
                            "name": tool_call.function.name,
                            "arguments": tool_call.function.arguments,
                        },
                        "type": "function",
                    },
                ],
            }
        )
        try:
            if isinstance(tool_res, dict):
                tool_res = json.dumps(tool_res, ensure_ascii=False)
        finally:
            hist.append({"role": "tool", "tool_call_id": tool_call.id, "content": str(tool_res)})
        return hist

    def bind_tools(self, toolcall_session, tools):
        if not (toolcall_session and tools):
            return
        self.is_tools = True

        for tool in tools:
            self.toolcall_sessions[tool["function"]["name"]] = toolcall_session
            self.tools.append(tool)

    def chat_with_tools(self, system: str, history: list, gen_conf: dict):
        gen_conf = self._clean_conf(gen_conf)
        if system:
            history.insert(0, {"role": "system", "content": system})

        gen_conf = self._clean_conf(gen_conf)
        ans = ""
        tk_count = 0
        hist = deepcopy(history)
        # Implement exponential backoff retry strategy
        for attempt in range(self.max_retries + 1):
            history = hist
            try:
                for _ in range(self.max_rounds * 2):
                    response = self.client.chat.completions.create(model=self.model_name, messages=history, tools=self.tools, **gen_conf)
                    tk_count += self.total_token_count(response)
                    if any([not response.choices, not response.choices[0].message]):
                        raise Exception(f"500 response structure error. Response: {response}")

                    if not hasattr(response.choices[0].message, "tool_calls") or not response.choices[0].message.tool_calls:
                        if hasattr(response.choices[0].message, "reasoning_content") and response.choices[0].message.reasoning_content:
                            ans += "<think>" + response.choices[0].message.reasoning_content + "</think>"

                        ans += response.choices[0].message.content
                        if response.choices[0].finish_reason == "length":
                            ans = self._length_stop(ans)

                        return ans, tk_count

                    for tool_call in response.choices[0].message.tool_calls:
                        name = tool_call.function.name
                        try:
                            args = json_repair.loads(tool_call.function.arguments)
                            tool_response = self.toolcall_sessions[name].tool_call(name, args)
                            history = self._append_history(history, tool_call, tool_response)
                            ans += self._verbose_tool_use(name, args, tool_response)
                        except Exception as e:
                            logging.exception(msg=f"Wrong JSON argument format in LLM tool call response: {tool_call}")
                            history.append({"role": "tool", "tool_call_id": tool_call.id, "content": f"Tool call error: \n{tool_call}\nException:\n" + str(e)})
                            ans += self._verbose_tool_use(name, {}, str(e))

            except Exception as e:
                e = self._exceptions(e, attempt)
                if e:
                    return e, tk_count
        assert False, "Shouldn't be here."

    def chat(self, system, history, gen_conf):
        if system:
            history.insert(0, {"role": "system", "content": system})
        gen_conf = self._clean_conf(gen_conf)

        # Implement exponential backoff retry strategy
        for attempt in range(self.max_retries + 1):
            try:
                return self._chat(history, gen_conf)
            except Exception as e:
                e = self._exceptions(e, attempt)
                if e:
                    return e, 0
        assert False, "Shouldn't be here."

    def _wrap_toolcall_message(self, stream):
        final_tool_calls = {}

        for chunk in stream:
            for tool_call in chunk.choices[0].delta.tool_calls or []:
                index = tool_call.index

                if index not in final_tool_calls:
                    final_tool_calls[index] = tool_call

                final_tool_calls[index].function.arguments += tool_call.function.arguments

        return final_tool_calls

    def chat_streamly_with_tools(self, system: str, history: list, gen_conf: dict):
        gen_conf = self._clean_conf(gen_conf)
        tools = self.tools
        if system:
            history.insert(0, {"role": "system", "content": system})

        total_tokens = 0
        hist = deepcopy(history)
        # Implement exponential backoff retry strategy
        for attempt in range(self.max_retries + 1):
            history = hist
            try:
                for _ in range(self.max_rounds * 2):
                    reasoning_start = False
                    response = self.client.chat.completions.create(model=self.model_name, messages=history, stream=True, tools=tools, **gen_conf)
                    final_tool_calls = {}
                    answer = ""
                    for resp in response:
                        if resp.choices[0].delta.tool_calls:
                            for tool_call in resp.choices[0].delta.tool_calls or []:
                                index = tool_call.index

                                if index not in final_tool_calls:
                                    if not tool_call.function.arguments:
                                        tool_call.function.arguments = ""
                                    final_tool_calls[index] = tool_call
                                else:
                                    final_tool_calls[index].function.arguments += tool_call.function.arguments if tool_call.function.arguments else ""
                            continue

                        if any([not resp.choices, not resp.choices[0].delta, not hasattr(resp.choices[0].delta, "content")]):
                            raise Exception("500 response structure error.")

                        if not resp.choices[0].delta.content:
                            resp.choices[0].delta.content = ""

                        if hasattr(resp.choices[0].delta, "reasoning_content") and resp.choices[0].delta.reasoning_content:
                            ans = ""
                            if not reasoning_start:
                                reasoning_start = True
                                ans = "<think>"
                            ans += resp.choices[0].delta.reasoning_content + "</think>"
                            yield ans
                        else:
                            reasoning_start = False
                            answer += resp.choices[0].delta.content
                            yield resp.choices[0].delta.content

                        tol = self.total_token_count(resp)
                        if not tol:
                            total_tokens += num_tokens_from_string(resp.choices[0].delta.content)
                        else:
                            total_tokens += tol

                        finish_reason = resp.choices[0].finish_reason if hasattr(resp.choices[0], "finish_reason") else ""
                        if finish_reason == "length":
                            yield self._length_stop("")

                    if answer:
                        yield total_tokens
                        return

                    for tool_call in final_tool_calls.values():
                        name = tool_call.function.name
                        try:
                            args = json_repair.loads(tool_call.function.arguments)
                            tool_response = self.toolcall_session[name].tool_call(name, args)
                            history = self._append_history(history, tool_call, tool_response)
                            yield self._verbose_tool_use(name, args, tool_response)
                        except Exception as e:
                            logging.exception(msg=f"Wrong JSON argument format in LLM tool call response: {tool_call}")
                            history.append({"role": "tool", "tool_call_id": tool_call.id, "content": f"Tool call error: \n{tool_call}\nException:\n" + str(e)})
                            yield self._verbose_tool_use(name, {}, str(e))

            except Exception as e:
                e = self._exceptions(e, attempt)
                if e:
                    yield total_tokens
                    return

        yield total_tokens

    def chat_streamly(self, system, history, gen_conf):
        if system:
            history.insert(0, {"role": "system", "content": system})
        gen_conf = self._clean_conf(gen_conf)
        ans = ""
        total_tokens = 0
        reasoning_start = False
        try:
            response = self.client.chat.completions.create(model=self.model_name, messages=history, stream=True, **gen_conf)
            for resp in response:
                if not resp.choices:
                    continue
                if not resp.choices[0].delta.content:
                    resp.choices[0].delta.content = ""
                if hasattr(resp.choices[0].delta, "reasoning_content") and resp.choices[0].delta.reasoning_content:
                    ans = ""
                    if not reasoning_start:
                        reasoning_start = True
                        ans = "<think>"
                    ans += resp.choices[0].delta.reasoning_content + "</think>"
                else:
                    reasoning_start = False
                    ans = resp.choices[0].delta.content

                tol = self.total_token_count(resp)
                if not tol:
                    total_tokens += num_tokens_from_string(resp.choices[0].delta.content)
                else:
                    total_tokens += tol

                if resp.choices[0].finish_reason == "length":
                    if is_chinese(ans):
                        ans += LENGTH_NOTIFICATION_CN
                    else:
                        ans += LENGTH_NOTIFICATION_EN
                yield ans

        except openai.APIError as e:
            yield ans + "\n**ERROR**: " + str(e)

        yield total_tokens

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

    def _calculate_dynamic_ctx(self, history):
        """Calculate dynamic context window size"""

        def count_tokens(text):
            """Calculate token count for text"""
            # Simple calculation: 1 token per ASCII character
            # 2 tokens for non-ASCII characters (Chinese, Japanese, Korean, etc.)
            total = 0
            for char in text:
                if ord(char) < 128:  # ASCII characters
                    total += 1
                else:  # Non-ASCII characters (Chinese, Japanese, Korean, etc.)
                    total += 2
            return total

        # Calculate total tokens for all messages
        total_tokens = 0
        for message in history:
            content = message.get("content", "")
            # Calculate content tokens
            content_tokens = count_tokens(content)
            # Add role marker token overhead
            role_tokens = 4
            total_tokens += content_tokens + role_tokens

        # Apply 1.2x buffer ratio
        total_tokens_with_buffer = int(total_tokens * 1.2)

        if total_tokens_with_buffer <= 8192:
            ctx_size = 8192
        else:
            ctx_multiplier = (total_tokens_with_buffer // 8192) + 1
            ctx_size = ctx_multiplier * 8192

        return ctx_size


class GptTurbo(Base):
    _FACTORY_NAME = "OpenAI"

    def __init__(self, key, model_name="gpt-3.5-turbo", base_url="https://api.openai.com/v1", **kwargs):
        if not base_url:
            base_url = "https://api.openai.com/v1"
        super().__init__(key, model_name, base_url, **kwargs)


class MoonshotChat(Base):
    _FACTORY_NAME = "Moonshot"

    def __init__(self, key, model_name="moonshot-v1-8k", base_url="https://api.moonshot.cn/v1", **kwargs):
        if not base_url:
            base_url = "https://api.moonshot.cn/v1"
        super().__init__(key, model_name, base_url)


class XinferenceChat(Base):
    _FACTORY_NAME = "Xinference"

    def __init__(self, key=None, model_name="", base_url="", **kwargs):
        if not base_url:
            raise ValueError("Local llm url cannot be None")
        base_url = urljoin(base_url, "v1")
        super().__init__(key, model_name, base_url, **kwargs)


class HuggingFaceChat(Base):
    _FACTORY_NAME = "HuggingFace"

    def __init__(self, key=None, model_name="", base_url="", **kwargs):
        if not base_url:
            raise ValueError("Local llm url cannot be None")
        base_url = urljoin(base_url, "v1")
        super().__init__(key, model_name.split("___")[0], base_url, **kwargs)


class ModelScopeChat(Base):
    _FACTORY_NAME = "ModelScope"

    def __init__(self, key=None, model_name="", base_url="", **kwargs):
        if not base_url:
            raise ValueError("Local llm url cannot be None")
        base_url = urljoin(base_url, "v1")
        super().__init__(key, model_name.split("___")[0], base_url, **kwargs)


class DeepSeekChat(Base):
    _FACTORY_NAME = "DeepSeek"

    def __init__(self, key, model_name="deepseek-chat", base_url="https://api.deepseek.com/v1", **kwargs):
        if not base_url:
            base_url = "https://api.deepseek.com/v1"
        super().__init__(key, model_name, base_url, **kwargs)


class AzureChat(Base):
    _FACTORY_NAME = "Azure-OpenAI"

    def __init__(self, key, model_name, base_url, **kwargs):
        api_key = json.loads(key).get("api_key", "")
        api_version = json.loads(key).get("api_version", "2024-02-01")
        super().__init__(key, model_name, base_url, **kwargs)
        self.client = AzureOpenAI(api_key=api_key, azure_endpoint=base_url, api_version=api_version)
        self.model_name = model_name


class BaiChuanChat(Base):
    _FACTORY_NAME = "BaiChuan"

    def __init__(self, key, model_name="Baichuan3-Turbo", base_url="https://api.baichuan-ai.com/v1", **kwargs):
        if not base_url:
            base_url = "https://api.baichuan-ai.com/v1"
        super().__init__(key, model_name, base_url, **kwargs)

    @staticmethod
    def _format_params(params):
        return {
            "temperature": params.get("temperature", 0.3),
            "top_p": params.get("top_p", 0.85),
        }

    def _clean_conf(self, gen_conf):
        return {
            "temperature": gen_conf.get("temperature", 0.3),
            "top_p": gen_conf.get("top_p", 0.85),
        }

    def _chat(self, history, gen_conf):
        response = self.client.chat.completions.create(
            model=self.model_name,
            messages=history,
            extra_body={"tools": [{"type": "web_search", "web_search": {"enable": True, "search_mode": "performance_first"}}]},
            **gen_conf,
        )
        ans = response.choices[0].message.content.strip()
        if response.choices[0].finish_reason == "length":
            if is_chinese([ans]):
                ans += LENGTH_NOTIFICATION_CN
            else:
                ans += LENGTH_NOTIFICATION_EN
        return ans, self.total_token_count(response)

    def chat_streamly(self, system, history, gen_conf):
        if system:
            history.insert(0, {"role": "system", "content": system})
        if "max_tokens" in gen_conf:
            del gen_conf["max_tokens"]
        ans = ""
        total_tokens = 0
        try:
            response = self.client.chat.completions.create(
                model=self.model_name,
                messages=history,
                extra_body={"tools": [{"type": "web_search", "web_search": {"enable": True, "search_mode": "performance_first"}}]},
                stream=True,
                **self._format_params(gen_conf),
            )
            for resp in response:
                if not resp.choices:
                    continue
                if not resp.choices[0].delta.content:
                    resp.choices[0].delta.content = ""
                ans = resp.choices[0].delta.content
                tol = self.total_token_count(resp)
                if not tol:
                    total_tokens += num_tokens_from_string(resp.choices[0].delta.content)
                else:
                    total_tokens = tol
                if resp.choices[0].finish_reason == "length":
                    if is_chinese([ans]):
                        ans += LENGTH_NOTIFICATION_CN
                    else:
                        ans += LENGTH_NOTIFICATION_EN
                yield ans

        except Exception as e:
            yield ans + "\n**ERROR**: " + str(e)

        yield total_tokens


class QWenChat(Base):
    _FACTORY_NAME = "Tongyi-Qianwen"

    def __init__(self, key, model_name=Generation.Models.qwen_turbo, base_url=None, **kwargs):
        if not base_url:
            base_url = "https://dashscope.aliyuncs.com/compatible-mode/v1"
        super().__init__(key, model_name, base_url=base_url, **kwargs)
        return


class ZhipuChat(Base):
    _FACTORY_NAME = "ZHIPU-AI"

    def __init__(self, key, model_name="glm-3-turbo", base_url=None, **kwargs):
        super().__init__(key, model_name, base_url=base_url, **kwargs)

        self.client = ZhipuAI(api_key=key)
        self.model_name = model_name

    def _clean_conf(self, gen_conf):
        if "max_tokens" in gen_conf:
            del gen_conf["max_tokens"]
        if "presence_penalty" in gen_conf:
            del gen_conf["presence_penalty"]
        if "frequency_penalty" in gen_conf:
            del gen_conf["frequency_penalty"]
        return gen_conf

    def chat_with_tools(self, system: str, history: list, gen_conf: dict):
        if "presence_penalty" in gen_conf:
            del gen_conf["presence_penalty"]
        if "frequency_penalty" in gen_conf:
            del gen_conf["frequency_penalty"]

        return super().chat_with_tools(system, history, gen_conf)

    def chat_streamly(self, system, history, gen_conf):
        if system:
            history.insert(0, {"role": "system", "content": system})
        if "max_tokens" in gen_conf:
            del gen_conf["max_tokens"]
        if "presence_penalty" in gen_conf:
            del gen_conf["presence_penalty"]
        if "frequency_penalty" in gen_conf:
            del gen_conf["frequency_penalty"]
        ans = ""
        tk_count = 0
        try:
            response = self.client.chat.completions.create(model=self.model_name, messages=history, stream=True, **gen_conf)
            for resp in response:
                if not resp.choices[0].delta.content:
                    continue
                delta = resp.choices[0].delta.content
                ans = delta
                if resp.choices[0].finish_reason == "length":
                    if is_chinese(ans):
                        ans += LENGTH_NOTIFICATION_CN
                    else:
                        ans += LENGTH_NOTIFICATION_EN
                    tk_count = self.total_token_count(resp)
                if resp.choices[0].finish_reason == "stop":
                    tk_count = self.total_token_count(resp)
                yield ans
        except Exception as e:
            yield ans + "\n**ERROR**: " + str(e)

        yield tk_count

    def chat_streamly_with_tools(self, system: str, history: list, gen_conf: dict):
        if "presence_penalty" in gen_conf:
            del gen_conf["presence_penalty"]
        if "frequency_penalty" in gen_conf:
            del gen_conf["frequency_penalty"]

        return super().chat_streamly_with_tools(system, history, gen_conf)


class OllamaChat(Base):
    _FACTORY_NAME = "Ollama"

    def __init__(self, key, model_name, base_url=None, **kwargs):
        super().__init__(key, model_name, base_url=base_url, **kwargs)

        self.client = Client(host=base_url) if not key or key == "x" else Client(host=base_url, headers={"Authorization": f"Bearer {key}"})
        self.model_name = model_name

    def _clean_conf(self, gen_conf):
        options = {}
        if "max_tokens" in gen_conf:
            options["num_predict"] = gen_conf["max_tokens"]
        for k in ["temperature", "top_p", "presence_penalty", "frequency_penalty"]:
            if k not in gen_conf:
                continue
            options[k] = gen_conf[k]
        return options

    def _chat(self, history, gen_conf):
        # Calculate context size
        ctx_size = self._calculate_dynamic_ctx(history)

        gen_conf["num_ctx"] = ctx_size
        response = self.client.chat(model=self.model_name, messages=history, options=gen_conf, keep_alive=-1)
        ans = response["message"]["content"].strip()
        token_count = response.get("eval_count", 0) + response.get("prompt_eval_count", 0)
        return ans, token_count

    def chat_streamly(self, system, history, gen_conf):
        if system:
            history.insert(0, {"role": "system", "content": system})
        if "max_tokens" in gen_conf:
            del gen_conf["max_tokens"]
        try:
            # Calculate context size
            ctx_size = self._calculate_dynamic_ctx(history)
            options = {"num_ctx": ctx_size}
            if "temperature" in gen_conf:
                options["temperature"] = gen_conf["temperature"]
            if "max_tokens" in gen_conf:
                options["num_predict"] = gen_conf["max_tokens"]
            if "top_p" in gen_conf:
                options["top_p"] = gen_conf["top_p"]
            if "presence_penalty" in gen_conf:
                options["presence_penalty"] = gen_conf["presence_penalty"]
            if "frequency_penalty" in gen_conf:
                options["frequency_penalty"] = gen_conf["frequency_penalty"]

            ans = ""
            try:
                response = self.client.chat(model=self.model_name, messages=history, stream=True, options=options, keep_alive=-1)
                for resp in response:
                    if resp["done"]:
                        token_count = resp.get("prompt_eval_count", 0) + resp.get("eval_count", 0)
                        yield token_count
                    ans = resp["message"]["content"]
                    yield ans
            except Exception as e:
                yield ans + "\n**ERROR**: " + str(e)
            yield 0
        except Exception as e:
            yield "**ERROR**: " + str(e)
            yield 0


class LocalAIChat(Base):
    _FACTORY_NAME = "LocalAI"

    def __init__(self, key, model_name, base_url=None, **kwargs):
        super().__init__(key, model_name, base_url=base_url, **kwargs)

        if not base_url:
            raise ValueError("Local llm url cannot be None")
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key="empty", base_url=base_url)
        self.model_name = model_name.split("___")[0]


class LocalLLM(Base):
    def __init__(self, key, model_name, base_url=None, **kwargs):
        super().__init__(key, model_name, base_url=base_url, **kwargs)
        from jina import Client

        self.client = Client(port=12345, protocol="grpc", asyncio=True)

    def _prepare_prompt(self, system, history, gen_conf):
        from rag.svr.jina_server import Prompt

        if system:
            history.insert(0, {"role": "system", "content": system})
        return Prompt(message=history, gen_conf=gen_conf)

    def _stream_response(self, endpoint, prompt):
        from rag.svr.jina_server import Generation

        answer = ""
        try:
            res = self.client.stream_doc(on=endpoint, inputs=prompt, return_type=Generation)
            loop = asyncio.get_event_loop()
            try:
                while True:
                    answer = loop.run_until_complete(res.__anext__()).text
                    yield answer
            except StopAsyncIteration:
                pass
        except Exception as e:
            yield answer + "\n**ERROR**: " + str(e)
        yield num_tokens_from_string(answer)

    def chat(self, system, history, gen_conf):
        if "max_tokens" in gen_conf:
            del gen_conf["max_tokens"]
        prompt = self._prepare_prompt(system, history, gen_conf)
        chat_gen = self._stream_response("/chat", prompt)
        ans = next(chat_gen)
        total_tokens = next(chat_gen)
        return ans, total_tokens

    def chat_streamly(self, system, history, gen_conf):
        if "max_tokens" in gen_conf:
            del gen_conf["max_tokens"]
        prompt = self._prepare_prompt(system, history, gen_conf)
        return self._stream_response("/stream", prompt)


class VolcEngineChat(Base):
    _FACTORY_NAME = "VolcEngine"

    def __init__(self, key, model_name, base_url="https://ark.cn-beijing.volces.com/api/v3", **kwargs):
        """
        Since do not want to modify the original database fields, and the VolcEngine authentication method is quite special,
        Assemble ark_api_key, ep_id into api_key, store it as a dictionary type, and parse it for use
        model_name is for display only
        """
        base_url = base_url if base_url else "https://ark.cn-beijing.volces.com/api/v3"
        ark_api_key = json.loads(key).get("ark_api_key", "")
        model_name = json.loads(key).get("ep_id", "") + json.loads(key).get("endpoint_id", "")
        super().__init__(ark_api_key, model_name, base_url, **kwargs)


class MiniMaxChat(Base):
    _FACTORY_NAME = "MiniMax"

    def __init__(self, key, model_name, base_url="https://api.minimax.chat/v1/text/chatcompletion_v2", **kwargs):
        super().__init__(key, model_name, base_url=base_url, **kwargs)

        if not base_url:
            base_url = "https://api.minimax.chat/v1/text/chatcompletion_v2"
        self.base_url = base_url
        self.model_name = model_name
        self.api_key = key

    def _clean_conf(self, gen_conf):
        for k in list(gen_conf.keys()):
            if k not in ["temperature", "top_p", "max_tokens"]:
                del gen_conf[k]
        return gen_conf

    def _chat(self, history, gen_conf):
        headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
        }
        payload = json.dumps({"model": self.model_name, "messages": history, **gen_conf})
        response = requests.request("POST", url=self.base_url, headers=headers, data=payload)
        response = response.json()
        ans = response["choices"][0]["message"]["content"].strip()
        if response["choices"][0]["finish_reason"] == "length":
            if is_chinese(ans):
                ans += LENGTH_NOTIFICATION_CN
            else:
                ans += LENGTH_NOTIFICATION_EN
        return ans, self.total_token_count(response)

    def chat_streamly(self, system, history, gen_conf):
        if system:
            history.insert(0, {"role": "system", "content": system})
        for k in list(gen_conf.keys()):
            if k not in ["temperature", "top_p", "max_tokens"]:
                del gen_conf[k]
        ans = ""
        total_tokens = 0
        try:
            headers = {
                "Authorization": f"Bearer {self.api_key}",
                "Content-Type": "application/json",
            }
            payload = json.dumps(
                {
                    "model": self.model_name,
                    "messages": history,
                    "stream": True,
                    **gen_conf,
                }
            )
            response = requests.request(
                "POST",
                url=self.base_url,
                headers=headers,
                data=payload,
            )
            for resp in response.text.split("\n\n")[:-1]:
                resp = json.loads(resp[6:])
                text = ""
                if "choices" in resp and "delta" in resp["choices"][0]:
                    text = resp["choices"][0]["delta"]["content"]
                ans = text
                tol = self.total_token_count(resp)
                if not tol:
                    total_tokens += num_tokens_from_string(text)
                else:
                    total_tokens = tol
                yield ans

        except Exception as e:
            yield ans + "\n**ERROR**: " + str(e)

        yield total_tokens


class MistralChat(Base):
    _FACTORY_NAME = "Mistral"

    def __init__(self, key, model_name, base_url=None, **kwargs):
        super().__init__(key, model_name, base_url=base_url, **kwargs)

        from mistralai.client import MistralClient

        self.client = MistralClient(api_key=key)
        self.model_name = model_name

    def _clean_conf(self, gen_conf):
        for k in list(gen_conf.keys()):
            if k not in ["temperature", "top_p", "max_tokens"]:
                del gen_conf[k]
        return gen_conf

    def _chat(self, history, gen_conf):
        response = self.client.chat(model=self.model_name, messages=history, **gen_conf)
        ans = response.choices[0].message.content
        if response.choices[0].finish_reason == "length":
            if is_chinese(ans):
                ans += LENGTH_NOTIFICATION_CN
            else:
                ans += LENGTH_NOTIFICATION_EN
        return ans, self.total_token_count(response)

    def chat_streamly(self, system, history, gen_conf):
        if system:
            history.insert(0, {"role": "system", "content": system})
        for k in list(gen_conf.keys()):
            if k not in ["temperature", "top_p", "max_tokens"]:
                del gen_conf[k]
        ans = ""
        total_tokens = 0
        try:
            response = self.client.chat_stream(model=self.model_name, messages=history, **gen_conf)
            for resp in response:
                if not resp.choices or not resp.choices[0].delta.content:
                    continue
                ans = resp.choices[0].delta.content
                total_tokens += 1
                if resp.choices[0].finish_reason == "length":
                    if is_chinese(ans):
                        ans += LENGTH_NOTIFICATION_CN
                    else:
                        ans += LENGTH_NOTIFICATION_EN
                yield ans

        except openai.APIError as e:
            yield ans + "\n**ERROR**: " + str(e)

        yield total_tokens


class BedrockChat(Base):
    _FACTORY_NAME = "Bedrock"

    def __init__(self, key, model_name, base_url=None, **kwargs):
        super().__init__(key, model_name, base_url=base_url, **kwargs)

        import boto3

        self.bedrock_ak = json.loads(key).get("bedrock_ak", "")
        self.bedrock_sk = json.loads(key).get("bedrock_sk", "")
        self.bedrock_region = json.loads(key).get("bedrock_region", "")
        self.model_name = model_name

        if self.bedrock_ak == "" or self.bedrock_sk == "" or self.bedrock_region == "":
            # Try to create a client using the default credentials (AWS_PROFILE, AWS_DEFAULT_REGION, etc.)
            self.client = boto3.client("bedrock-runtime")
        else:
            self.client = boto3.client(service_name="bedrock-runtime", region_name=self.bedrock_region, aws_access_key_id=self.bedrock_ak, aws_secret_access_key=self.bedrock_sk)

    def _clean_conf(self, gen_conf):
        for k in list(gen_conf.keys()):
            if k not in ["temperature"]:
                del gen_conf[k]
        return gen_conf

    def _chat(self, history, gen_conf):
        system = history[0]["content"] if history and history[0]["role"] == "system" else ""
        hist = []
        for item in history:
            if item["role"] == "system":
                continue
            hist.append(deepcopy(item))
            if not isinstance(hist[-1]["content"], list) and not isinstance(hist[-1]["content"], tuple):
                hist[-1]["content"] = [{"text": hist[-1]["content"]}]
        # Send the message to the model, using a basic inference configuration.
        response = self.client.converse(
            modelId=self.model_name,
            messages=hist,
            inferenceConfig=gen_conf,
            system=[{"text": (system if system else "Answer the user's message.")}],
        )

        # Extract and print the response text.
        ans = response["output"]["message"]["content"][0]["text"]
        return ans, num_tokens_from_string(ans)

    def chat_streamly(self, system, history, gen_conf):
        from botocore.exceptions import ClientError

        for k in list(gen_conf.keys()):
            if k not in ["temperature"]:
                del gen_conf[k]
        for item in history:
            if not isinstance(item["content"], list) and not isinstance(item["content"], tuple):
                item["content"] = [{"text": item["content"]}]

        if self.model_name.split(".")[0] == "ai21":
            try:
                response = self.client.converse(modelId=self.model_name, messages=history, inferenceConfig=gen_conf, system=[{"text": (system if system else "Answer the user's message.")}])
                ans = response["output"]["message"]["content"][0]["text"]
                return ans, num_tokens_from_string(ans)

            except (ClientError, Exception) as e:
                return f"ERROR: Can't invoke '{self.model_name}'. Reason: {e}", 0

        ans = ""
        try:
            # Send the message to the model, using a basic inference configuration.
            streaming_response = self.client.converse_stream(
                modelId=self.model_name, messages=history, inferenceConfig=gen_conf, system=[{"text": (system if system else "Answer the user's message.")}]
            )

            # Extract and print the streamed response text in real-time.
            for resp in streaming_response["stream"]:
                if "contentBlockDelta" in resp:
                    ans = resp["contentBlockDelta"]["delta"]["text"]
                    yield ans

        except (ClientError, Exception) as e:
            yield ans + f"ERROR: Can't invoke '{self.model_name}'. Reason: {e}"

        yield num_tokens_from_string(ans)


class GeminiChat(Base):
    _FACTORY_NAME = "Gemini"

    def __init__(self, key, model_name, base_url=None, **kwargs):
        super().__init__(key, model_name, base_url=base_url, **kwargs)

        from google.generativeai import GenerativeModel, client

        client.configure(api_key=key)
        _client = client.get_default_generative_client()
        self.model_name = "models/" + model_name
        self.model = GenerativeModel(model_name=self.model_name)
        self.model._client = _client

    def _clean_conf(self, gen_conf):
        for k in list(gen_conf.keys()):
            if k not in ["temperature", "top_p", "max_tokens"]:
                del gen_conf[k]
        return gen_conf

    def _chat(self, history, gen_conf):
        from google.generativeai.types import content_types

        system = history[0]["content"] if history and history[0]["role"] == "system" else ""
        hist = []
        for item in history:
            if item["role"] == "system":
                continue
            hist.append(deepcopy(item))
            item = hist[-1]
            if "role" in item and item["role"] == "assistant":
                item["role"] = "model"
            if "role" in item and item["role"] == "system":
                item["role"] = "user"
            if "content" in item:
                item["parts"] = item.pop("content")

        if system:
            self.model._system_instruction = content_types.to_content(system)
        response = self.model.generate_content(hist, generation_config=gen_conf)
        ans = response.text
        return ans, response.usage_metadata.total_token_count

    def chat_streamly(self, system, history, gen_conf):
        from google.generativeai.types import content_types

        gen_conf = self._clean_conf(gen_conf)
        if system:
            self.model._system_instruction = content_types.to_content(system)
        for item in history:
            if "role" in item and item["role"] == "assistant":
                item["role"] = "model"
            if "content" in item:
                item["parts"] = item.pop("content")
        ans = ""
        try:
            response = self.model.generate_content(history, generation_config=gen_conf, stream=True)
            for resp in response:
                ans = resp.text
                yield ans

            yield response._chunks[-1].usage_metadata.total_token_count
        except Exception as e:
            yield ans + "\n**ERROR**: " + str(e)

        yield 0


class GroqChat(Base):
    _FACTORY_NAME = "Groq"

    def __init__(self, key, model_name, base_url=None, **kwargs):
        super().__init__(key, model_name, base_url=base_url, **kwargs)

        from groq import Groq

        self.client = Groq(api_key=key)
        self.model_name = model_name

    def _clean_conf(self, gen_conf):
        for k in list(gen_conf.keys()):
            if k not in ["temperature", "top_p", "max_tokens"]:
                del gen_conf[k]
        return gen_conf

    def chat_streamly(self, system, history, gen_conf):
        if system:
            history.insert(0, {"role": "system", "content": system})
        for k in list(gen_conf.keys()):
            if k not in ["temperature", "top_p", "max_tokens"]:
                del gen_conf[k]
        ans = ""
        total_tokens = 0
        try:
            response = self.client.chat.completions.create(model=self.model_name, messages=history, stream=True, **gen_conf)
            for resp in response:
                if not resp.choices or not resp.choices[0].delta.content:
                    continue
                ans = resp.choices[0].delta.content
                total_tokens += 1
                if resp.choices[0].finish_reason == "length":
                    if is_chinese(ans):
                        ans += LENGTH_NOTIFICATION_CN
                    else:
                        ans += LENGTH_NOTIFICATION_EN
                yield ans

        except Exception as e:
            yield ans + "\n**ERROR**: " + str(e)

        yield total_tokens


## openrouter
class OpenRouterChat(Base):
    _FACTORY_NAME = "OpenRouter"

    def __init__(self, key, model_name, base_url="https://openrouter.ai/api/v1", **kwargs):
        if not base_url:
            base_url = "https://openrouter.ai/api/v1"
        super().__init__(key, model_name, base_url, **kwargs)


class StepFunChat(Base):
    _FACTORY_NAME = "StepFun"

    def __init__(self, key, model_name, base_url="https://api.stepfun.com/v1", **kwargs):
        if not base_url:
            base_url = "https://api.stepfun.com/v1"
        super().__init__(key, model_name, base_url, **kwargs)


class NvidiaChat(Base):
    _FACTORY_NAME = "NVIDIA"

    def __init__(self, key, model_name, base_url="https://integrate.api.nvidia.com/v1", **kwargs):
        if not base_url:
            base_url = "https://integrate.api.nvidia.com/v1"
        super().__init__(key, model_name, base_url, **kwargs)


class LmStudioChat(Base):
    _FACTORY_NAME = "LM-Studio"

    def __init__(self, key, model_name, base_url, **kwargs):
        if not base_url:
            raise ValueError("Local llm url cannot be None")
        base_url = urljoin(base_url, "v1")
        super().__init__(key, model_name, base_url, **kwargs)
        self.client = OpenAI(api_key="lm-studio", base_url=base_url)
        self.model_name = model_name


class OpenAI_APIChat(Base):
    _FACTORY_NAME = ["VLLM", "OpenAI-API-Compatible"]

    def __init__(self, key, model_name, base_url):
        if not base_url:
            raise ValueError("url cannot be None")
        model_name = model_name.split("___")[0]
        super().__init__(key, model_name, base_url)


class PPIOChat(Base):
    _FACTORY_NAME = "PPIO"

    def __init__(self, key, model_name, base_url="https://api.ppinfra.com/v3/openai", **kwargs):
        if not base_url:
            base_url = "https://api.ppinfra.com/v3/openai"
        super().__init__(key, model_name, base_url, **kwargs)


class CoHereChat(Base):
    _FACTORY_NAME = "Cohere"

    def __init__(self, key, model_name, base_url=None, **kwargs):
        super().__init__(key, model_name, base_url=base_url, **kwargs)

        from cohere import Client

        self.client = Client(api_key=key)
        self.model_name = model_name

    def _clean_conf(self, gen_conf):
        if "max_tokens" in gen_conf:
            del gen_conf["max_tokens"]
        if "top_p" in gen_conf:
            gen_conf["p"] = gen_conf.pop("top_p")
        if "frequency_penalty" in gen_conf and "presence_penalty" in gen_conf:
            gen_conf.pop("presence_penalty")
        return gen_conf

    def _chat(self, history, gen_conf):
        hist = []
        for item in history:
            hist.append(deepcopy(item))
            item = hist[-1]
            if "role" in item and item["role"] == "user":
                item["role"] = "USER"
            if "role" in item and item["role"] == "assistant":
                item["role"] = "CHATBOT"
            if "content" in item:
                item["message"] = item.pop("content")
        mes = hist.pop()["message"]
        response = self.client.chat(model=self.model_name, chat_history=hist, message=mes, **gen_conf)
        ans = response.text
        if response.finish_reason == "MAX_TOKENS":
            ans += "...\nFor the content length reason, it stopped, continue?" if is_english([ans]) else "······\n由于长度的原因，回答被截断了，要继续吗？"
        return (
            ans,
            response.meta.tokens.input_tokens + response.meta.tokens.output_tokens,
        )

    def chat_streamly(self, system, history, gen_conf):
        if system:
            history.insert(0, {"role": "system", "content": system})
        if "max_tokens" in gen_conf:
            del gen_conf["max_tokens"]
        if "top_p" in gen_conf:
            gen_conf["p"] = gen_conf.pop("top_p")
        if "frequency_penalty" in gen_conf and "presence_penalty" in gen_conf:
            gen_conf.pop("presence_penalty")
        for item in history:
            if "role" in item and item["role"] == "user":
                item["role"] = "USER"
            if "role" in item and item["role"] == "assistant":
                item["role"] = "CHATBOT"
            if "content" in item:
                item["message"] = item.pop("content")
        mes = history.pop()["message"]
        ans = ""
        total_tokens = 0
        try:
            response = self.client.chat_stream(model=self.model_name, chat_history=history, message=mes, **gen_conf)
            for resp in response:
                if resp.event_type == "text-generation":
                    ans = resp.text
                    total_tokens += num_tokens_from_string(resp.text)
                elif resp.event_type == "stream-end":
                    if resp.finish_reason == "MAX_TOKENS":
                        ans += "...\nFor the content length reason, it stopped, continue?" if is_english([ans]) else "······\n由于长度的原因，回答被截断了，要继续吗？"
                yield ans

        except Exception as e:
            yield ans + "\n**ERROR**: " + str(e)

        yield total_tokens


class LeptonAIChat(Base):
    _FACTORY_NAME = "LeptonAI"

    def __init__(self, key, model_name, base_url=None, **kwargs):
        if not base_url:
            base_url = urljoin("https://" + model_name + ".lepton.run", "api/v1")
        super().__init__(key, model_name, base_url, **kwargs)


class TogetherAIChat(Base):
    _FACTORY_NAME = "TogetherAI"

    def __init__(self, key, model_name, base_url="https://api.together.xyz/v1", **kwargs):
        if not base_url:
            base_url = "https://api.together.xyz/v1"
        super().__init__(key, model_name, base_url, **kwargs)


class PerfXCloudChat(Base):
    _FACTORY_NAME = "PerfXCloud"

    def __init__(self, key, model_name, base_url="https://cloud.perfxlab.cn/v1", **kwargs):
        if not base_url:
            base_url = "https://cloud.perfxlab.cn/v1"
        super().__init__(key, model_name, base_url, **kwargs)


class UpstageChat(Base):
    _FACTORY_NAME = "Upstage"

    def __init__(self, key, model_name, base_url="https://api.upstage.ai/v1/solar", **kwargs):
        if not base_url:
            base_url = "https://api.upstage.ai/v1/solar"
        super().__init__(key, model_name, base_url, **kwargs)


class NovitaAIChat(Base):
    _FACTORY_NAME = "NovitaAI"

    def __init__(self, key, model_name, base_url="https://api.novita.ai/v3/openai", **kwargs):
        if not base_url:
            base_url = "https://api.novita.ai/v3/openai"
        super().__init__(key, model_name, base_url, **kwargs)


class SILICONFLOWChat(Base):
    _FACTORY_NAME = "SILICONFLOW"

    def __init__(self, key, model_name, base_url="https://api.siliconflow.cn/v1", **kwargs):
        if not base_url:
            base_url = "https://api.siliconflow.cn/v1"
        super().__init__(key, model_name, base_url, **kwargs)


class YiChat(Base):
    _FACTORY_NAME = "01.AI"

    def __init__(self, key, model_name, base_url="https://api.lingyiwanwu.com/v1", **kwargs):
        if not base_url:
            base_url = "https://api.lingyiwanwu.com/v1"
        super().__init__(key, model_name, base_url, **kwargs)


class GiteeChat(Base):
    _FACTORY_NAME = "GiteeAI"

    def __init__(self, key, model_name, base_url="https://ai.gitee.com/v1/", **kwargs):
        if not base_url:
            base_url = "https://ai.gitee.com/v1/"
        super().__init__(key, model_name, base_url, **kwargs)


class ReplicateChat(Base):
    _FACTORY_NAME = "Replicate"

    def __init__(self, key, model_name, base_url=None, **kwargs):
        super().__init__(key, model_name, base_url=base_url, **kwargs)

        from replicate.client import Client

        self.model_name = model_name
        self.client = Client(api_token=key)

    def _chat(self, history, gen_conf):
        system = history[0]["content"] if history and history[0]["role"] == "system" else ""
        prompt = "\n".join([item["role"] + ":" + item["content"] for item in history[-5:] if item["role"] != "system"])
        response = self.client.run(
            self.model_name,
            input={"system_prompt": system, "prompt": prompt, **gen_conf},
        )
        ans = "".join(response)
        return ans, num_tokens_from_string(ans)

    def chat_streamly(self, system, history, gen_conf):
        if "max_tokens" in gen_conf:
            del gen_conf["max_tokens"]
        prompt = "\n".join([item["role"] + ":" + item["content"] for item in history[-5:]])
        ans = ""
        try:
            response = self.client.run(
                self.model_name,
                input={"system_prompt": system, "prompt": prompt, **gen_conf},
            )
            for resp in response:
                ans = resp
                yield ans

        except Exception as e:
            yield ans + "\n**ERROR**: " + str(e)

        yield num_tokens_from_string(ans)


class HunyuanChat(Base):
    _FACTORY_NAME = "Tencent Hunyuan"

    def __init__(self, key, model_name, base_url=None, **kwargs):
        super().__init__(key, model_name, base_url=base_url, **kwargs)

        from tencentcloud.common import credential
        from tencentcloud.hunyuan.v20230901 import hunyuan_client

        key = json.loads(key)
        sid = key.get("hunyuan_sid", "")
        sk = key.get("hunyuan_sk", "")
        cred = credential.Credential(sid, sk)
        self.model_name = model_name
        self.client = hunyuan_client.HunyuanClient(cred, "")

    def _clean_conf(self, gen_conf):
        _gen_conf = {}
        if "temperature" in gen_conf:
            _gen_conf["Temperature"] = gen_conf["temperature"]
        if "top_p" in gen_conf:
            _gen_conf["TopP"] = gen_conf["top_p"]
        return _gen_conf

    def _chat(self, history, gen_conf):
        from tencentcloud.hunyuan.v20230901 import models

        hist = [{k.capitalize(): v for k, v in item.items()} for item in history]
        req = models.ChatCompletionsRequest()
        params = {"Model": self.model_name, "Messages": hist, **gen_conf}
        req.from_json_string(json.dumps(params))
        response = self.client.ChatCompletions(req)
        ans = response.Choices[0].Message.Content
        return ans, response.Usage.TotalTokens

    def chat_streamly(self, system, history, gen_conf):
        from tencentcloud.common.exception.tencent_cloud_sdk_exception import (
            TencentCloudSDKException,
        )
        from tencentcloud.hunyuan.v20230901 import models

        _gen_conf = {}
        _history = [{k.capitalize(): v for k, v in item.items()} for item in history]
        if system:
            _history.insert(0, {"Role": "system", "Content": system})
        if "max_tokens" in gen_conf:
            del gen_conf["max_tokens"]
        if "temperature" in gen_conf:
            _gen_conf["Temperature"] = gen_conf["temperature"]
        if "top_p" in gen_conf:
            _gen_conf["TopP"] = gen_conf["top_p"]
        req = models.ChatCompletionsRequest()
        params = {
            "Model": self.model_name,
            "Messages": _history,
            "Stream": True,
            **_gen_conf,
        }
        req.from_json_string(json.dumps(params))
        ans = ""
        total_tokens = 0
        try:
            response = self.client.ChatCompletions(req)
            for resp in response:
                resp = json.loads(resp["data"])
                if not resp["Choices"] or not resp["Choices"][0]["Delta"]["Content"]:
                    continue
                ans = resp["Choices"][0]["Delta"]["Content"]
                total_tokens += 1

                yield ans

        except TencentCloudSDKException as e:
            yield ans + "\n**ERROR**: " + str(e)

        yield total_tokens


class SparkChat(Base):
    _FACTORY_NAME = "XunFei Spark"

    def __init__(self, key, model_name, base_url="https://spark-api-open.xf-yun.com/v1", **kwargs):
        if not base_url:
            base_url = "https://spark-api-open.xf-yun.com/v1"
        model2version = {
            "Spark-Max": "generalv3.5",
            "Spark-Lite": "general",
            "Spark-Pro": "generalv3",
            "Spark-Pro-128K": "pro-128k",
            "Spark-4.0-Ultra": "4.0Ultra",
        }
        version2model = {v: k for k, v in model2version.items()}
        assert model_name in model2version or model_name in version2model, f"The given model name is not supported yet. Support: {list(model2version.keys())}"
        if model_name in model2version:
            model_version = model2version[model_name]
        else:
            model_version = model_name
        super().__init__(key, model_version, base_url, **kwargs)


class BaiduYiyanChat(Base):
    _FACTORY_NAME = "BaiduYiyan"

    def __init__(self, key, model_name, base_url=None, **kwargs):
        super().__init__(key, model_name, base_url=base_url, **kwargs)

        import qianfan

        key = json.loads(key)
        ak = key.get("yiyan_ak", "")
        sk = key.get("yiyan_sk", "")
        self.client = qianfan.ChatCompletion(ak=ak, sk=sk)
        self.model_name = model_name.lower()

    def _clean_conf(self, gen_conf):
        gen_conf["penalty_score"] = ((gen_conf.get("presence_penalty", 0) + gen_conf.get("frequency_penalty", 0)) / 2) + 1
        if "max_tokens" in gen_conf:
            del gen_conf["max_tokens"]
        return gen_conf

    def _chat(self, history, gen_conf):
        system = history[0]["content"] if history and history[0]["role"] == "system" else ""
        response = self.client.do(model=self.model_name, messages=[h for h in history if h["role"] != "system"], system=system, **gen_conf).body
        ans = response["result"]
        return ans, self.total_token_count(response)

    def chat_streamly(self, system, history, gen_conf):
        gen_conf["penalty_score"] = ((gen_conf.get("presence_penalty", 0) + gen_conf.get("frequency_penalty", 0)) / 2) + 1
        if "max_tokens" in gen_conf:
            del gen_conf["max_tokens"]
        ans = ""
        total_tokens = 0

        try:
            response = self.client.do(model=self.model_name, messages=history, system=system, stream=True, **gen_conf)
            for resp in response:
                resp = resp.body
                ans = resp["result"]
                total_tokens = self.total_token_count(resp)

                yield ans

        except Exception as e:
            return ans + "\n**ERROR**: " + str(e), 0

        yield total_tokens


class AnthropicChat(Base):
    _FACTORY_NAME = "Anthropic"

    def __init__(self, key, model_name, base_url="https://api.anthropic.com/v1/", **kwargs):
        if not base_url:
            base_url = "https://api.anthropic.com/v1/"
        super().__init__(key, model_name, base_url=base_url, **kwargs)


class GoogleChat(Base):
    _FACTORY_NAME = "Google Cloud"

    def __init__(self, key, model_name, base_url=None, **kwargs):
        super().__init__(key, model_name, base_url=base_url, **kwargs)

        import base64

        from google.oauth2 import service_account

        key = json.loads(key)
        access_token = json.loads(base64.b64decode(key.get("google_service_account_key", "")))
        project_id = key.get("google_project_id", "")
        region = key.get("google_region", "")

        scopes = ["https://www.googleapis.com/auth/cloud-platform"]
        self.model_name = model_name

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

    def _clean_conf(self, gen_conf):
        if "claude" in self.model_name:
            if "max_tokens" in gen_conf:
                del gen_conf["max_tokens"]
        else:
            if "max_tokens" in gen_conf:
                gen_conf["max_output_tokens"] = gen_conf["max_tokens"]
            for k in list(gen_conf.keys()):
                if k not in ["temperature", "top_p", "max_output_tokens"]:
                    del gen_conf[k]
        return gen_conf

    def _chat(self, history, gen_conf):
        system = history[0]["content"] if history and history[0]["role"] == "system" else ""
        if "claude" in self.model_name:
            response = self.client.messages.create(
                model=self.model_name,
                messages=[h for h in history if h["role"] != "system"],
                system=system,
                stream=False,
                **gen_conf,
            ).json()
            ans = response["content"][0]["text"]
            if response["stop_reason"] == "max_tokens":
                ans += "...\nFor the content length reason, it stopped, continue?" if is_english([ans]) else "······\n由于长度的原因，回答被截断了，要继续吗？"
            return (
                ans,
                response["usage"]["input_tokens"] + response["usage"]["output_tokens"],
            )

        self.client._system_instruction = system
        hist = []
        for item in history:
            if item["role"] == "system":
                continue
            hist.append(deepcopy(item))
            item = hist[-1]
            if "role" in item and item["role"] == "assistant":
                item["role"] = "model"
            if "content" in item:
                item["parts"] = [
                    {
                        "text": item.pop("content"),
                    }
                ]

        response = self.client.generate_content(hist, generation_config=gen_conf)
        ans = response.text
        return ans, response.usage_metadata.total_token_count

    def chat_streamly(self, system, history, gen_conf):
        if "claude" in self.model_name:
            if "max_tokens" in gen_conf:
                del gen_conf["max_tokens"]
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
                for res in response.iter_lines():
                    res = res.decode("utf-8")
                    if "content_block_delta" in res and "data" in res:
                        text = json.loads(res[6:])["delta"]["text"]
                        ans = text
                        total_tokens += num_tokens_from_string(text)
            except Exception as e:
                yield ans + "\n**ERROR**: " + str(e)

            yield total_tokens
        else:
            self.client._system_instruction = system
            if "max_tokens" in gen_conf:
                gen_conf["max_output_tokens"] = gen_conf["max_tokens"]
            for k in list(gen_conf.keys()):
                if k not in ["temperature", "top_p", "max_output_tokens"]:
                    del gen_conf[k]
            for item in history:
                if "role" in item and item["role"] == "assistant":
                    item["role"] = "model"
                if "content" in item:
                    item["parts"] = item.pop("content")
            ans = ""
            try:
                response = self.model.generate_content(history, generation_config=gen_conf, stream=True)
                for resp in response:
                    ans = resp.text
                    yield ans

            except Exception as e:
                yield ans + "\n**ERROR**: " + str(e)

            yield response._chunks[-1].usage_metadata.total_token_count


class GPUStackChat(Base):
    _FACTORY_NAME = "GPUStack"

    def __init__(self, key=None, model_name="", base_url="", **kwargs):
        if not base_url:
            raise ValueError("Local llm url cannot be None")
        base_url = urljoin(base_url, "v1")
        super().__init__(key, model_name, base_url, **kwargs)
