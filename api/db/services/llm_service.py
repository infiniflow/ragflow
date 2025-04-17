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
import logging

from langfuse import Langfuse

from api import settings
from api.db import LLMType
from api.db.db_models import DB, LLM, LLMFactories, TenantLLM
from api.db.services.common_service import CommonService
from api.db.services.langfuse_service import TenantLangfuseService
from api.db.services.user_service import TenantService
from rag.llm import ChatModel, CvModel, EmbeddingModel, RerankModel, Seq2txtModel, TTSModel


class LLMFactoriesService(CommonService):
    model = LLMFactories


class LLMService(CommonService):
    model = LLM


class TenantLLMService(CommonService):
    model = TenantLLM

    @classmethod
    @DB.connection_context()
    def get_api_key(cls, tenant_id, model_name):
        mdlnm, fid = TenantLLMService.split_model_name_and_factory(model_name)
        if not fid:
            objs = cls.query(tenant_id=tenant_id, llm_name=mdlnm)
        else:
            objs = cls.query(tenant_id=tenant_id, llm_name=mdlnm, llm_factory=fid)
        if not objs:
            return
        return objs[0]

    @classmethod
    @DB.connection_context()
    def get_my_llms(cls, tenant_id):
        fields = [cls.model.llm_factory, LLMFactories.logo, LLMFactories.tags, cls.model.model_type, cls.model.llm_name, cls.model.used_tokens]
        objs = cls.model.select(*fields).join(LLMFactories, on=(cls.model.llm_factory == LLMFactories.name)).where(cls.model.tenant_id == tenant_id, ~cls.model.api_key.is_null()).dicts()

        return list(objs)

    @staticmethod
    def split_model_name_and_factory(model_name):
        arr = model_name.split("@")
        if len(arr) < 2:
            return model_name, None
        if len(arr) > 2:
            return "@".join(arr[0:-1]), arr[-1]

        # model name must be xxx@yyy
        try:
            model_factories = settings.FACTORY_LLM_INFOS
            model_providers = set([f["name"] for f in model_factories])
            if arr[-1] not in model_providers:
                return model_name, None
            return arr[0], arr[-1]
        except Exception as e:
            logging.exception(f"TenantLLMService.split_model_name_and_factory got exception: {e}")
        return model_name, None

    @classmethod
    @DB.connection_context()
    def get_model_config(cls, tenant_id, llm_type, llm_name=None):
        e, tenant = TenantService.get_by_id(tenant_id)
        if not e:
            raise LookupError("Tenant not found")

        if llm_type == LLMType.EMBEDDING.value:
            mdlnm = tenant.embd_id if not llm_name else llm_name
        elif llm_type == LLMType.SPEECH2TEXT.value:
            mdlnm = tenant.asr_id
        elif llm_type == LLMType.IMAGE2TEXT.value:
            mdlnm = tenant.img2txt_id if not llm_name else llm_name
        elif llm_type == LLMType.CHAT.value:
            mdlnm = tenant.llm_id if not llm_name else llm_name
        elif llm_type == LLMType.RERANK:
            mdlnm = tenant.rerank_id if not llm_name else llm_name
        elif llm_type == LLMType.TTS:
            mdlnm = tenant.tts_id if not llm_name else llm_name
        else:
            assert False, "LLM type error"

        model_config = cls.get_api_key(tenant_id, mdlnm)
        mdlnm, fid = TenantLLMService.split_model_name_and_factory(mdlnm)
        if model_config:
            model_config = model_config.to_dict()
            llm = LLMService.query(llm_name=mdlnm) if not fid else LLMService.query(llm_name=mdlnm, fid=fid)
            if llm:
                model_config["is_tools"] = llm[0].is_tools
        if not model_config:
            if llm_type in [LLMType.EMBEDDING, LLMType.RERANK]:
                llm = LLMService.query(llm_name=mdlnm) if not fid else LLMService.query(llm_name=mdlnm, fid=fid)
                if llm and llm[0].fid in ["Youdao", "FastEmbed", "BAAI"]:
                    model_config = {"llm_factory": llm[0].fid, "api_key": "", "llm_name": mdlnm, "api_base": ""}
            if not model_config:
                if mdlnm == "flag-embedding":
                    model_config = {"llm_factory": "Tongyi-Qianwen", "api_key": "", "llm_name": llm_name, "api_base": ""}
                else:
                    if not mdlnm:
                        raise LookupError(f"Type of {llm_type} model is not set.")
                    raise LookupError("Model({}) not authorized".format(mdlnm))
        return model_config

    @classmethod
    @DB.connection_context()
    def model_instance(cls, tenant_id, llm_type, llm_name=None, lang="Chinese"):
        model_config = TenantLLMService.get_model_config(tenant_id, llm_type, llm_name)
        if llm_type == LLMType.EMBEDDING.value:
            if model_config["llm_factory"] not in EmbeddingModel:
                return
            return EmbeddingModel[model_config["llm_factory"]](model_config["api_key"], model_config["llm_name"], base_url=model_config["api_base"])

        if llm_type == LLMType.RERANK:
            if model_config["llm_factory"] not in RerankModel:
                return
            return RerankModel[model_config["llm_factory"]](model_config["api_key"], model_config["llm_name"], base_url=model_config["api_base"])

        if llm_type == LLMType.IMAGE2TEXT.value:
            if model_config["llm_factory"] not in CvModel:
                return
            return CvModel[model_config["llm_factory"]](model_config["api_key"], model_config["llm_name"], lang, base_url=model_config["api_base"])

        if llm_type == LLMType.CHAT.value:
            if model_config["llm_factory"] not in ChatModel:
                return
            return ChatModel[model_config["llm_factory"]](model_config["api_key"], model_config["llm_name"], base_url=model_config["api_base"])

        if llm_type == LLMType.SPEECH2TEXT:
            if model_config["llm_factory"] not in Seq2txtModel:
                return
            return Seq2txtModel[model_config["llm_factory"]](key=model_config["api_key"], model_name=model_config["llm_name"], lang=lang, base_url=model_config["api_base"])
        if llm_type == LLMType.TTS:
            if model_config["llm_factory"] not in TTSModel:
                return
            return TTSModel[model_config["llm_factory"]](
                model_config["api_key"],
                model_config["llm_name"],
                base_url=model_config["api_base"],
            )

    @classmethod
    @DB.connection_context()
    def increase_usage(cls, tenant_id, llm_type, used_tokens, llm_name=None):
        try:
            if not DB.is_connection_usable():
                DB.connect()
        except Exception:
            DB.close()
            DB.connect()
        e, tenant = TenantService.get_by_id(tenant_id)
        if not e:
            logging.error(f"Tenant not found: {tenant_id}")
            return 0

        llm_map = {
            LLMType.EMBEDDING.value: tenant.embd_id,
            LLMType.SPEECH2TEXT.value: tenant.asr_id,
            LLMType.IMAGE2TEXT.value: tenant.img2txt_id,
            LLMType.CHAT.value: tenant.llm_id if not llm_name else llm_name,
            LLMType.RERANK.value: tenant.rerank_id if not llm_name else llm_name,
            LLMType.TTS.value: tenant.tts_id if not llm_name else llm_name,
        }

        mdlnm = llm_map.get(llm_type)
        if mdlnm is None:
            logging.error(f"LLM type error: {llm_type}")
            return 0

        llm_name, llm_factory = TenantLLMService.split_model_name_and_factory(mdlnm)

        try:
            num = (
                cls.model.update(used_tokens=cls.model.used_tokens + used_tokens)
                .where(cls.model.tenant_id == tenant_id, cls.model.llm_name == llm_name, cls.model.llm_factory == llm_factory if llm_factory else True)
                .execute()
            )
        except Exception:
            logging.exception("TenantLLMService.increase_usage got exception,Failed to update used_tokens for tenant_id=%s, llm_name=%s", tenant_id, llm_name)
            return 0

        return num

    @classmethod
    @DB.connection_context()
    def get_openai_models(cls):
        objs = cls.model.select().where((cls.model.llm_factory == "OpenAI"), ~(cls.model.llm_name == "text-embedding-3-small"), ~(cls.model.llm_name == "text-embedding-3-large")).dicts()
        return list(objs)


class LLMBundle:
    def __init__(self, tenant_id, llm_type, llm_name=None, lang="Chinese"):
        self.tenant_id = tenant_id
        self.llm_type = llm_type
        self.llm_name = llm_name
        self.mdl = TenantLLMService.model_instance(tenant_id, llm_type, llm_name, lang=lang)
        assert self.mdl, "Can't find model for {}/{}/{}".format(tenant_id, llm_type, llm_name)
        model_config = TenantLLMService.get_model_config(tenant_id, llm_type, llm_name)
        self.max_length = model_config.get("max_tokens", 8192)

        self.is_tools = model_config.get("is_tools", False)

        langfuse_keys = TenantLangfuseService.filter_by_tenant(tenant_id=tenant_id)
        if langfuse_keys:
            langfuse = Langfuse(public_key=langfuse_keys.public_key, secret_key=langfuse_keys.secret_key, host=langfuse_keys.host)
            if langfuse.auth_check():
                self.langfuse = langfuse
                self.trace = self.langfuse.trace(name=f"{self.llm_type}-{self.llm_name}")
        else:
            self.langfuse = None

    def bind_tools(self, toolcall_session, tools):
        if not self.is_tools:
            return
        self.mdl.bind_tools(toolcall_session, tools)

    def encode(self, texts: list):
        if self.langfuse:
            generation = self.trace.generation(name="encode", model=self.llm_name, input={"texts": texts})

        embeddings, used_tokens = self.mdl.encode(texts)
        if not TenantLLMService.increase_usage(self.tenant_id, self.llm_type, used_tokens):
            logging.error("LLMBundle.encode can't update token usage for {}/EMBEDDING used_tokens: {}".format(self.tenant_id, used_tokens))

        if self.langfuse:
            generation.end(usage_details={"total_tokens": used_tokens})

        return embeddings, used_tokens

    def encode_queries(self, query: str):
        if self.langfuse:
            generation = self.trace.generation(name="encode_queries", model=self.llm_name, input={"query": query})

        emd, used_tokens = self.mdl.encode_queries(query)
        if not TenantLLMService.increase_usage(self.tenant_id, self.llm_type, used_tokens):
            logging.error("LLMBundle.encode_queries can't update token usage for {}/EMBEDDING used_tokens: {}".format(self.tenant_id, used_tokens))

        if self.langfuse:
            generation.end(usage_details={"total_tokens": used_tokens})

        return emd, used_tokens

    def similarity(self, query: str, texts: list):
        if self.langfuse:
            generation = self.trace.generation(name="similarity", model=self.llm_name, input={"query": query, "texts": texts})

        sim, used_tokens = self.mdl.similarity(query, texts)
        if not TenantLLMService.increase_usage(self.tenant_id, self.llm_type, used_tokens):
            logging.error("LLMBundle.similarity can't update token usage for {}/RERANK used_tokens: {}".format(self.tenant_id, used_tokens))

        if self.langfuse:
            generation.end(usage_details={"total_tokens": used_tokens})

        return sim, used_tokens

    def describe(self, image, max_tokens=300):
        if self.langfuse:
            generation = self.trace.generation(name="describe", metadata={"model": self.llm_name})

        txt, used_tokens = self.mdl.describe(image)
        if not TenantLLMService.increase_usage(self.tenant_id, self.llm_type, used_tokens):
            logging.error("LLMBundle.describe can't update token usage for {}/IMAGE2TEXT used_tokens: {}".format(self.tenant_id, used_tokens))

        if self.langfuse:
            generation.end(output={"output": txt}, usage_details={"total_tokens": used_tokens})

        return txt

    def describe_with_prompt(self, image, prompt):
        if self.langfuse:
            generation = self.trace.generation(name="describe_with_prompt", metadata={"model": self.llm_name, "prompt": prompt})

        txt, used_tokens = self.mdl.describe_with_prompt(image, prompt)
        if not TenantLLMService.increase_usage(self.tenant_id, self.llm_type, used_tokens):
            logging.error("LLMBundle.describe can't update token usage for {}/IMAGE2TEXT used_tokens: {}".format(self.tenant_id, used_tokens))

        if self.langfuse:
            generation.end(output={"output": txt}, usage_details={"total_tokens": used_tokens})

        return txt

    def transcription(self, audio):
        if self.langfuse:
            generation = self.trace.generation(name="transcription", metadata={"model": self.llm_name})

        txt, used_tokens = self.mdl.transcription(audio)
        if not TenantLLMService.increase_usage(self.tenant_id, self.llm_type, used_tokens):
            logging.error("LLMBundle.transcription can't update token usage for {}/SEQUENCE2TXT used_tokens: {}".format(self.tenant_id, used_tokens))

        if self.langfuse:
            generation.end(output={"output": txt}, usage_details={"total_tokens": used_tokens})

        return txt

    def tts(self, text):
        if self.langfuse:
            span = self.trace.span(name="tts", input={"text": text})

        for chunk in self.mdl.tts(text):
            if isinstance(chunk, int):
                if not TenantLLMService.increase_usage(self.tenant_id, self.llm_type, chunk, self.llm_name):
                    logging.error("LLMBundle.tts can't update token usage for {}/TTS".format(self.tenant_id))
                return
            yield chunk

        if self.langfuse:
            span.end()

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

    def chat(self, system, history, gen_conf):
        if self.langfuse:
            generation = self.trace.generation(name="chat", model=self.llm_name, input={"system": system, "history": history})

        chat = self.mdl.chat
        if self.is_tools and self.mdl.is_tools:
            chat = self.mdl.chat_with_tools

        txt, used_tokens = chat(system, history, gen_conf)
        txt = self._remove_reasoning_content(txt)

        if isinstance(txt, int) and not TenantLLMService.increase_usage(self.tenant_id, self.llm_type, used_tokens, self.llm_name):
            logging.error("LLMBundle.chat can't update token usage for {}/CHAT llm_name: {}, used_tokens: {}".format(self.tenant_id, self.llm_name, used_tokens))

        if self.langfuse:
            generation.end(output={"output": txt}, usage_details={"total_tokens": used_tokens})

        return txt

    def chat_streamly(self, system, history, gen_conf):
        if self.langfuse:
            generation = self.trace.generation(name="chat_streamly", model=self.llm_name, input={"system": system, "history": history})

        ans = ""
        chat_streamly = self.mdl.chat_streamly
        total_tokens = 0 
        if self.is_tools and self.mdl.is_tools:
            chat_streamly = self.mdl.chat_streamly_with_tools

        for txt in chat_streamly(system, history, gen_conf):
            if isinstance(txt, int):
                total_tokens = txt
                if self.langfuse:
                    generation.end(output={"output": ans})
                break

            if txt.endswith("</think>"):
                ans = ans.rstrip("</think>")

            ans += txt
            yield ans
        if total_tokens > 0:
            if not TenantLLMService.increase_usage(self.tenant_id, self.llm_type, txt, self.llm_name):
                logging.error("LLMBundle.chat_streamly can't update token usage for {}/CHAT llm_name: {}, content: {}".format(self.tenant_id, self.llm_name, txt))
