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
from api.db.services.llm_service import LLMBundle
from api.db.services.tenant_llm_service import TenantLLMService
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
        return conf


class LLM(ComponentBase):
    component_name = "LLM"

    def __init__(self, canvas, component_id, param: ComponentParamBase):
        super().__init__(canvas, component_id, param)
        self.chat_mdl = LLMBundle(self._canvas.get_tenant_id(), TenantLLMService.llm_id2llm_type(self._param.llm_id),
                                  self._param.llm_id, max_retries=self._param.max_retries,
                                  retry_interval=self._param.delay_after_error
                                  )
        self.imgs = []

    def get_input_form(self) -> dict[str, dict]:
        res = {}
        for k, v in self.get_input_elements().items():
            res[k] = {
                "type": "line",
                "name": v["name"]
            }
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
        for p in self._param.prompts:
            if msg and msg[-1]["role"] == p["role"]:
                continue
            p = deepcopy(p)
            p["content"] = self.string_format(p["content"], args)
            msg.append(p)
        return msg, self.string_format(self._param.sys_prompt, args)

    def _prepare_prompt_variables(self):
        if self._param.visual_files_var:
            self.imgs = self._canvas.get_variable_value(self._param.visual_files_var)
            if not self.imgs:
                self.imgs = []
            self.imgs = [img for img in self.imgs if img[:len("data:image/")] == "data:image/"]
            if self.imgs and TenantLLMService.llm_id2llm_type(self._param.llm_id) == LLMType.CHAT.value:
                self.chat_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.IMAGE2TEXT.value,
                                          self._param.llm_id, max_retries=self._param.max_retries,
                                          retry_interval=self._param.delay_after_error
                                          )


        args = {}
        vars = self.get_input_elements() if not self._param.debug_inputs else self._param.debug_inputs
        for k, o in vars.items():
            args[k] = o["value"]
            if not isinstance(args[k], str):
                try:
                    args[k] = json.dumps(args[k], ensure_ascii=False)
                except Exception:
                    args[k] = str(args[k])
            self.set_input_value(k, args[k])

        msg, sys_prompt = self._sys_prompt_and_msg(self._canvas.get_history(self._param.message_history_window_size)[:-1], args)
        user_defined_prompt, sys_prompt = self._extract_prompts(sys_prompt)
        if self._param.cite and self._canvas.get_reference()["chunks"]:
            sys_prompt += citation_prompt(user_defined_prompt)

        return sys_prompt, msg, user_defined_prompt

    def _extract_prompts(self, sys_prompt):
        pts = {}
        for tag in ["TASK_ANALYSIS", "PLAN_GENERATION", "REFLECTION", "CONTEXT_SUMMARY", "CONTEXT_RANKING", "CITATION_GUIDELINES"]:
            r = re.search(rf"<{tag}>(.*?)</{tag}>", sys_prompt, flags=re.DOTALL|re.IGNORECASE)
            if not r:
                continue
            pts[tag.lower()] = r.group(1)
            sys_prompt = re.sub(rf"<{tag}>(.*?)</{tag}>", "", sys_prompt, flags=re.DOTALL|re.IGNORECASE)
        return pts, sys_prompt

    async def _generate_async(self, msg: list[dict], **kwargs) -> str:
        if not self.imgs:
            return await self.chat_mdl.async_chat(msg[0]["content"], msg[1:], self._param.gen_conf(), **kwargs)
        return await self.chat_mdl.async_chat(msg[0]["content"], msg[1:], self._param.gen_conf(), images=self.imgs, **kwargs)

    async def _generate_streamly(self, msg: list[dict], **kwargs) -> AsyncGenerator[str, None]:
        async def delta_wrapper(txt_iter):
            ans = ""
            last_idx = 0
            endswith_think = False

            def delta(txt):
                nonlocal ans, last_idx, endswith_think
                delta_ans = txt[last_idx:]
                ans = txt

                if delta_ans.find("<think>") == 0:
                    last_idx += len("<think>")
                    return "<think>"
                elif delta_ans.find("<think>") > 0:
                    delta_ans = txt[last_idx:last_idx + delta_ans.find("<think>")]
                    last_idx += delta_ans.find("<think>")
                    return delta_ans
                elif delta_ans.endswith("</think>"):
                    endswith_think = True
                elif endswith_think:
                    endswith_think = False
                    return "</think>"

                last_idx = len(ans)
                if ans.endswith("</think>"):
                    last_idx -= len("</think>")
                return re.sub(r"(<think>|</think>)", "", delta_ans)

            async for t in txt_iter:
                yield delta(t)

        if not self.imgs:
            async for t in delta_wrapper(self.chat_mdl.async_chat_streamly(msg[0]["content"], msg[1:], self._param.gen_conf(), **kwargs)):
                yield t
            return

        async for t in delta_wrapper(self.chat_mdl.async_chat_streamly(msg[0]["content"], msg[1:], self._param.gen_conf(), images=self.imgs, **kwargs)):
            yield t

    async def _stream_output_async(self, prompt, msg):
        _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(self.chat_mdl.max_length * 0.97))
        answer = ""
        last_idx = 0
        endswith_think = False

        def delta(txt):
            nonlocal answer, last_idx, endswith_think
            delta_ans = txt[last_idx:]
            answer = txt

            if delta_ans.find("<think>") == 0:
                last_idx += len("<think>")
                return "<think>"
            elif delta_ans.find("<think>") > 0:
                delta_ans = txt[last_idx:last_idx + delta_ans.find("<think>")]
                last_idx += delta_ans.find("<think>")
                return delta_ans
            elif delta_ans.endswith("</think>"):
                endswith_think = True
            elif endswith_think:
                endswith_think = False
                return "</think>"

            last_idx = len(answer)
            if answer.endswith("</think>"):
                last_idx -= len("</think>")
            return re.sub(r"(<think>|</think>)", "", delta_ans)

        stream_kwargs = {"images": self.imgs} if self.imgs else {}
        async for ans in self.chat_mdl.async_chat_streamly(msg[0]["content"], msg[1:], self._param.gen_conf(), **stream_kwargs):
            if self.check_if_canceled("LLM streaming"):
                return

            if isinstance(ans, int):
                continue

            if ans.find("**ERROR**") >= 0:
                if self.get_exception_default_value():
                    self.set_output("content", self.get_exception_default_value())
                    yield self.get_exception_default_value()
                else:
                    self.set_output("_ERROR", ans)
                return

            yield delta(ans)

        self.set_output("content", answer)

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60)))
    async def _invoke_async(self, **kwargs):
        if self.check_if_canceled("LLM processing"):
            return

        def clean_formated_answer(ans: str) -> str:
            ans = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
            ans = re.sub(r"^.*```json", "", ans, flags=re.DOTALL)
            return re.sub(r"```\n*$", "", ans, flags=re.DOTALL)

        prompt, msg, _ = self._prepare_prompt_variables()
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

                _, msg_fit = message_fit_in(
                    [{"role": "system", "content": prompt_with_schema}, *deepcopy(msg)],
                    int(self.chat_mdl.max_length * 0.97),
                )
                error = ""
                ans = await self._generate_async(msg_fit)
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
        if any([self._canvas.get_component_obj(cid).component_name.lower() == "message" for cid in downstreams]) and not (
            ex and ex["goto"]
        ):
            self.set_output("content", partial(self._stream_output_async, prompt, deepcopy(msg)))
            return

        error = ""
        for _ in range(self._param.max_retries + 1):
            if self.check_if_canceled("LLM processing"):
                return

            _, msg_fit = message_fit_in(
                [{"role": "system", "content": prompt}, *deepcopy(msg)], int(self.chat_mdl.max_length * 0.97)
            )
            error = ""
            ans = await self._generate_async(msg_fit)
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

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60)))
    def _invoke(self, **kwargs):
        return asyncio.run(self._invoke_async(**kwargs))

    async def add_memory(self, user:str, assist:str, func_name: str, params: dict, results: str, user_defined_prompt:dict={}):
        summ = await tool_call_summary(self.chat_mdl, func_name, params, results, user_defined_prompt)
        logging.info(f"[MEMORY]: {summ}")
        self._canvas.add_memory(user, assist, summ)

    def thoughts(self) -> str:
        _, msg,_ = self._prepare_prompt_variables()
        return "⌛Give me a moment—starting from: \n\n" + re.sub(r"(User's query:|[\\]+)", '', msg[-1]['content'], flags=re.DOTALL) + "\n\nI’ll figure out our best next move."
