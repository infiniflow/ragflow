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
import asyncio
import json
import logging
import os
import re
from copy import deepcopy
from typing import Any, AsyncGenerator
import json_repair
from functools import partial
from common.constants import LLMType
from api.db.services.dialog_service import _stream_with_think_delta
from api.db.services.llm_service import LLMBundle
from api.db.joint_services.tenant_model_service import get_model_config_from_provider_instance, get_model_type_by_name
from agent.component.base import ComponentBase, ComponentParamBase
from common.connection_utils import timeout
from rag.prompts.generator import tool_call_summary, message_fit_in, citation_prompt, structured_output_prompt


class LLMParam(ComponentParamBase):
    """
    Define the LLM component parameters.
    """

    def __init__(self):
        super().__init__()
        self.llm_id = ""
        self.sys_prompt = ""
        self.prompts = [{"role": "user", "content": "{sys.query}"}]
        self.max_tokens = 0
        self.temperature = 0
        self.top_p = 0
        self.presence_penalty = 0
        self.frequency_penalty = 0
        self.output_structure = None
        self.cite = True
        self.visual_files_var = None

    def check(self):
        self.check_decimal_float(float(self.temperature), "[Agent] Temperature")
        self.check_decimal_float(float(self.presence_penalty), "[Agent] Presence penalty")
        self.check_decimal_float(float(self.frequency_penalty), "[Agent] Frequency penalty")
        self.check_nonnegative_number(int(self.max_tokens), "[Agent] Max tokens")
        self.check_decimal_float(float(self.top_p), "[Agent] Top P")
        self.check_empty(self.llm_id, "[Agent] LLM")
        self.check_empty(self.prompts, "[Agent] User prompt")

    def gen_conf(self):
        conf = {}

        def get_attr(nm):
            try:
                return getattr(self, nm)
            except Exception:
                pass

        if int(self.max_tokens) > 0 and get_attr("maxTokensEnabled"):
            conf["max_tokens"] = int(self.max_tokens)
        if float(self.temperature) > 0 and get_attr("temperatureEnabled"):
            conf["temperature"] = float(self.temperature)
        if float(self.top_p) > 0 and get_attr("topPEnabled"):
            conf["top_p"] = float(self.top_p)
        if float(self.presence_penalty) > 0 and get_attr("presencePenaltyEnabled"):
            conf["presence_penalty"] = float(self.presence_penalty)
        if float(self.frequency_penalty) > 0 and get_attr("frequencyPenaltyEnabled"):
            conf["frequency_penalty"] = float(self.frequency_penalty)
        if hasattr(self, "thinking") and self.thinking and self.thinking != "default":
            conf["thinking"] = self.thinking
        return conf


class LLM(ComponentBase):
    component_name = "LLM"

    def __init__(self, canvas, component_id, param: ComponentParamBase):
        super().__init__(canvas, component_id, param)
        model_types = get_model_type_by_name(self._canvas.get_tenant_id(), self._param.llm_id)
        model_type = "chat" if "chat" in model_types else model_types[0]
        chat_model_config = get_model_config_from_provider_instance(self._canvas.get_tenant_id(), model_type, self._param.llm_id)
        self.chat_mdl = LLMBundle(self._canvas.get_tenant_id(), chat_model_config, max_retries=self._param.max_retries, retry_interval=self._param.delay_after_error)
        self.imgs = []

    def get_input_form(self) -> dict[str, dict]:
        res = {}
        for k, v in self.get_input_elements().items():
            res[k] = {"type": "line", "name": v["name"]}
        return res

    def get_input_elements(self) -> dict[str, Any]:
        res = self.get_input_elements_from_text(self._param.sys_prompt)
        if isinstance(self._param.prompts, str):
            self._param.prompts = [{"role": "user", "content": self._param.prompts}]
        for prompt in self._param.prompts:
            d = self.get_input_elements_from_text(prompt["content"])
            res.update(d)
        return res

    def set_debug_inputs(self, inputs: dict[str, dict]):
        self._param.debug_inputs = inputs

    def add2system_prompt(self, txt):
        self._param.sys_prompt += txt

    def _sys_prompt_and_msg(self, msg, args):
        if isinstance(self._param.prompts, str):
            self._param.prompts = [{"role": "user", "content": self._param.prompts}]
        history_size = len(msg)
        for p in self._param.prompts:
            formatted = deepcopy(p)
            formatted["content"] = self.string_format(formatted["content"], args)
            if len(msg) == history_size and msg and msg[-1]["role"] == formatted["role"]:
                msg[-1] = formatted
            else:
                msg.append(formatted)
        return msg, self.string_format(self._param.sys_prompt, args)

    @staticmethod
    def effective_context_length(max_length) -> int:
        return max_length or 8192

    @classmethod
    def context_fit_budget(cls, max_length) -> int:
        return int(cls.effective_context_length(max_length) * 0.97)

    @staticmethod
    def validate_fitted_messages(msg_fit: list[dict]) -> str | None:
        if len(msg_fit) < 2:
            return "**ERROR**: message_fit_in produced insufficient messages for LLM"
        last = msg_fit[-1]
        if last.get("role") != "user" or not str(last.get("content") or "").strip():
            return "**ERROR**: LLM user message is empty after prompt fitting; check model max_tokens context setting"
        return None

    @classmethod
    def fit_messages(cls, system_prompt: str, msg: list[dict], max_length) -> tuple[list[dict], str | None]:
        _, msg_fit = message_fit_in(
            [{"role": "system", "content": system_prompt}, *deepcopy(msg)],
            cls.context_fit_budget(max_length),
        )
        return msg_fit, cls.validate_fitted_messages(msg_fit)

    @staticmethod
    def _extract_data_images(value) -> list[str]:
        imgs = []

        def walk(v):
            if v is None:
                return
            if isinstance(v, str):
                v = v.strip()
                if v.startswith("data:image/"):
                    imgs.append(v)
                return
            if isinstance(v, (list, tuple, set)):
                for item in v:
                    walk(item)
                return
            if isinstance(v, dict):
                if "content" in v:
                    walk(v.get("content"))
                else:
                    for item in v.values():
                        walk(item)

        walk(value)
        return imgs

    @staticmethod
    def _uniq_images(images: list[str]) -> list[str]:
        seen = set()
        uniq = []
        for img in images:
            if not isinstance(img, str):
                continue
            if not img.startswith("data:image/"):
                continue
            if img in seen:
                continue
            seen.add(img)
            uniq.append(img)
        return uniq

    @classmethod
    def _remove_data_images(cls, value):
        if value is None:
            return None

        if isinstance(value, str):
            return None if value.strip().startswith("data:image/") else value

        if isinstance(value, list):
            cleaned = []
            for item in value:
                v = cls._remove_data_images(item)
                if v is None:
                    continue
                if isinstance(v, (list, tuple, set, dict)) and not v:
                    continue
                cleaned.append(v)
            return cleaned

        if isinstance(value, tuple):
            cleaned = []
            for item in value:
                v = cls._remove_data_images(item)
                if v is None:
                    continue
                if isinstance(v, (list, tuple, set, dict)) and not v:
                    continue
                cleaned.append(v)
            return tuple(cleaned)

        if isinstance(value, set):
            cleaned = []
            for item in value:
                v = cls._remove_data_images(item)
                if v is None:
                    continue
                if isinstance(v, (list, tuple, set, dict)) and not v:
                    continue
                cleaned.append(v)
            return cleaned

        if isinstance(value, dict):
            if value.get("type") in {"image_url", "input_image", "image"} and cls._extract_data_images(value):
                return None

            cleaned = {}
            for k, item in value.items():
                v = cls._remove_data_images(item)
                if v is None:
                    continue
                if isinstance(v, (list, tuple, set, dict)) and not v:
                    continue
                cleaned[k] = v
            return cleaned

        return value

    def _collect_sys_files(self) -> tuple[list[str], list[str]]:
        files = self._canvas.globals.get("sys.files") or []
        if not files:
            logging.debug("[LLM] sys.files empty; skipping attachment injection")
            return [], []

        logging.info("[LLM] sys.files present: count=%d", len(files))

        explicit = "{sys.files}" in (self._param.sys_prompt or "")
        if not explicit and isinstance(self._param.prompts, list):
            for p in self._param.prompts:
                if isinstance(p, dict) and "{sys.files}" in (p.get("content") or ""):
                    explicit = True
                    break
        if explicit:
            logging.info("[LLM] prompt template references {sys.files}; skipping auto-injection (explicit=%s)", explicit)
            return [], []

        text_parts: list[str] = []
        image_data_uris: list[str] = []
        for f in files:
            if not isinstance(f, str):
                logging.debug("[LLM] skipping non-str sys.files entry: type=%s", type(f).__name__)
                continue
            if f.startswith("data:image/"):
                image_data_uris.append(f)
            else:
                text_parts.append(f)
        logging.info(
            "[LLM] sys.files split: text_parts=%d image_data_uris=%d (explicit=%s)",
            len(text_parts),
            len(image_data_uris),
            explicit,
        )
        return text_parts, image_data_uris

    def _prepare_prompt_variables(self):
        self.imgs = []
        if self._param.visual_files_var:
            visual_val = self._canvas.get_variable_value(self._param.visual_files_var)
            self.imgs.extend(self._extract_data_images(visual_val))

        args = {}
        vars = self.get_input_elements() if not self._param.debug_inputs else self._param.debug_inputs
        extracted_imgs = []
        for k, o in vars.items():
            raw_value = o["value"]
            extracted_imgs.extend(self._extract_data_images(raw_value))
            args[k] = self._remove_data_images(raw_value)
            if args[k] is None:
                args[k] = ""
            if not isinstance(args[k], str):
                try:
                    args[k] = json.dumps(args[k], ensure_ascii=False)
                except Exception:
                    args[k] = str(args[k])
            self.set_input_value(k, args[k])

        sys_file_texts, sys_file_imgs = self._collect_sys_files()
        prev_img_count = len(self.imgs) + len(extracted_imgs)
        self.imgs = self._uniq_images(self.imgs + extracted_imgs + sys_file_imgs)
        logging.debug(
            "[LLM] imgs rebuilt: total=%d sys_files_added=%d unique_dropped=%d",
            len(self.imgs),
            len(sys_file_imgs),
            max(0, prev_img_count + len(sys_file_imgs) - len(self.imgs)),
        )
        model_types = get_model_type_by_name(self._canvas.get_tenant_id(), self._param.llm_id)
        if self.imgs and LLMType.IMAGE2TEXT.value in model_types:
            model_type = LLMType.IMAGE2TEXT.value
        elif LLMType.CHAT.value in model_types:
            model_type = LLMType.CHAT.value
        else:
            model_type = model_types[0]
        model_config = get_model_config_from_provider_instance(self._canvas.get_tenant_id(), model_type, self._param.llm_id)
        if self.imgs:
            self.chat_mdl = LLMBundle(self._canvas.get_tenant_id(), model_config, max_retries=self._param.max_retries, retry_interval=self._param.delay_after_error)

        msg, sys_prompt = self._sys_prompt_and_msg(self._canvas.get_history(self._param.message_history_window_size)[:-1], args)

        if sys_file_texts:
            joined = "\n\n".join(sys_file_texts)
            merged_idx = -1
            for i in range(len(msg) - 1, -1, -1):
                if msg[i].get("role") == "user":
                    msg[i]["content"] = (msg[i].get("content") or "") + "\n\n" + joined
                    merged_idx = i
                    break
            else:
                msg.append({"role": "user", "content": joined})
                merged_idx = len(msg) - 1
            logging.info(
                "[LLM] sys.files text merged into msg: parts=%d total_chars=%d msg_index=%d action=%s",
                len(sys_file_texts),
                len(joined),
                merged_idx,
                "merged_into_existing_user" if merged_idx < len(msg) - 1 or msg[merged_idx].get("content", "") != joined else "appended_new_user",
            )

        user_defined_prompt, sys_prompt = self._extract_prompts(sys_prompt)
        if self._param.cite and self._canvas.get_reference()["chunks"]:
            sys_prompt += citation_prompt(user_defined_prompt)

        return sys_prompt, msg, user_defined_prompt

    def _extract_prompts(self, sys_prompt):
        pts = {}
        for tag in ["TASK_ANALYSIS", "PLAN_GENERATION", "REFLECTION", "CONTEXT_SUMMARY", "CONTEXT_RANKING", "CITATION_GUIDELINES"]:
            r = re.search(rf"<{tag}>(.*?)</{tag}>", sys_prompt, flags=re.DOTALL | re.IGNORECASE)
            if not r:
                continue
            pts[tag.lower()] = r.group(1)
            sys_prompt = re.sub(rf"<{tag}>(.*?)</{tag}>", "", sys_prompt, flags=re.DOTALL | re.IGNORECASE)
        return pts, sys_prompt

    async def _generate_async(self, msg: list[dict], **kwargs) -> str:
        if not self.imgs:
            return await self.chat_mdl.async_chat(msg[0]["content"], msg[1:], self._param.gen_conf(), **kwargs)
        return await self.chat_mdl.async_chat(msg[0]["content"], msg[1:], self._param.gen_conf(), images=self.imgs, **kwargs)

    async def _generate_streamly(self, msg: list[dict], **kwargs) -> AsyncGenerator[str, None]:
        stream_kwargs = {"images": self.imgs} if self.imgs else {}
        stream_kwargs.update(kwargs)
        stream = self.chat_mdl.async_chat_streamly_delta(msg[0]["content"], msg[1:], self._param.gen_conf(), **stream_kwargs)
        async for _, value, _ in _stream_with_think_delta(stream, min_tokens=0):
            yield value

    async def _stream_output_async(self, prompt, msg):
        msg_fit, fit_error = self.fit_messages(prompt, msg, self.chat_mdl.max_length)
        if fit_error:
            logging.error("LLM streaming prompt fit error: %s", fit_error)
            if self.get_exception_default_value():
                fallback = self.get_exception_default_value()
                self.set_output("content", fallback)
                yield fallback
            else:
                self.set_output("_ERROR", fit_error)
            return

        answer = ""
        stream_kwargs = {"images": self.imgs} if self.imgs else {}
        extra_chat_kwargs = self._get_chat_template_kwargs()
        stream_kwargs.update(extra_chat_kwargs)
        stream = self.chat_mdl.async_chat_streamly_delta(msg_fit[0]["content"], msg_fit[1:], self._param.gen_conf(), **stream_kwargs)
        async for _, ans, _ in _stream_with_think_delta(stream, min_tokens=0):
            if self.check_if_canceled("LLM streaming"):
                return

            if ans.find("**ERROR**") >= 0:
                if self.get_exception_default_value():
                    self.set_output("content", self.get_exception_default_value())
                    yield self.get_exception_default_value()
                else:
                    self.set_output("_ERROR", ans)
                return

            answer += ans
            yield ans

        self.set_output("content", answer)

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10 * 60)))
    async def _invoke_async(self, **kwargs):
        if self.check_if_canceled("LLM processing"):
            return

        def clean_formated_answer(ans: str) -> str:
            ans = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
            ans = re.sub(r"^.*```json", "", ans, flags=re.DOTALL)
            return re.sub(r"```\n*$", "", ans, flags=re.DOTALL)

        prompt, msg, _ = self._prepare_prompt_variables()
        extra_chat_kwargs = self._get_chat_template_kwargs()
        error: str = ""
        output_structure = None
        try:
            output_structure = self._param.outputs["structured"]
        except Exception:
            pass
        if output_structure and isinstance(output_structure, dict) and output_structure.get("properties") and len(output_structure["properties"]) > 0:
            schema = json.dumps(output_structure, ensure_ascii=False, indent=2)
            prompt_with_schema = prompt + structured_output_prompt(schema)
            for _ in range(self._param.max_retries + 1):
                if self.check_if_canceled("LLM processing"):
                    return

                msg_fit, fit_error = self.fit_messages(prompt_with_schema, msg, self.chat_mdl.max_length)
                if fit_error:
                    logging.error("LLM structured prompt fit error: %s", fit_error)
                    self.set_output("_ERROR", fit_error)
                    return
                error = ""
                ans = await self._generate_async(msg_fit, **extra_chat_kwargs)
                msg_fit.pop(0)
                if ans.find("**ERROR**") >= 0:
                    logging.error(f"LLM response error: {ans}")
                    error = ans
                    continue
                try:
                    self.set_output("structured", json_repair.loads(clean_formated_answer(ans)))
                    return
                except Exception:
                    msg_fit.append({"role": "user", "content": "The answer can't not be parsed as JSON"})
                    error = "The answer can't not be parsed as JSON"
            if error:
                self.set_output("_ERROR", error)
            return

        downstreams = self._canvas.get_component(self._id)["downstream"] if self._canvas.get_component(self._id) else []
        ex = self.exception_handler()
        if any([self._canvas.get_component_obj(cid).component_name.lower() == "message" for cid in downstreams]) and not (ex and ex["goto"]):
            self.set_output("content", partial(self._stream_output_async, prompt, deepcopy(msg)))
            return

        error = ""
        for _ in range(self._param.max_retries + 1):
            if self.check_if_canceled("LLM processing"):
                return

            msg_fit, fit_error = self.fit_messages(prompt, msg, self.chat_mdl.max_length)
            if fit_error:
                logging.error("LLM prompt fit error: %s", fit_error)
                error = fit_error
                break
            error = ""
            ans = await self._generate_async(msg_fit, **extra_chat_kwargs)
            msg_fit.pop(0)
            if ans.find("**ERROR**") >= 0:
                logging.error(f"LLM response error: {ans}")
                error = ans
                continue
            self.set_output("content", ans)
            break

        if error:
            if self.get_exception_default_value():
                self.set_output("content", self.get_exception_default_value())
            else:
                self.set_output("_ERROR", error)

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10 * 60)))
    def _invoke(self, **kwargs):
        return asyncio.run(self._invoke_async(**kwargs))

    def _get_chat_template_kwargs(self) -> dict[str, Any]:
        chat_template_kwargs = self._canvas.globals.get("sys.chat_template_kwargs")
        if chat_template_kwargs is None:
            return {}

        # The API should pass this as a JSON object, but accept a JSON string for compatibility.
        if isinstance(chat_template_kwargs, str):
            try:
                chat_template_kwargs = json_repair.loads(chat_template_kwargs)
            except Exception:
                logging.warning("Ignore invalid sys.chat_template_kwargs: expected JSON object or JSON string object.")
                return {}

        if not isinstance(chat_template_kwargs, dict):
            logging.warning("Ignore invalid sys.chat_template_kwargs type: %s", type(chat_template_kwargs).__name__)
            return {}
        return {"chat_template_kwargs": chat_template_kwargs}

    async def add_memory(self, user: str, assist: str, func_name: str, params: dict, results: str, user_defined_prompt: dict = {}):
        summ = await tool_call_summary(self.chat_mdl, func_name, params, results, user_defined_prompt)
        logging.info(f"[MEMORY]: {summ}")
        self._canvas.add_memory(user, assist, summ)

    def thoughts(self) -> str:
        _, msg, _ = self._prepare_prompt_variables()
        return "⌛Give me a moment—starting from: \n\n" + re.sub(r"(User's query:|[\\]+)", "", msg[-1]["content"], flags=re.DOTALL) + "\n\nI’ll figure out our best next move."
