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
import re
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
from common.token_utils import num_tokens_from_string
from rag.nlp import naive_merge
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
        self.check_empty(self.compilation_template_group_ids, "Compilation Template Groups")
        if isinstance(self.compilation_template_group_ids, str):
            self.compilation_template_group_ids = [self.compilation_template_group_ids]


class Compiler(ProcessBase, LLM):
    component_name = "Compiler"
    _PARSER_TEXT_CHUNK_TOKEN_SIZE = 512
    _PARSER_CANDIDATE_TOKEN_SIZE = 128
    _PARSER_MIN_SPLIT_CHARACTERS = 24

    @classmethod
    def _split_slumber_level(cls, text: str, delimiters: list[str]) -> list[str]:
        """Split at boundaries while keeping delimiters with the previous part."""
        if not text:
            return []
        pattern = "|".join(re.escape(delimiter) for delimiter in sorted(delimiters, key=len, reverse=True))
        if not pattern:
            return [text]

        parts = []
        start = 0
        for match in re.finditer(pattern, text, flags=re.DOTALL):
            end = match.end()
            if end - start >= cls._PARSER_MIN_SPLIT_CHARACTERS:
                parts.append(text[start:end])
                start = end
        if start < len(text):
            parts.append(text[start:])

        # Keep very short delimiter fragments with their neighbour, matching
        # Chonkie's min_chars behavior instead of producing tiny candidates.
        merged = []
        for part in parts:
            if merged and len(part.strip()) < cls._PARSER_MIN_SPLIT_CHARACTERS:
                merged[-1] += part
            else:
                merged.append(part)
        return [part for part in merged if part.strip()]

    @classmethod
    def _slumber_candidates(cls, text: str, level: int = 0) -> list[str]:
        """Apply Slumber's paragraph→sentence→pause→word→token hierarchy."""
        levels = [
            ["\n\n", "\r\n", "\n", "\r"],
            [". ", "! ", "? ", "。", "！", "？", "；", ";"],
            ["{", "}", '"', "[", "]", "<", ">", "(", ")", ":", "：", ";", "，", ",", "—", "|", "~", "-", "...", "`", "'"],
            [" "],
        ]
        if not text.strip():
            return []
        if level >= len(levels):
            return [part for part in naive_merge(text, cls._PARSER_TEXT_CHUNK_TOKEN_SIZE, "", 0) if part.strip()]

        splits = cls._split_slumber_level(text, levels[level])
        if len(splits) <= 1 and level < len(levels) - 1:
            return cls._slumber_candidates(text, level + 1)

        candidates = []
        for split in splits:
            if num_tokens_from_string(split) > cls._PARSER_CANDIDATE_TOKEN_SIZE:
                candidates.extend(cls._slumber_candidates(split, level + 1))
            else:
                candidates.append(split)
        return candidates

    @staticmethod
    def _is_atomic_json_record(record: dict) -> bool:
        """Keep structured or position-bound Parser records intact."""
        doc_type = str(record.get("doc_type_kwd") or "").strip().lower()
        layout_type = str(record.get("layout_type") or "").strip().lower()
        return bool(doc_type in {"table", "image"} or record.get("img_id") or record.get("image") or layout_type in {"figure", "table"} or record.get("bbox"))

    @classmethod
    def _normalize_upstream_chunks(cls, upstream: dict, split_json_text: bool = False) -> list[dict]:
        """Normalize direct Parser output into Compiler chunks.

        Parser JSON is already a list of chunk-like records, so preserve each
        record and its metadata. Textual Parser outputs need a lightweight
        token split before they enter the existing compilation pipeline.
        """
        output_format = upstream.get("output_format")
        if output_format == "chunks":
            chunks = upstream.get("chunks") or []
            return deepcopy(chunks) if isinstance(chunks, list) else []

        if output_format == "json":
            records = upstream.get("json") or upstream.get("json_result") or []
            if not isinstance(records, list):
                return []
            chunks = []
            for record in records:
                if not isinstance(record, dict):
                    continue
                chunk = deepcopy(record)
                text = chunk.get("text")
                if not isinstance(text, str):
                    text = chunk.get("content_with_weight")
                if isinstance(text, str) and text.strip():
                    if Compiler._is_atomic_json_record(chunk) or not split_json_text:
                        chunk["text"] = text
                        chunks.append(chunk)
                    else:
                        for part in Compiler._slumber_candidates(text):
                            split_chunk = deepcopy(chunk)
                            split_chunk["text"] = part
                            chunks.append(split_chunk)
            return chunks

        if output_format in {"markdown", "text", "html"}:
            text = upstream.get(output_format)
            if not isinstance(text, str):
                text = upstream.get(f"{output_format}_result")
            if not isinstance(text, str) or not text.strip():
                return []
            return [{"text": part} for part in cls._slumber_candidates(text) if part.strip()]

        return []

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
        from rag.advanced_rag.knowlege_compile.structure import (
            _struct_upsert_graph_json,
            _struct_upsert_tree_graph_rows,
        )
        from rag.svr.task_executor_refactor.chunk_post_processor import (
            raptor_tree_to_graph,
            rewrite_duplicate_tree_names,
        )
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
                    clustering_method="ahc",
                    max_errors=3,
                )
            except Exception:
                logging.exception("Compiler: tree-template %s build failed for doc %s", template_id, doc_id)
                continue
            if tree is None:
                continue

            if bool(raptor_cfg.get("rechunk")):
                self._compile_progress(msg="Compiler: tree rechunking is not supported for in-memory pipeline chunks; keeping original chunks.")

            await rewrite_duplicate_tree_names(tree, chat_mdl_by_tid[template_id])
            after_graph = raptor_tree_to_graph(tree)
            try:
                await _struct_upsert_tree_graph_rows(
                    after_graph,
                    tenant_id,
                    kb_id,
                    doc_id,
                    embedding_model,
                    compilation_template_id=template_id,
                )
                await _struct_upsert_graph_json(
                    after_graph,
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

                await upsert_dataset_nav_doc(
                    tenant_id,
                    kb_id,
                    doc_id,
                    tree,
                    embd_mdl=embedding_model,
                    chat_mdl=chat_mdl_by_tid[template_id],
                )
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
        chunks = self._normalize_upstream_chunks(kwargs)

        tenant_id = self._canvas.get_tenant_id()
        doc_id = self._canvas._doc_id
        kb_id = getattr(self._canvas, "_kb_id", None) or DocumentService.get_knowledgebase_id(doc_id)
        language = self._compile_language(kwargs)

        if not chunks:
            self.set_output("chunks", chunks)
            return

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

        def _template_requests_rechunk(cfg: dict) -> bool:
            if not isinstance(cfg, dict):
                return False
            if bool(cfg.get("rechunk")):
                return True
            raptor_cfg = cfg.get("raptor")
            return isinstance(raptor_cfg, dict) and bool(raptor_cfg.get("rechunk"))

        should_split_json = any(_template_requests_rechunk(cfg) for _, cfg in active_templates)
        if should_split_json and kwargs.get("output_format") == "json":
            chunks = self._normalize_upstream_chunks(kwargs, split_json_text=True)
            if not chunks:
                self.set_output("chunks", chunks)
                return

        for ck in chunks:
            ck["doc_id"] = doc_id
            ck["id"] = xxhash.xxh64((ck["text"] + str(ck["doc_id"])).encode("utf-8")).hexdigest()

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
