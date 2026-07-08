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
import logging
import random
from copy import deepcopy

import xxhash

from agent.component.llm import LLMParam, LLM
from api.db.joint_services.tenant_model_service import get_model_config_from_provider_instance
from api.db.services.document_service import DocumentService
from api.db.services.llm_service import LLMBundle
from api.db.services.task_service import has_canceled
from common.constants import LLMType
from rag.advanced_rag.knowlege_compile.runner import (
    DOC_STRUCTURE_COMPILE_BATCH_CHUNKS,
    load_active_templates,
    resolve_template_ids_from_groups,
    run_structure_compile_over_batches,
    split_tree_templates,
)
from rag.flow.base import ProcessBase, ProcessParamBase


class CompilerParam(ProcessParamBase, LLMParam):
    """Parameters for the knowledge-Compiler flow component.

    Same LLM-backed shape as the Extractor, but instead of a single inline
    ``knowledge_compilation`` config it drives compilation from one or more
    saved **compilation-template groups** (``compilation_template_group_ids``).
    Each group resolves to a set of templates, each of which carries its own
    structure-compilation config (kind, fields, synthesis, ...).
    """

    def __init__(self):
        super().__init__()
        self.compilation_template_group_ids = []

    def check(self):
        super().check()
        self.check_empty(self.compilation_template_group_ids, "Compilation Template Groups")


class Compiler(ProcessBase, LLM):
    component_name = "Compiler"

    def _compile_progress(self, prog=None, msg=""):
        """Adapt the knowledge-compile ``callback`` protocol to the flow
        callback. Downstream compile helpers invoke the callback either as
        ``callback(prog, msg)`` (positional) or ``callback(msg=...)``; the
        flow's ``self.callback`` expects ``(progress, message)``.
        """
        self.callback(prog, msg)

    def _compile_language(self, kwargs: dict) -> str:
        language = kwargs.get("language") or getattr(self._canvas, "_language", None)
        if isinstance(language, str):
            language = language.strip()
        if not language and getattr(self._canvas, "_doc_id", None):
            config = DocumentService.get_chunking_config(self._canvas._doc_id) or {}
            language = config.get("language")
            if isinstance(language, str):
                language = language.strip()
        return language or "English"

    async def _invoke(self, **kwargs):
        self.set_output("output_format", "chunks")
        self.callback(random.randint(1, 5) / 100.0, "Start knowledge compilation.")

        # Collect the upstream chunk list (same contract as the Extractor).
        inputs = self.get_input_elements()
        chunks = []
        for _, v in inputs.items():
            val = v["value"]
            if isinstance(val, list):
                chunks = deepcopy(val)

        tenant_id = self._canvas.get_tenant_id()
        doc_id = self._canvas._doc_id
        kb_id = getattr(self._canvas, "_kb_id", None) or DocumentService.get_knowledgebase_id(doc_id)
        language = self._compile_language(kwargs)

        if not chunks:
            self.set_output("chunks", chunks)
            return

        for ck in chunks:
            ck["doc_id"] = doc_id
            ck["id"] = xxhash.xxh64((ck["text"] + str(ck["doc_id"])).encode("utf-8")).hexdigest()

        # Resolve the configured template groups to concrete, active
        # (non-artifact) structure-compilation templates.
        template_ids = resolve_template_ids_from_groups(self._param.compilation_template_group_ids, tenant_id)
        active_templates = load_active_templates(template_ids, tenant_id)
        if not active_templates:
            self.callback(msg="No active compilation templates resolved from the configured groups.")
            self.set_output("chunks", chunks)
            return

        # Per-template chat model: a template may pin its own ``llm_id``;
        # otherwise fall back to this component's configured chat model.
        llm_bundle_cache: dict[str, LLMBundle] = {}
        chat_mdl_by_tid: dict[str, LLMBundle] = {}
        filtered_templates: list[tuple[str, dict]] = []
        default_chat_mdl = None
        for template_id, parser_cfg in active_templates:
            tpl_llm_id = parser_cfg.get("llm_id") if isinstance(parser_cfg, dict) else None
            if isinstance(tpl_llm_id, str) and tpl_llm_id.strip():
                chat_llm_id = tpl_llm_id.strip()
                if chat_llm_id not in llm_bundle_cache:
                    try:
                        cfg = get_model_config_from_provider_instance(tenant_id, LLMType.CHAT, chat_llm_id)
                        llm_bundle_cache[chat_llm_id] = LLMBundle(
                            tenant_id,
                            cfg,
                            lang=language,
                            max_retries=self._param.max_retries,
                            retry_interval=self._param.delay_after_error,
                        )
                    except Exception:
                        logging.exception(
                            "Compiler: cannot resolve chat model %s for template %s; skipping",
                            chat_llm_id,
                            template_id,
                        )
                        continue
                chat_mdl_by_tid[template_id] = llm_bundle_cache[chat_llm_id]
            else:
                if default_chat_mdl is None:
                    default_chat_mdl = LLMBundle(
                        tenant_id,
                        self.chat_mdl.model_config,
                        lang=language,
                        max_retries=self._param.max_retries,
                        retry_interval=self._param.delay_after_error,
                    )
                chat_mdl_by_tid[template_id] = default_chat_mdl
            filtered_templates.append((template_id, parser_cfg))

        if not filtered_templates:
            self.set_output("chunks", chunks)
            return
        active_templates = filtered_templates

        embedding_model = LLMBundle(
            tenant_id,
            LLMType.EMBEDDING,
            lang=language,
            max_retries=self._param.max_retries,
            retry_interval=self._param.delay_after_error,
        )

        tree_templates, non_tree_templates = split_tree_templates(active_templates)
        if tree_templates:
            # ``tree`` templates run RAPTOR over the whole document by
            # reloading vectors from the doc store; that path is owned by the
            # chunking task executor and isn't available from the flow.
            logging.warning(
                "Compiler: %d tree-kind template(s) are not supported in the flow pipeline; skipping",
                len(tree_templates),
            )
            self.callback(msg=f"Skipping {len(tree_templates)} tree-kind template(s) (unsupported in flow).")

        if non_tree_templates:
            task_id = getattr(self._canvas, "task_id", None)

            def _cancelled() -> bool:
                return bool(task_id) and has_canceled(task_id)

            async def _chunk_batches():
                for i in range(0, len(chunks), DOC_STRUCTURE_COMPILE_BATCH_CHUNKS):
                    yield chunks[i:i + DOC_STRUCTURE_COMPILE_BATCH_CHUNKS]

            await run_structure_compile_over_batches(
                active_templates=non_tree_templates,
                chat_mdl_by_tid=chat_mdl_by_tid,
                embedding_model=embedding_model,
                tenant_id=tenant_id,
                kb_id=kb_id,
                doc_id=doc_id,
                language=language,
                chunk_batches=_chunk_batches(),
                progress_cb=self._compile_progress,
                cancel_check=_cancelled,
            )

        self.set_output("chunks", chunks)
