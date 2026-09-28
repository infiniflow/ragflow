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
from common.constants import LLMType
from api.db.db_models import DB, LLMFactories, TenantLLM
from api.db.services.common_service import CommonService
from api.db.services.langfuse_service import TenantLangfuseService


class LLMFactoriesService(CommonService):
    model = LLMFactories


class TenantLLMService(CommonService):
    model = TenantLLM

    @classmethod
    @DB.connection_context()
    def model_instance(cls, model_config: dict, lang="Chinese", **kwargs):
        if not model_config:
            raise LookupError("Model config is required")
        from rag.llm import ChatModel, CvModel, EmbeddingModel, OcrModel, RerankModel, Seq2txtModel, TTSModel

        kwargs.update({"provider": model_config["llm_factory"]})
        api_key = model_config.get("api_key_payload", model_config["api_key"])
        if model_config["model_type"] == LLMType.EMBEDDING.value:
            if model_config["llm_factory"] not in EmbeddingModel:
                logging.error("Factory not in embedding model. Supported factories: %s", list(EmbeddingModel.keys()))
                return None
            return EmbeddingModel[model_config["llm_factory"]](api_key, model_config["llm_name"], base_url=model_config["api_base"])

        elif model_config["model_type"] == LLMType.RERANK.value:
            if model_config["llm_factory"] not in RerankModel:
                logging.error("Factory not in rerank model. Supported factories: %s", list(RerankModel.keys()))
                return None
            return RerankModel[model_config["llm_factory"]](api_key, model_config["llm_name"], base_url=model_config["api_base"], max_token=model_config.get("max_tokens"))

        elif model_config["model_type"] == LLMType.VISION.value:
            if model_config["llm_factory"] not in CvModel:
                logging.error("Factory not in cv model. Supported factories: %s", list(CvModel.keys()))
                return None
            return CvModel[model_config["llm_factory"]](api_key, model_config["llm_name"], lang, base_url=model_config["api_base"], **kwargs)

        elif model_config["model_type"] == LLMType.CHAT.value:
            if model_config["llm_factory"] not in ChatModel:
                logging.error("Factory not in chat model. Supported factories: %s", list(ChatModel.keys()))
                return None
            return ChatModel[model_config["llm_factory"]](api_key, model_config["llm_name"], base_url=model_config["api_base"], **kwargs)

        elif model_config["model_type"] == LLMType.ASR.value:
            if model_config["llm_factory"] not in Seq2txtModel:
                logging.error("Factory not in asr model. Supported factories: %s", list(Seq2txtModel.keys()))
                return None
            return Seq2txtModel[model_config["llm_factory"]](key=api_key, model_name=model_config["llm_name"], lang=lang, base_url=model_config["api_base"])
        elif model_config["model_type"] == LLMType.TTS.value:
            if model_config["llm_factory"] not in TTSModel:
                logging.error("Factory not in tts model. Supported factories: %s", list(TTSModel.keys()))
                return None
            return TTSModel[model_config["llm_factory"]](
                api_key,
                model_config["llm_name"],
                base_url=model_config["api_base"],
            )

        elif model_config["model_type"] == LLMType.OCR.value:
            if model_config["llm_factory"] not in OcrModel:
                logging.error("Factory not in ocr model. Supported factories: %s", list(OcrModel.keys()))
                return None
            return OcrModel[model_config["llm_factory"]](
                key=api_key,
                model_name=model_config["llm_name"],
                base_url=model_config.get("api_base", ""),
                **kwargs,
            )

        return None


class LLM4Tenant:
    def __init__(self, tenant_id: str, model_config: dict, lang="Chinese", **kwargs):
        self.trace_context = kwargs.pop("trace_context", None) or {}
        self.langfuse_session_id = kwargs.pop("langfuse_session_id", None)
        self.tenant_id = tenant_id
        self.lang = lang
        self.llm_name = model_config["llm_name"]
        self.model_config = model_config
        self.mdl = TenantLLMService.model_instance(model_config, lang=lang, **kwargs)
        assert self.mdl, "Can't find model for {}/{}/{}".format(tenant_id, model_config["model_type"], model_config["llm_name"])
        self.max_length = model_config.get("max_tokens") or 8192

        self.is_tools = model_config.get("is_tools", False)
        self.verbose_tool_use = kwargs.get("verbose_tool_use")

        langfuse_keys = TenantLangfuseService.filter_by_tenant(tenant_id=tenant_id)
        self.langfuse = None
        if langfuse_keys:
            langfuse = Langfuse(public_key=langfuse_keys.public_key, secret_key=langfuse_keys.secret_key, host=langfuse_keys.host)
            try:
                if langfuse.auth_check():
                    self.langfuse = langfuse
                    if not self.trace_context:
                        trace_id = self.langfuse.create_trace_id()
                        self.trace_context = {"trace_id": trace_id}
            except Exception:
                # Skip langfuse tracing if connection fails
                pass

    def close(self):
        """Release resources held by this LLM4Tenant instance.

        IMPORTANT: do NOT call ``langfuse.flush()`` or ``langfuse.shutdown()``
        here. ``close()`` runs once per task, synchronously, on the asyncio
        event-loop thread of the task executor. Two problems follow:

        - ``flush()`` blocks on an unbounded ``queue.join()`` in the underlying
          OpenTelemetry span processor. If the exporter cannot drain (slow or
          unreachable Langfuse, or an already-shutdown processor) it never
          returns.
        - ``shutdown()`` permanently tears down the process-wide Langfuse /
          OpenTelemetry tracer provider that every ``LLMBundle`` shares. After
          the first task shuts it down, every subsequent ``flush()`` blocks
          forever.

        Because this runs on the event loop, a single stuck ``flush()`` freezes
        the entire task executor: all in-flight parse tasks stop making
        progress and no new tasks are ever picked up (observed as document
        parsing being stuck with every executor thread parked on a lock).

        Langfuse already exports spans from its own background processor and
        flushes at process exit, so releasing the reference is sufficient here.
        """
        # Release the Langfuse client reference. ``Langfuse.flush()`` waits on
        # ``Queue.join()`` with no timeout, so we never call it here: the shared
        # ``LangfuseResourceManager`` flushes its queues at process exit, and
        # a per-task ``flush()`` would block the task executor indefinitely
        # if a consumer is wedged. ``self.langfuse = None`` drops our handle so
        # the next ``LLM4Tenant`` reuses the same shared client.
        self.langfuse = None

        # Release underlying model instance if it has a close method
        if self.mdl and hasattr(self.mdl, "close") and callable(getattr(self.mdl, "close")):
            try:
                self.mdl.close()
            except Exception:
                logging.warning("LLM4Tenant.close: error while closing model instance", exc_info=True)
