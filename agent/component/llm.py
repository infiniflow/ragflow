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
import logging
import os
import re
from typing import Any, Generator
import json_repair
from functools import partial
from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from api.db.services.tenant_llm_service import TenantLLMService
from agent.component.base import ComponentBase, ComponentParamBase
from api.utils.api_utils import timeout
from rag.prompts import message_fit_in, citation_prompt
from rag.prompts.prompts import tool_call_summary


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
        self.check_empty(self.sys_prompt, "[Agent] System prompt")
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
    
    def __init__(self, canvas, id, param: ComponentParamBase):
        super().__init__(canvas, id, param)
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
        for prompt in self._param.prompts:
            d = self.get_input_elements_from_text(prompt["content"])
            res.update(d)
        return res

    def set_debug_inputs(self, inputs: dict[str, dict]):
        self._param.debug_inputs = inputs

    def add2system_prompt(self, txt):
        self._param.sys_prompt += txt

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
        sys_prompt = self._param.sys_prompt
        for k, o in vars.items():
            args[k] = o["value"]
            if not isinstance(args[k], str):
                try:
                    args[k] = json.dumps(args[k], ensure_ascii=False)
                except Exception:
                    args[k] = str(args[k])
            self.set_input_value(k, args[k])

        msg = self._canvas.get_history(self._param.message_history_window_size)[:-1]
        for p in self._param.prompts:
            if msg and msg[-1]["role"] == p["role"]:
                continue
            msg.append(p)

        sys_prompt = self.string_format(sys_prompt, args)
        for m in msg:
            m["content"] = self.string_format(m["content"], args)
        if self._param.cite and self._canvas.get_reference()["chunks"]:
            sys_prompt += citation_prompt()

        return sys_prompt, msg

    def _generate(self, msg:list[dict], **kwargs) -> str:
        if not self.imgs:
            return self.chat_mdl.chat(msg[0]["content"], msg[1:], self._param.gen_conf(), **kwargs)
        return self.chat_mdl.chat(msg[0]["content"], msg[1:], self._param.gen_conf(), images=self.imgs, **kwargs)

    def _generate_streamly(self, msg:list[dict], **kwargs) -> Generator[str, None, None]:
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
                delta_ans = txt[last_idx:last_idx+delta_ans.find("<think>")]
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

        if not self.imgs:
            for txt in self.chat_mdl.chat_streamly(msg[0]["content"], msg[1:], self._param.gen_conf(), **kwargs):
                yield delta(txt)
        else:
            for txt in self.chat_mdl.chat_streamly(msg[0]["content"], msg[1:], self._param.gen_conf(), images=self.imgs, **kwargs):
                yield delta(txt)

    @timeout(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60))
    def _invoke(self, **kwargs):
        def clean_formated_answer(ans: str) -> str:
            ans = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
            ans = re.sub(r"^.*```json", "", ans, flags=re.DOTALL)
            return re.sub(r"```\n*$", "", ans, flags=re.DOTALL)

        prompt, msg = self._prepare_prompt_variables()
        error = ""

        if self._param.output_structure:
            prompt += "\nThe output MUST follow this JSON format:\n"+json.dumps(self._param.output_structure, ensure_ascii=False, indent=2)
            prompt += "\nRedundant information is FORBIDDEN."
            for _ in range(self._param.max_retries+1):
                _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(self.chat_mdl.max_length * 0.97))
                error = ""
                ans = self._generate(msg)
                msg.pop(0)
                if ans.find("**ERROR**") >= 0:
                    logging.error(f"LLM response error: {ans}")
                    error = ans
                    continue
                try:
                    self.set_output("structured_content", json_repair.loads(clean_formated_answer(ans)))
                    return
                except Exception:
                    msg.append({"role": "user", "content": "The answer can't not be parsed as JSON"})
                    error = "The answer can't not be parsed as JSON"
            if error:
                self.set_output("_ERROR", error)
            return

        downstreams = self._canvas.get_component(self._id)["downstream"] if self._canvas.get_component(self._id) else []
        ex = self.exception_handler()
        if any([self._canvas.get_component_obj(cid).component_name.lower()=="message" for cid in downstreams]) and not self._param.output_structure and not (ex and ex["goto"]):
            self.set_output("content", partial(self._stream_output, prompt, msg))
            return

        for _ in range(self._param.max_retries+1):
            _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(self.chat_mdl.max_length * 0.97))
            error = ""
            ans = self._generate(msg)
            msg.pop(0)
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

    def _stream_output(self, prompt, msg):
        _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(self.chat_mdl.max_length * 0.97))
        answer = ""
        for ans in self._generate_streamly(msg):
            if ans.find("**ERROR**") >= 0:
                if self.get_exception_default_value():
                    self.set_output("content", self.get_exception_default_value())
                    yield self.get_exception_default_value()
                else:
                    self.set_output("_ERROR", ans)
                return
            yield ans
            answer += ans
        self.set_output("content", answer)

    def add_memory(self, user:str, assist:str, func_name: str, params: dict, results: str):
        summ = tool_call_summary(self.chat_mdl, func_name, params, results)
        logging.info(f"[MEMORY]: {summ}")
        self._canvas.add_memory(user, assist, summ)

    def thoughts(self) -> str:
        _, msg = self._prepare_prompt_variables()
        return "⌛Give me a moment—starting from: \n\n" + re.sub(r"(User's query:|[\\]+)", '', msg[-1]['content'], flags=re.DOTALL) + "\n\nI’ll figure out our best next move."