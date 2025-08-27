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
import inspect
import logging
import re
from functools import partial
from typing import Generator
from api.db.db_models import LLM
from api.db.services.common_service import CommonService
from api.db.services.tenant_llm_service import LLM4Tenant, TenantLLMService


class LLMService(CommonService):
    model = LLM


def get_init_tenant_llm(user_id):
    from api import settings
    tenant_llm = []

    seen = set()
    factory_configs = []
    for factory_config in [
        settings.CHAT_CFG,
        settings.EMBEDDING_CFG,
        settings.ASR_CFG,
        settings.IMAGE2TEXT_CFG,
        settings.RERANK_CFG,
    ]:
        factory_name = factory_config["factory"]
        if factory_name not in seen:
            seen.add(factory_name)
            factory_configs.append(factory_config)

    for factory_config in factory_configs:
        for llm in LLMService.query(fid=factory_config["factory"]):
            tenant_llm.append(
                {
                    "tenant_id": user_id,
                    "llm_factory": factory_config["factory"],
                    "llm_name": llm.llm_name,
                    "model_type": llm.model_type,
                    "api_key": factory_config["api_key"],
                    "api_base": factory_config["base_url"],
                    "max_tokens": llm.max_tokens if llm.max_tokens else 8192,
                }
            )

    if settings.LIGHTEN != 1:
        for buildin_embedding_model in settings.BUILTIN_EMBEDDING_MODELS:
            mdlnm, fid = TenantLLMService.split_model_name_and_factory(buildin_embedding_model)
            tenant_llm.append(
                {
                    "tenant_id": user_id,
                    "llm_factory": fid,
                    "llm_name": mdlnm,
                    "model_type": "embedding",
                    "api_key": "",
                    "api_base": "",
                    "max_tokens": 1024 if buildin_embedding_model == "BAAI/bge-large-zh-v1.5@BAAI" else 512,
                }
            )

    unique = {}
    for item in tenant_llm:
        key = (item["tenant_id"], item["llm_factory"], item["llm_name"])
        if key not in unique:
            unique[key] = item
    return list(unique.values())


class LLMBundle(LLM4Tenant):
    def __init__(self, tenant_id, llm_type, llm_name=None, lang="Chinese", **kwargs):
        super().__init__(tenant_id, llm_type, llm_name, lang, **kwargs)

    def bind_tools(self, toolcall_session, tools):
        if not self.is_tools:
            logging.warning(f"Model {self.llm_name} does not support tool call, but you have assigned one or more tools to it!")
            return
        self.mdl.bind_tools(toolcall_session, tools)

    def encode(self, texts: list):
        if self.langfuse:
            generation = self.langfuse.start_generation(trace_context=self.trace_context, name="encode", model=self.llm_name, input={"texts": texts})

        embeddings, used_tokens = self.mdl.encode(texts)
        llm_name = getattr(self, "llm_name", None)
        if not TenantLLMService.increase_usage(self.tenant_id, self.llm_type, used_tokens, llm_name):
            logging.error("LLMBundle.encode can't update token usage for {}/EMBEDDING used_tokens: {}".format(self.tenant_id, used_tokens))

        if self.langfuse:
            generation.update(usage_details={"total_tokens": used_tokens})
            generation.end()

        return embeddings, used_tokens

    def encode_queries(self, query: str):
        if self.langfuse:
            generation = self.langfuse.start_generation(trace_context=self.trace_context, name="encode_queries", model=self.llm_name, input={"query": query})

        emd, used_tokens = self.mdl.encode_queries(query)
        llm_name = getattr(self, "llm_name", None)
        if not TenantLLMService.increase_usage(self.tenant_id, self.llm_type, used_tokens, llm_name):
            logging.error("LLMBundle.encode_queries can't update token usage for {}/EMBEDDING used_tokens: {}".format(self.tenant_id, used_tokens))

        if self.langfuse:
            generation.update(usage_details={"total_tokens": used_tokens})
            generation.end()

        return emd, used_tokens

    def similarity(self, query: str, texts: list):
        if self.langfuse:
            generation = self.langfuse.start_generation(trace_context=self.trace_context, name="similarity", model=self.llm_name, input={"query": query, "texts": texts})

        sim, used_tokens = self.mdl.similarity(query, texts)
        if not TenantLLMService.increase_usage(self.tenant_id, self.llm_type, used_tokens):
            logging.error("LLMBundle.similarity can't update token usage for {}/RERANK used_tokens: {}".format(self.tenant_id, used_tokens))

        if self.langfuse:
            generation.update(usage_details={"total_tokens": used_tokens})
            generation.end()

        return sim, used_tokens

    def describe(self, image, max_tokens=300):
        if self.langfuse:
            generation = self.langfuse.start_generation(trace_context=self.trace_context, name="describe", metadata={"model": self.llm_name})

        txt, used_tokens = self.mdl.describe(image)
        if not TenantLLMService.increase_usage(self.tenant_id, self.llm_type, used_tokens):
            logging.error("LLMBundle.describe can't update token usage for {}/IMAGE2TEXT used_tokens: {}".format(self.tenant_id, used_tokens))

        if self.langfuse:
            generation.update(output={"output": txt}, usage_details={"total_tokens": used_tokens})
            generation.end()

        return txt

    def describe_with_prompt(self, image, prompt):
        if self.langfuse:
            generation = self.langfuse.start_generation(trace_context=self.trace_context, name="describe_with_prompt", metadata={"model": self.llm_name, "prompt": prompt})

        txt, used_tokens = self.mdl.describe_with_prompt(image, prompt)
        if not TenantLLMService.increase_usage(self.tenant_id, self.llm_type, used_tokens):
            logging.error("LLMBundle.describe can't update token usage for {}/IMAGE2TEXT used_tokens: {}".format(self.tenant_id, used_tokens))

        if self.langfuse:
            generation.update(output={"output": txt}, usage_details={"total_tokens": used_tokens})
            generation.end()

        return txt

    def transcription(self, audio):
        if self.langfuse:
            generation = self.langfuse.start_generation(trace_context=self.trace_context, name="transcription", metadata={"model": self.llm_name})

        txt, used_tokens = self.mdl.transcription(audio)
        if not TenantLLMService.increase_usage(self.tenant_id, self.llm_type, used_tokens):
            logging.error("LLMBundle.transcription can't update token usage for {}/SEQUENCE2TXT used_tokens: {}".format(self.tenant_id, used_tokens))

        if self.langfuse:
            generation.update(output={"output": txt}, usage_details={"total_tokens": used_tokens})
            generation.end()

        return txt

    def tts(self, text: str) -> Generator[bytes, None, None]:
        if self.langfuse:
            generation = self.langfuse.start_generation(trace_context=self.trace_context, name="tts", input={"text": text})

        for chunk in self.mdl.tts(text):
            if isinstance(chunk, int):
                if not TenantLLMService.increase_usage(self.tenant_id, self.llm_type, chunk, self.llm_name):
                    logging.error("LLMBundle.tts can't update token usage for {}/TTS".format(self.tenant_id))
                return
            yield chunk

        if self.langfuse:
            generation.end()

    def _remove_reasoning_content(self, txt: str) -> str:
        first_think_start = txt.find("<think>")
        if first_think_start == -1:
            return txt

        last_think_end = txt.rfind("</think>")
        if last_think_end == -1:
            return txt

        if last_think_end < first_think_start:
            return txt

        return txt[last_think_end + len("</think>") :]
    
    @staticmethod
    def _clean_param(chat_partial, **kwargs):
        func = chat_partial.func
        sig = inspect.signature(func)
        keyword_args = []
        support_var_args = False
        for param in sig.parameters.values():
            if param.kind == inspect.Parameter.VAR_KEYWORD or param.kind == inspect.Parameter.VAR_POSITIONAL:
                support_var_args = True
            elif param.kind == inspect.Parameter.KEYWORD_ONLY:
                keyword_args.append(param.name)

        use_kwargs = kwargs
        if not support_var_args:
            use_kwargs = {k: v for k, v in kwargs.items() if k in keyword_args}
        return use_kwargs
        
    def chat(self, system: str, history: list, gen_conf: dict = {}, **kwargs) -> str:
        if self.langfuse:
            generation = self.langfuse.start_generation(trace_context=self.trace_context, name="chat", model=self.llm_name, input={"system": system, "history": history})

        chat_partial = partial(self.mdl.chat, system, history, gen_conf)
        if self.is_tools and self.mdl.is_tools:
            chat_partial = partial(self.mdl.chat_with_tools, system, history, gen_conf)
            
        use_kwargs = self._clean_param(chat_partial, **kwargs)
        txt, used_tokens = chat_partial(**use_kwargs)
        txt = self._remove_reasoning_content(txt)

        if not self.verbose_tool_use:
            txt = re.sub(r"<tool_call>.*?</tool_call>", "", txt, flags=re.DOTALL)

        if isinstance(txt, int) and not TenantLLMService.increase_usage(self.tenant_id, self.llm_type, used_tokens, self.llm_name):
            logging.error("LLMBundle.chat can't update token usage for {}/CHAT llm_name: {}, used_tokens: {}".format(self.tenant_id, self.llm_name, used_tokens))

        if self.langfuse:
            generation.update(output={"output": txt}, usage_details={"total_tokens": used_tokens})
            generation.end()

        return txt

    def chat_streamly(self, system: str, history: list, gen_conf: dict = {}, **kwargs):
        if self.langfuse:
            generation = self.langfuse.start_generation(trace_context=self.trace_context, name="chat_streamly", model=self.llm_name, input={"system": system, "history": history})

        ans = ""
        chat_partial = partial(self.mdl.chat_streamly, system, history, gen_conf)
        total_tokens = 0
        if self.is_tools and self.mdl.is_tools:
            chat_partial = partial(self.mdl.chat_streamly_with_tools, system, history, gen_conf)
        use_kwargs = self._clean_param(chat_partial, **kwargs)
        for txt in chat_partial(**use_kwargs):
            if isinstance(txt, int):
                total_tokens = txt
                if self.langfuse:
                    generation.update(output={"output": ans})
                    generation.end()
                break

            if txt.endswith("</think>"):
                ans = ans.rstrip("</think>")

            if not self.verbose_tool_use:
                txt = re.sub(r"<tool_call>.*?</tool_call>", "", txt, flags=re.DOTALL)

            ans += txt
            yield ans

        if total_tokens > 0:
            if not TenantLLMService.increase_usage(self.tenant_id, self.llm_type, txt, self.llm_name):
                logging.error("LLMBundle.chat_streamly can't update token usage for {}/CHAT llm_name: {}, content: {}".format(self.tenant_id, self.llm_name, txt))
