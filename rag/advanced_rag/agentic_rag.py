#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

"""Agentic-RAG capability layer.

``RAGTools`` bundles every retrieval primitive the agentic-search graph
(:mod:`rag.advanced_rag.agentic_rag_graph`) needs — question formalisation,
document scoping, keyword analysis, KB / web / structured retrieval, a
sufficiency judge and follow-up-question generation — plus the two things
the *outer* LLM is ever allowed to call as tools: ``rag`` (run the whole
agentic-search graph) and ``summarize_document`` (dump one document for an
explicit summary request).

The individual search steps are deliberately NOT ``@tool``-decorated: the
graph orchestrates them itself, so ``chat_mdl`` stays a plain reasoning
model (no tool schema is bound onto it) and its ``async_chat*`` calls take
the fast non-tool-calling path.
"""

from copy import deepcopy
import logging
import re
from typing import Any, List

import json_repair
from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from common import settings
from common.misc_utils import thread_pool_exec
from common.token_utils import num_tokens_from_string
from rag.advanced_rag.agentic_rag_graph import _strip_think_stream
from rag.app.tag import label_question
from rag.llm.tool_decorator import tool
from rag.prompts.generator import (
    citation_prompt,
    form_message,
    gen_meta_filter,
    kb_prompt,
    message_fit_in,
    multi_queries_gen,
    sufficiency_select,
)
from api.db.db_models import Document, Knowledgebase
from rag.utils.tavily_conn import Tavily


# Tokens held back from the model's context when fitting retrieved evidence
# into the sufficiency / follow-up prompts. The evidence sits in the MIDDLE of
# those templates (question first, JSON output rules last), so if the combined
# prompt overflows the downstream trimmer eats the output rules, not the
# evidence. Reserving headroom for the template skeleton + question + output
# lets us trim the evidence up front instead.
_EVIDENCE_PROMPT_RESERVE_TOKENS = 1024


class RAGTools:
    def __init__(
        self,
        tenant_ids: list[str],
        chat_mdl: LLMBundle,
        embed_mdl: LLMBundle | None = None,
        kb_ids: List[str] | None = None,
        kbs: list[Knowledgebase] | None = None,
        tav: Tavily | None = None,
        meta_data_filter: dict | None = None,
        user_defined_prompts: dict | None = None,
        do_refer: bool | None = True,
        thinking_mode: str = "medium",
    ):
        self.tenant_ids = tenant_ids
        self.chat_mdl = deepcopy(chat_mdl)
        self.embed_mdl = embed_mdl
        self.thinking_mode = thinking_mode
        self.field_map = {}
        self.sql_kbs = []
        self.kbs = []
        self.kb_ids = []

        def _exclude_sql_kb(kb):
            if kb.parser_config and "field_map" in kb.parser_config:
                self.field_map.update(kb.parser_config["field_map"])
                self.sql_kbs.append(kb)
            else:
                self.kbs.append(kb)
                self.kb_ids.append(kb.id)

        if kb_ids:
            for kb in KnowledgebaseService.get_by_ids(kb_ids):
                _exclude_sql_kb(kb)
        elif kbs:
            for kb in kbs:
                _exclude_sql_kb(kb)

        self.tav = tav
        self.meta_data_filter = meta_data_filter
        self.user_defined_prompts = user_defined_prompts or {}
        self.kbinfos = {"chunks": [], "doc_aggs": []}
        self.do_refer = do_refer
        # Citation pool shared with the final-answer node: the graph publishes
        # the chunks it actually used here (in the SAME order the answer's
        # ``[ID:n]`` markers index), so the caller can resolve references.
        self.kbinfos: dict[str, list] = {"chunks": [], "doc_aggs": []}

        # Per-request retrieval cache keyed by the effective query + scope, so
        # the same question is never retrieved twice within one turn (e.g.
        # pre_search vs. an identical claim search in orchestrator_loop).
        self.search_cache: dict = {}

        # The two tools the outer LLM may bind. They are NOT auto-bound here —
        # the agentic-search flow drives the graph directly — but callers that
        # want a tool surface can do ``chat_mdl.bind_tools(tools=rag_tools.tools)``.
        self.tools = [self.rag, self.summarize_document]

    # ------------------------------------------------------------------ #
    # Capability flags / cheap introspection
    # ------------------------------------------------------------------ #
    def has_unstructured(self) -> bool:
        return bool(self.kb_ids)

    def has_structured(self) -> bool:
        return bool(self.sql_kbs and self.field_map)

    def has_web(self) -> bool:
        return self.tav is not None

    def has_llm(self) -> bool:
        return self.chat_mdl is not None

    async def _fit_messages(self, system: str, user: str) -> list:
        """Fit system+user messages into the model's context window."""
        from rag.prompts.generator import form_message, message_fit_in

        _, msg = message_fit_in(form_message(system, user), self.chat_mdl.max_length)
        return msg

    def get_citation_guidelines(self) -> str:
        """Return the citation guidelines the final answer must follow."""
        return citation_prompt(self.user_defined_prompts)

    def sys_prompt(self) -> str:
        """Thin router prompt for callers that bind ``self.tools``.

        The heavy workflow now lives inside the ``rag`` graph, so the outer
        model only has to decide between answering-with-retrieval (``rag``)
        and an explicit single-document summary (``summarize_document``).
        """
        summarize_line = (
            "- Call `summarize_document` ONLY when the user explicitly asks to summarise a specific document ('summarise the security audit', 'tldr the onboarding guide'). It needs a document ID.\n"
            if self.has_unstructured()
            else ""
        )
        return (
            "You are a smart agent. For any question that needs "
            "evidence from the knowledge bases or the web, call the `rag` tool "
            "with a self-contained question — it runs the full search-and-answer "
            "pipeline and returns a cited answer.\n"
            "After the `rag` tool returns, do not call `rag` again for the same "
            "user question. Use the returned cited answer as the final answer "
            "unless the user explicitly asks a new question.\n"
            f"{summarize_line}"
            "Do not invent facts and do not fabricate document IDs."
        )

    # ------------------------------------------------------------------ #
    # Graph node helpers (plain async methods — never exposed as tools)
    # ------------------------------------------------------------------ #
    async def formalize(self, messages: List[Any]) -> tuple[str, str]:
        """Rewrite the latest user message into a standalone question AND derive
        its search keywords (each with close synonyms), in one LLM call.

        ``messages`` may be a list of role dicts (``{"role", "content"}``) or
        pre-formatted ``"Speaker: text"`` strings.

        Returns ``(question, keywords)`` where ``keywords`` is a comma-separated
        string of the question's key terms plus 1-2 close synonyms / alternative
        phrasings for each, in the same language as the question.
        """
        if not messages:
            return "", ""

        lines: list[str] = []
        last_user = ""
        for m in messages:
            if isinstance(m, str):
                lines.append(m)
                last_user = m
                continue
            role = m.get("role", "user")
            content = m.get("content", "") or ""
            if role == "user":
                last_user = content
            prefix = "User" if role == "user" else ("Assistant" if role == "assistant" else str(role).capitalize())
            lines.append(f"{prefix}: {content}")
        transcript = "\n".join(lines)

        system = (
            "You are given a conversation. Do BOTH of the following and return JSON only:\n"
            "1. Rewrite the LAST user message into a single, self-contained question that can be "
            "understood without seeing the prior conversation — resolve pronouns, ellipses and "
            "follow-up shortcuts using earlier turns. Preserve the original language of the last "
            "user message. If it is already a complete standalone question, keep it unchanged.\n"
            "2. Extract keywords ONLY from the wording of the STANDALONE QUESTION itself — the "
            "salient content words and phrases that literally appear in it (key nouns, named "
            "entities, domain terms). Do NOT answer the question, and do NOT include any term that "
            "would be part of the answer or is not present in the question. Then, for each extracted "
            "term, you MAY add 1-2 close synonyms or alternative phrasings OF THAT SAME TERM. Output "
            "them all together as one comma-separated list, in the SAME language as the question.\n"
            '   Example — question "In which year did Apple acquire Beats?": keywords = '
            '"Apple, Apple Inc., acquire, acquisition, Beats" (terms from the question + synonyms; '
            'the year is the ANSWER, so it must NOT appear).\n\n'
            'Output ONLY JSON, no prose, no code fences: '
            '{"question": "<standalone question>", "keywords": "<term1, term2, synonym1, ...>"}'
        )
        user = f"Conversation:\n{transcript}\n\nOutput JSON:"
        _, msg = message_fit_in(form_message(system, user), self.chat_mdl.max_length)
        ans = await self.chat_mdl.async_chat(msg[0]["content"], msg[1:], {"temperature": 0.1})
        if isinstance(ans, tuple):
            ans = ans[0]
        cleaned = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
        cleaned = re.sub(r"```(?:json)?\s*|\s*```", "", cleaned).strip()
        try:
            data = json_repair.loads(cleaned)
        except Exception as e:
            logging.warning(f"formalize could not parse LLM output: {e!r} raw={ans[:200]!r}")
            data = {}
        if not isinstance(data, dict):
            data = {}

        question = str(data.get("question") or "").strip().strip('"').strip("'")
        if not question:
            # Fall back to the raw last user message rather than an empty question.
            question = (last_user or "").strip()

        keywords = data.get("keywords") or ""
        if isinstance(keywords, list):
            keywords = ", ".join(str(k).strip() for k in keywords if str(k).strip())
        keywords = str(keywords).strip()
        return question, keywords

    async def pick_documents(self, question: str) -> List[str] | None:
        """Narrow the search to a document subset for ``question``.

        Uses document metadata when the bound KBs carry any (mirrors the old
        ``filter_docs_by_metadata``); otherwise asks an LLM to pick relevant
        titles (mirrors the old ``select_documents``). Returns ``None`` when
        no useful scope can be derived, meaning "search everything".
        """
        return None
        if not self.kb_ids:
            return None

        metas = await self._get_cached_metas()
        if metas:
            ids = await self._filter_by_metadata(question, metas)
            return ids or None

        ids = await self._select_by_titles(question)
        return ids or None

    async def _filter_by_metadata(self, question: str, metas: dict) -> List[str]:
        filters = await gen_meta_filter(self.chat_mdl, metas, question)
        logging.debug(f"Metadata filter(auto) generated: {filters}")
        conditions = filters.get("conditions") or []
        if not conditions:
            return []
        logic = filters.get("logic", "and")
        try:
            doc_ids = await thread_pool_exec(
                DocMetadataService.filter_doc_ids_by_meta_pushdown,
                self.kb_ids,
                conditions,
                logic,
            )
        except Exception as e:
            logging.error(f"Metadata filter push down errored: {e}")
            return []
        return doc_ids or []

    async def _select_by_titles(self, question: str, max_docs: int = 512) -> List[str]:
        docs = await thread_pool_exec(self._collect_doc_titles, max_docs)
        if not docs:
            return []

        catalogue = "\n".join(f"docID: {doc_id}, title: {title}" for doc_id, title in docs)
        system = (
            "You filter a document catalogue to find which documents are relevant "
            "to a user's question. Use ONLY the titles in the catalogue — do not "
            "invent docIDs. "
            "Output ONLY a JSON array of the docIDs you consider relevant, e.g. "
            '["abc123", "def456"]. If no document is clearly relevant, output []. '
            "No explanations, no Markdown, no code fences, no prose around the array."
        )
        user = f"Question:\n{question}\n\nDocuments:\n{catalogue}\n\nRelevant docIDs (JSON array):"
        _, msg = message_fit_in(form_message(system, user), self.chat_mdl.max_length)
        ans = await self.chat_mdl.async_chat(msg[0]["content"], msg[1:], {"temperature": 0.1})
        if isinstance(ans, tuple):
            ans = ans[0]
        cleaned = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
        cleaned = re.sub(r"```(?:json)?\s*|\s*```", "", cleaned).strip()
        try:
            ids = json_repair.loads(cleaned)
        except Exception as e:
            logging.warning(f"select_by_titles could not parse LLM output: {e!r} raw={ans[:200]!r}")
            return []
        if not isinstance(ids, list):
            return []
        known = {doc_id for doc_id, _ in docs}
        return [doc_id for doc_id in ids if isinstance(doc_id, str) and doc_id in known]

    async def extract_keywords(self, question: str) -> str:
        """Produce a compact keyword string (terms + a few close synonyms).

        Replaces the keywords the outer LLM used to hand to the retrieval
        tool. Falls back to the question itself when extraction fails.
        """
        if not question:
            return ""
        system = (
            "Extract the search terms for a knowledge-base query from the "
            "question below. Output 3-8 of the most important content terms, "
            "plus 1-2 close synonyms or alternative phrasings for any ambiguous "
            "term. Single words or short noun phrases, space-separated, in the "
            "SAME language as the question. Output ONLY the terms — no labels, "
            "no punctuation lists, no explanation."
        )
        try:
            _, msg = message_fit_in(form_message(system, question), self.chat_mdl.max_length)
            ans = await self.chat_mdl.async_chat(msg[0]["content"], msg[1:], {"temperature": 0.2})
            if isinstance(ans, tuple):
                ans = ans[0]
            ans = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL).strip()
        except Exception:
            logging.exception("extract_keywords failed")
            ans = ""
        return ans or question

    async def retrieve(
        self,
        question: str,
        keywords: str | list = "",
        doc_scope: List[str] | None = None,
        top_n: int = 6,
        similarity_threshold: float = 0.2,
        using_embedding: bool = False,
    ) -> dict[str, list]:
        """Retrieve chunks from the unstructured KBs for one question.

        Returns a raw ``{"chunks": [...], "doc_aggs": [...]}`` dict — no
        citation stamping, no accumulation onto ``self.kbinfos`` (the graph
        owns merging so parallel per-question retrieval stays race-free).
        """
        if not self.kb_ids:
            return {"chunks": [], "doc_aggs": []}
        if isinstance(keywords, list):
            keywords = ",".join(keywords)
        logging.info(f"@retrieve: {question}@{keywords}")

        if doc_scope:
            candidates = [d for d in doc_scope if isinstance(d, str)]
            known = await thread_pool_exec(self._filter_known_doc_ids, candidates)
            valid = [d for d in candidates if d in known]
            if valid:
                doc_scope = valid
            else:
                if candidates:
                    logging.warning("retrieve: every supplied doc ID was unknown; falling back to unfiltered retrieval")
                doc_scope = None

        search_terms = keywords.strip() if keywords else ""
        if not search_terms:
            search_terms = question
        else:
            question = question + " " + search_terms

        embd_mdl = self.embed_mdl if using_embedding else None
        vector_weight = 0.7 if embd_mdl else 0
        kbinfos = await settings.retriever.retrieval(
            question,
            embd_mdl,
            self.tenant_ids,
            self.kb_ids,
            1,
            top_n,
            similarity_threshold,
            vector_similarity_weight=vector_weight,
            aggs=True,
            highlight=True,
            doc_ids=doc_scope,
            rank_feature=label_question(question, self.kbs),
        )
        if not kbinfos:
            return {"chunks": [], "doc_aggs": []}
        kbinfos["chunks"] = settings.retriever.retrieval_by_children(kbinfos.get("chunks", []), self.tenant_ids)
        return {"chunks": kbinfos.get("chunks", []), "doc_aggs": kbinfos.get("doc_aggs", [])}

    async def web_retrieve(self, query: str) -> dict[str, list]:
        """Retrieve chunks from the public web (Tavily). Raw kbinfos shape."""
        if self.tav is None:
            return {"chunks": [], "doc_aggs": []}
        try:
            tav_res = await thread_pool_exec(self.tav.retrieve_chunks, query)
        except Exception:
            logging.exception("web_retrieve failed")
            return {"chunks": [], "doc_aggs": []}
        return {"chunks": tav_res.get("chunks", []), "doc_aggs": tav_res.get("doc_aggs", [])}

    async def structured_retrieve(self, question: str) -> dict[str, Any]:
        """Query the structured (tabular) KBs by translating to SQL.

        Returns ``{"answer": str, "chunks": [...], "doc_aggs": [...]}``. The
        answer is the natural-language SQL result the final node can weave in;
        the chunks/doc_aggs feed the shared citation pool.
        """
        if not self.has_structured():
            return {"answer": "", "chunks": [], "doc_aggs": []}

        # Lazy import — dialog_service constructs RAGTools.
        from api.db.services.dialog_service import use_sql

        sql_kb_ids = [kb.id for kb in self.sql_kbs]
        tenant_id = self.sql_kbs[0].tenant_id
        try:
            ans = await use_sql(question, self.field_map, tenant_id, self.chat_mdl, quota=True, kb_ids=sql_kb_ids)
        except Exception as e:
            logging.exception(f"structured_retrieve: use_sql failed: {e}")
            return {"answer": "", "chunks": [], "doc_aggs": []}
        if not ans:
            return {"answer": "", "chunks": [], "doc_aggs": []}
        reference = ans.get("reference") or {}
        return {
            "answer": ans.get("answer", "") or "",
            "chunks": reference.get("chunks") or [],
            "doc_aggs": reference.get("doc_aggs") or [],
        }

    def _fit_evidence(self, question: str, evidence_md: str) -> str:
        """Trim ``evidence_md`` so ``question`` + evidence + the prompt template
        stay inside the model's context window.

        ``message_fit_in`` keeps the small side (the question) whole and trims
        the large side (the evidence); we shrink the budget by a reserve so the
        template skeleton and JSON output rules still fit afterwards.
        """
        if not evidence_md:
            return evidence_md
        budget = max(256, self.chat_mdl.max_length - _EVIDENCE_PROMPT_RESERVE_TOKENS)
        _, msg = message_fit_in(form_message(question, evidence_md), budget)
        return msg[-1]["content"]

    async def judge_sufficiency(self, question: str, evidence_md: str) -> dict:
        """Judge whether ``evidence_md`` answers ``question`` and pick useful chunks.

        ``evidence_md`` must carry ``ID: n`` markers per chunk (as produced by
        ``kb_prompt``). Returns the verdict dict:
        ``{"is_sufficient": bool, "reasoning": str, "missing_information": [...],
        "useful_chunk_ids": [int, ...]}``.
        """
        evidence_md = self._fit_evidence(question, evidence_md)
        try:
            return await sufficiency_select(self.chat_mdl, question, evidence_md) or {}
        except Exception:
            logging.exception("judge_sufficiency failed")
            return {}

    async def gen_followups(self, question: str, query: str, missing: List[str], evidence_md: str) -> List[dict]:
        """Generate complementary follow-up (question, query) pairs for gaps."""
        evidence_md = self._fit_evidence(question, evidence_md)
        try:
            res = await multi_queries_gen(self.chat_mdl, question, query or question, missing or [], evidence_md) or {}
        except Exception:
            logging.exception("gen_followups failed")
            return []
        qs = res.get("questions") or []
        return [q for q in qs if isinstance(q, dict) and (q.get("question") or "").strip()]

    async def fetch_full_document(self, doc_id: str) -> dict[str, list]:
        """Fetch a whole document's chunks in reading order (raw kbinfos)."""
        if not self.kb_ids:
            return {"chunks": [], "doc_aggs": []}
        resolved = await thread_pool_exec(self._resolve_doc_tenant, doc_id)
        if resolved is None:
            logging.warning(f"fetch_full_document: doc_id {doc_id!r} not in any bound KB — refusing to fetch")
            return {"chunks": [], "doc_aggs": []}
        kb_id, tenant_id = resolved

        cks = []
        tokens = 0
        for offset in range(0, 10000, 128):
            chunks = await thread_pool_exec(
                settings.retriever.chunk_list,
                doc_id,
                tenant_id,
                [kb_id],
                max_count=offset + 128,
                offset=offset,
                fields=["content_with_weight", "docnm_kwd", "doc_id"],
                sort_by_position=True,
                retrieve_all=False,
            )
            if not chunks:
                break
            for ck in chunks:
                num = num_tokens_from_string(str(ck["content_with_weight"]))
                if tokens + num > self.chat_mdl.max_length:
                    break
                tokens += num
                cks.append(ck)
        if not cks:
            return {"chunks": [], "doc_aggs": []}
        doc_name = next((c.get("docnm_kwd") or "" for c in cks if c.get("docnm_kwd")), "")
        return {
            "chunks": cks,
            "doc_aggs": [{"doc_name": doc_name, "doc_id": doc_id, "count": len(cks)}],
        }

    # ------------------------------------------------------------------ #
    # Bound tools
    # ------------------------------------------------------------------ #
    @tool(timeout=600)
    async def rag(self, question: str) -> str:
        """Answer a question with evidence from the knowledge bases and the web.

        Runs the full agentic-search pipeline: it formalises the question,
        narrows the document scope, analyses keywords, retrieves evidence,
        checks whether the evidence is sufficient (looping with follow-up
        searches when it is not), and finally composes a cited answer.

        :param question: a self-contained natural-language question.

        :returns: the composed answer with inline citation markers.
        """
        from rag.advanced_rag.agentic_rag_graph import run_agentic_rag

        messages = [{"role": "user", "content": question}] if question else []
        final = ""
        async for delta in _strip_think_stream(run_agentic_rag(self, messages)):
            if isinstance(delta, str):
                final += delta
        for p, r in [(r"\(\**(ID:\d)\**\)", "[\1]")]:
            final = re.sub(p, r, final)
        return final

    @tool
    async def summarize_document(self, doc_id: str) -> list[str]:
        """Return a single document's content, position-ordered, ready to summarise.

        Call ONLY for an explicit summary request about a specific document.
        For general Q&A use the `rag` tool instead.

        :param doc_id: a 32-character lowercase hex document ID that some
            other tool returned in this turn. Never invent one.

        :returns: formatted chunk blocks (document order) fitting the model's
            context budget, prefixed with the citation rules to apply.
        """
        kbinfos = await self.fetch_full_document(doc_id)
        if not kbinfos["chunks"]:
            return []
        start_idx = len(self.kbinfos.get("chunks", []))
        self.kbinfos["chunks"].extend(kbinfos["chunks"])
        self.kbinfos["doc_aggs"].extend(kbinfos["doc_aggs"])
        blocks = kb_prompt(self.kbinfos, self.chat_mdl.max_length)
        if not self.do_refer:
            return blocks[start_idx:] if start_idx else blocks
        header = "# Citation rules\nApply the following rules VERBATIM to your final answer.\n\n" + citation_prompt(self.user_defined_prompts).strip() + "\n\n----\n\n"
        return [header] + (blocks[start_idx:] if start_idx else blocks)

    # ------------------------------------------------------------------ #
    # Low-level DB helpers (sync — wrap in thread_pool_exec at call sites)
    # ------------------------------------------------------------------ #
    async def _get_cached_metas(self) -> dict:
        cached = getattr(self, "_metas_cache", None)
        if cached is not None:
            return cached
        if not self.kb_ids:
            self._metas_cache = {}
            return self._metas_cache
        self._metas_cache = await thread_pool_exec(DocMetadataService.get_flatted_meta_by_kbs, self.kb_ids)
        return self._metas_cache or {}

    def _collect_doc_titles(self, max_docs: int = 512) -> list[tuple[str, str]] | None:
        result: list[tuple[str, str]] = []
        for kb_id in self.kb_ids:
            for doc in DocumentService.query(kb_id=kb_id):
                result.append((doc.id, doc.name))
                if len(result) >= max_docs:
                    return None
        return result

    def _filter_known_doc_ids(self, candidate_ids: list[str]) -> set[str]:
        if not candidate_ids or not self.kb_ids:
            return set()
        rows = Document.select(Document.id).where((Document.id.in_(list(candidate_ids))) & (Document.kb_id.in_(self.kb_ids)))
        return {row.id for row in rows}

    def _resolve_doc_tenant(self, doc_id: str) -> tuple[str, str] | None:
        rows = list(Document.select(Document.kb_id).where((Document.id == doc_id) & (Document.kb_id.in_(self.kb_ids))))
        if not rows:
            return None
        kb_id = rows[0].kb_id
        for kb in self.kbs:
            if kb.id == kb_id:
                return kb_id, kb.tenant_id
        return None
