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
from types import SimpleNamespace

import xxhash

from agent.component.llm import LLMParam, LLM
from api.db.joint_services.tenant_model_service import get_model_config_by_id, get_tenant_default_model_by_type, resolve_model_config
from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
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
        if isinstance(self.compilation_template_group_ids, str):
            self.compilation_template_group_ids = [self.compilation_template_group_ids]


class Compiler(ProcessBase, LLM):
    component_name = "Compiler"

    def _compile_progress(self, prog=None, msg=""):
        """Adapt the knowledge-compile ``callback`` protocol to the flow
        callback. Downstream compile helpers invoke the callback either as
        ``callback(prog, msg)`` (positional) or ``callback(msg=...)``; the
        flow's ``self.callback`` expects ``(progress, message)``.
        """
        self.callback(0 if prog is None else prog, msg)

    async def _compile_tree_templates(
        self,
        templates: list[tuple[str, dict]],
        chat_mdl_by_tid: dict[str, LLMBundle],
        embedding_model: LLMBundle,
        chunks: list[dict],
        tenant_id: str,
        kb_id: str,
        doc_id: str,
    ) -> None:
        """Build and persist tree graphs from the pipeline's in-memory chunks.

        The document post-chunking path can reload chunks from the doc store,
        but a pipeline Compiler runs before DataflowService persists its final
        chunks. Supply RAPTOR with the same ``(text, vector, chunk_id)`` shape
        from the current pipeline output instead.
        """
        from rag.advanced_rag.knowlege_compile.structure import _struct_upsert_graph_json
        from rag.svr.task_executor_refactor.chunk_post_processor import raptor_tree_to_graph
        from rag.svr.task_executor_refactor.raptor_service import RaptorService

        tree_inputs = []
        texts = []
        for chunk in chunks:
            text = chunk.get("content_with_weight") or chunk.get("text") or ""
            if not isinstance(text, str) or not text.strip():
                continue
            chunk_id = str(chunk.get("id") or "")
            if not chunk_id:
                continue
            texts.append(text)
            tree_inputs.append((text, chunk_id))
        if not tree_inputs:
            return

        vectors, _ = embedding_model.encode(texts)
        tree_chunks = [(text, vector, chunk_id) for (text, chunk_id), vector in zip(tree_inputs, vectors)]
        if not tree_chunks:
            return

        tree_context = SimpleNamespace(
            tenant_id=tenant_id,
            kb_id=kb_id,
            doc_id=doc_id,
            id=getattr(self._canvas, "task_id", ""),
            progress_cb=self._compile_progress,
        )
        raptor_service = RaptorService(tree_context)

        for idx, (template_id, parser_cfg) in enumerate(templates):
            raptor_cfg = (parser_cfg or {}).get("raptor") or {}
            raptor_config = {
                "prompt": raptor_cfg.get("prompt") or "Please write a concise summary of the following texts:\n{cluster_content}",
                "max_token": int(raptor_cfg.get("max_token") or 512),
                "threshold": float(raptor_cfg.get("threshold") or 0.1),
                "random_seed": int(raptor_cfg.get("random_seed") or 0),
                "max_cluster": int(raptor_cfg.get("max_cluster") or 64),
                "ext": raptor_cfg.get("ext") or {},
            }
            self._compile_progress(msg=f"tree-template ({idx + 1}/{len(templates)}): building tree for doc={doc_id}")
            try:
                tree = await raptor_service.build_doc_tree(
                    chunks=tree_chunks,
                    raptor_config=raptor_config,
                    chat_mdl=chat_mdl_by_tid[template_id],
                    embd_mdl=embedding_model,
                    tree_builder="raptor",
                    clustering_method="gmm",
                    max_errors=3,
                )
            except Exception:
                logging.exception("Compiler: tree-template %s build failed for doc %s", template_id, doc_id)
                continue
            if tree is None:
                continue

            if bool(raptor_cfg.get("rechunk")):
                self._compile_progress(msg="Compiler: tree rechunking is not supported for in-memory pipeline chunks; keeping original chunks.")

            try:
                await _struct_upsert_graph_json(
                    raptor_tree_to_graph(tree),
                    tenant_id,
                    kb_id,
                    doc_id,
                    compile_kwd="tree",
                    compilation_template_id=template_id,
                )
            except Exception:
                logging.exception("Compiler: tree-template %s graph upsert failed for doc %s", template_id, doc_id)
                continue

            try:
                from rag.advanced_rag.knowlege_compile.dataset_nav import upsert_dataset_nav_doc

                await upsert_dataset_nav_doc(tenant_id, kb_id, doc_id, tree)
            except Exception:
                logging.exception("Compiler: tree-template %s dataset navigation upsert failed for doc %s", template_id, doc_id)

            self._compile_progress(msg=f"tree-template ({idx + 1}/{len(templates)}): persisted tree graph for doc {doc_id}")

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

        # Pipeline components receive the previous component's output as
        # kwargs. Do not call LLM.get_input_elements() here: it resolves the
        # inherited prompt variables through Canvas.globals, while Pipeline
        # is a Graph and has no globals.
        chunks = deepcopy(kwargs.get("chunks") or [])
        if not chunks:
            for val in kwargs.values():
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
            self.callback(0, "No active compilation templates resolved from the configured groups.")
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
                        cfg = resolve_model_config(tenant_id, LLMType.CHAT, chat_llm_id)
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

        if self._canvas._kb_id:
            e, kb = KnowledgebaseService.get_by_id(self._canvas._kb_id)
            if kb.tenant_embd_id:
                try:
                    embd_model_config = get_model_config_by_id(self._canvas._tenant_id, LLMType.EMBEDDING, kb.tenant_embd_id)
                except LookupError:
                    embd_model_config = resolve_model_config(self._canvas._tenant_id, LLMType.EMBEDDING, kb.embd_id)
            else:
                embd_model_config = resolve_model_config(self._canvas._tenant_id, LLMType.EMBEDDING, kb.embd_id)
        else:
            embd_model_config = get_tenant_default_model_by_type(self._canvas._tenant_id, LLMType.EMBEDDING)
        embedding_model = LLMBundle(
            tenant_id,
            embd_model_config,
            lang=language,
            max_retries=self._param.max_retries,
            retry_interval=self._param.delay_after_error,
        )

        tree_templates, non_tree_templates = split_tree_templates(active_templates)
        if tree_templates:
            await self._compile_tree_templates(
                tree_templates,
                chat_mdl_by_tid,
                embedding_model,
                chunks,
                tenant_id,
                kb_id,
                doc_id,
            )

        if non_tree_templates:
            task_id = getattr(self._canvas, "task_id", None)

            def _cancelled() -> bool:
                return bool(task_id) and has_canceled(task_id)

            async def _chunk_batches():
                for i in range(0, len(chunks), DOC_STRUCTURE_COMPILE_BATCH_CHUNKS):
                    yield chunks[i : i + DOC_STRUCTURE_COMPILE_BATCH_CHUNKS]

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
