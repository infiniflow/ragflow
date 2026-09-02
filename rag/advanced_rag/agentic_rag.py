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

import logging
import re
from collections.abc import Callable
from typing import Any

import json_repair

from api.db.db_models import Document, Knowledgebase
from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from common import settings
from common.misc_utils import thread_pool_exec
from common.token_utils import num_tokens_from_string
from rag.advanced_rag.agentic_rag_graph import _split_think_stream
from rag.advanced_rag.harness.keywords import extract_weighted_keywords
from rag.advanced_rag.harness.stats import CountingChatModel, LLMUsageStats, in_phase, using_stats
from rag.advanced_rag.harness.tools.search import _compact_keywords, _resolve_rerank_candidates, _resolve_top_k, _setting
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
from rag.utils.web_search_conn import WebSearchProvider

# Tokens held back from the model's context when fitting retrieved evidence
# into the sufficiency / follow-up prompts. The evidence sits in the MIDDLE of
# those templates (question first, JSON output rules last), so if the combined
# prompt overflows the downstream trimmer eats the output rules, not the
# evidence. Reserving headroom for the template skeleton + question + output
# lets us trim the evidence up front instead.
_EVIDENCE_PROMPT_RESERVE_TOKENS = 1024

# Fixed evidence budget for ``_fit_evidence`` (sufficiency judge / follow-ups /
# formalize-answer evidence trimming). Kept far below the model context so a
# large retrieval pool never fills the window with evidence; each call stays
# cheap. ~8000 tokens ≈ 32K chars is enough to support a grounded answer.
_EVIDENCE_BUDGET_TOKENS = 8000

_LOG = logging.getLogger(__name__)

# P0: significant-keyword overlap above which a new `rag` question reuses a
# cached answer. Overlap = |shared| / min(|a|, |b|) over the question's
# significant words (stopwords dropped). 0.6 + ">=2 shared words" collapses the
# re-ask pattern in the logs ("legal population" → "estimated population of
# Paris in 2019", both overlap 0.75) while leaving genuinely different questions
# (Paris vs. Brown County = 0.25) untouched.
_RAG_CACHE_MIN_OVERLAP = 0.6
_RAG_CACHE_MIN_SHARED = 2

# Lightweight stopwords for the cross-`rag`-call dedup only. Never reused for
# retrieval/answer quality.
_RAG_CACHE_STOPWORDS = frozenset(
    [
        "the",
        "a",
        "an",
        "is",
        "was",
        "were",
        "what",
        "which",
        "when",
        "where",
        "who",
        "how",
        "of",
        "in",
        "to",
        "for",
        "and",
        "or",
        "but",
        "on",
        "at",
        "by",
        "be",
        "as",
        "it",
        "that",
        "this",
        "about",
        "with",
        "their",
        "its",
        "have",
        "has",
        "had",
        "been",
        "being",
        "from",
        "over",
        "under",
        "do",
        "does",
        "did",
        "not",
        "no",
        "yes",
        "can",
        "could",
        "should",
        "would",
        "also",
        "only",
        "very",
        "much",
        "more",
        "most",
        "some",
        "any",
    ]
)


def _resolve_effective_question(question: str, original_user_question: str) -> str:
    """Prefer the user's ORIGINAL, complete question over the outer model's
    rewritten `question` argument, but only when the two are clearly the SAME
    user turn (>=2 shared significant keywords). The outer smart agent's rewrite
    frequently drops the FINAL target of a multi-hop question (Q317 lost the
    purchaser's death date, Q305 lost the "shortest abbreviation" attribute) by
    compressing to the first hop; using the lossy rewrite means the inner graph
    answers a question whose answer-attribute was deleted, and no planner /
    sufficiency fix can recover it. Keyword-overlap guards against a genuine
    multi-turn re-ask for a DIFFERENT question, which must keep its own query.
    """
    if not question or not original_user_question:
        return question or ""
    _oq = (original_user_question or "").strip()
    if not _oq:
        return question
    _qk = set(_question_keywords(question)[0])
    _ok = set(_question_keywords(_oq)[0])
    if _qk and _ok and len(_qk & _ok) >= 2:
        return _oq
    return question


def _question_keywords(question: str) -> tuple[set[str], set[str]]:
    """(significant words, numeric tokens) of a question.

    For English (the observed re-ask pattern) plain tokenisation suffices; CJK
    text falls back to the whole-token as a single significant unit. Numeric
    tokens (years, figures) are returned separately so ``_cache_similar`` can
    refuse to collapse questions that differ in the number being asked about
    (e.g. "Paris population in 2019" vs "in 2015").
    """
    tokens = re.findall(r"[a-zA-Z0-9\u4e00-\u9fff]+", (question or "").lower())
    numbers = {t for t in tokens if t.isdigit()}
    sig = {t for t in tokens if t not in _RAG_CACHE_STOPWORDS and len(t) > 1 and not t.isdigit()}
    if not sig:
        sig = {t for t in tokens if len(t) > 1 and not t.isdigit()}
    return sig, numbers


def _cache_similar(
    a: tuple[set[str], set[str]],
    b: tuple[set[str], set[str]],
) -> bool:
    """True when a new question's significant words mostly overlap a cached one.

    Uses word overlap (shared / min cardinality) so "legal population of Paris
    2019" (5 sig words) is caught by "population of Paris 2019" (3 sig words)
    and vice-versa, while requiring >= 2 shared words. The numeric sets must
    either be both empty or identical: if the two questions name different
    years/figures they are NOT the same question and must not share an answer.
    """
    aw, an = a
    bw, bn = b
    if not aw or not bw:
        return False
    if an or bn:  # a question with an explicit number must match on it exactly
        if an != bn:
            return False
    shared = len(aw & bw)
    if shared < _RAG_CACHE_MIN_SHARED:
        return False
    return shared / min(len(aw), len(bw)) >= _RAG_CACHE_MIN_OVERLAP


class RAGTools:
    def __init__(
        self,
        tenant_ids: list[str],
        chat_mdl: LLMBundle,
        embed_mdl: LLMBundle | None = None,
        kb_ids: list[str] | None = None,
        kbs: list[Knowledgebase] | None = None,
        web_search: WebSearchProvider | None = None,
        meta_data_filter: dict | None = None,
        doc_scope: list[str] | None = None,
        user_defined_prompts: dict | None = None,
        empty_response: str = "",
        do_refer: bool | None = True,
        thinking_mode: str = "medium",
        text_attachments_content: str = "",
        original_user_question: str = "",
        system_prompt: str = "",
        similarity_threshold: float | None = None,
        vector_similarity_weight: float | None = None,
        top_n: int | None = None,
        rerank_candidates_count: int | None = None,
        top_k: int | None = None,
    ):
        self.tenant_ids = tenant_ids
        # Retrieval settings from the caller (a chat assistant's dialog row, an
        # agent's Retrieval component). The search tools fall back to their own
        # defaults on None, so an unconfigured caller is unaffected.
        self.similarity_threshold = similarity_threshold
        self.vector_similarity_weight = vector_similarity_weight
        self.top_n = top_n
        self.rerank_candidates_count = rerank_candidates_count
        self.top_k = top_k
        # The user's ORIGINAL, complete question as received from the chat layer
        # (before the outer smart agent may have rewritten/compressed it). The
        # outer LLM's `rag(question=...)` argument is model-generated and
        # frequently drops the FINAL target of a multi-hop question (Q317 lost
        # the purchaser's death, Q305 lost the "shortest abbreviation" attribute)
        # because it compresses the question to the first hop. When this original
        # is available and this is the same user turn, the `rag` tool must use it
        # instead of the model-rewritten argument, so the inner graph never
        # answers a question whose answer-attribute was silently deleted.
        self.original_user_question = original_user_question
        # P0 instrumentation: count LLM calls / token usage per harness phase.
        # The wrapper proxies every ``async_chat*`` entry point (and ``clone``)
        # of the bundle, keeping the rest of the harness untouched.
        self.llm_stats = LLMUsageStats()
        self.chat_mdl = CountingChatModel(chat_mdl.clone(), self.llm_stats)
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

        self.web_search = web_search
        self.meta_data_filter = meta_data_filter
        self.doc_scope = list(dict.fromkeys(doc_scope)) if doc_scope is not None else None
        self.user_defined_prompts = user_defined_prompts or {}
        self.empty_response = empty_response
        self.do_refer = do_refer
        self.text_attachments_content = text_attachments_content or ""
        self.system_prompt = system_prompt or ""
        # Optional sink used by the outer agent stream to preserve the final
        # answer deltas produced by the inner research graph.  The tool API
        # still returns the complete string to the caller, but the stream
        # endpoint can forward the original deltas instead of that aggregate.
        self.answer_sink: Callable[[str, bool], None] | None = None
        self.tool_started_sink: Callable[[], None] | None = None
        # Citation pool shared with the final-answer node: the graph publishes
        # the chunks it actually used here (in the SAME order the answer's
        # ``[ID:n]`` markers index), so the caller can resolve references.
        self.kbinfos: dict[str, list] = {"chunks": [], "doc_aggs": []}

        # P0: cross-`rag`-call result cache keyed by a character n-gram digest of
        # the question. When a new sub-question is near-identical to one already
        # researched this turn (e.g. the user re-asks a Paris figure as
        # "legal population" then "commune inhabitants"), we reuse the cached
        # answer instead of re-running the whole agentic graph. Conservative
        # threshold (≥0.85 n-gram Jaccard) so near-identical phrasing is caught
        # without confusing genuinely different questions.
        self._rag_cache: dict[str, tuple[str, set[str]]] = {}
        # Sufficiency verdict of the most recent agentic-graph run, used by the outer
        # loop (via `rag`) to decide whether to re-run research and how to rephrase
        # the next question. Set in run_agentic_rag.
        self._rag_verdict: dict | None = None
        # Outer-loop guardrail: count of consecutive rag calls that ended
        # UNANSWERABLE / INSUFFICIENT (evidence conflicts or missing). Once this
        # exceeds a threshold, `rag` tells the outer LLM to stop re-running, so we
        # keep the outer loop's question-rewrite value while bounding useless retries.
        self._consecutive_unanswerable = 0

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
        return self.web_search is not None

    def has_llm(self) -> bool:
        return self.chat_mdl is not None

    def scoped_doc_ids(self, doc_scope: list[str] | None = None) -> list[str] | None:
        if self.doc_scope is None:
            return doc_scope
        if not doc_scope:
            return list(self.doc_scope)
        allowed = set(self.doc_scope)
        return [doc_id for doc_id in doc_scope if doc_id in allowed]

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
        router_prompt = (
            "You are a smart agent. For any question that needs "
            "evidence from the knowledge bases or the web, call the `rag` tool "
            "with a self-contained question — it runs the full search-and-answer "
            "pipeline and returns a cited answer.\n"
            "After the `rag` tool returns, do not call `rag` again for the same "
            "user question. Use the returned cited answer as the final answer "
            "unless the user explicitly asks a new question.\n"
            "CRITICAL — preserve the full multi-hop structure when phrasing the "
            "`rag` question. A question that compares two or more DISTINCT "
            'targets or needs an arithmetic result across them ("how much taller '
            'is X than Y", "how many days after A\'s death did B die", "which of '
            'these was discovered last") MUST keep every target and relation in '
            "the question you pass to `rag`. Never rewrite a comparison into a "
            "single-entity question — dropping the second entity (e.g. the "
            'purchaser in "how many days after his death did the man who '
            'purchased it in 1933 die") makes the pipeline answer only the first '
            "part. Pass the complete comparison.\n"
            f"{summarize_line}"
            "Do not invent facts and do not fabricate document IDs."
        )
        if self.system_prompt:
            return f"{self.system_prompt}\n\n{router_prompt}"
        return router_prompt

    # ------------------------------------------------------------------ #
    # Graph node helpers (plain async methods — never exposed as tools)
    # ------------------------------------------------------------------ #
    @in_phase("formalize")
    async def formalize(self, messages: list[Any]) -> tuple[str, str]:
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

        user_msgs = [m for m in messages if (isinstance(m, str) or m.get("role", "user") == "user")]
        multi_turn = len(user_msgs) > 1
        if not multi_turn and last_user:
            _LOG.info("[Formalize] Single-turn self-contained question — kept verbatim (no rewrite): %s", last_user.strip()[:120])
            try:
                keywords = await self.extract_keywords(last_user)
            except Exception as exc:
                _LOG.info("[Formalize] extract_keywords failed for single-turn question: %s", exc)
                keywords = ""
            return last_user.strip(), keywords

        system = (
            "You are given a conversation. Do BOTH of the following and return JSON only:\n"
            "1. Rewrite the LAST user message into a single, self-contained question that can be "
            "understood without the prior conversation — resolve pronouns, ellipses and follow-up "
            "shortcuts using the earlier turns. In most cases, it should be EXACTLY THE SAME as the "
            "last user query — only rewrite when there is something to resolve (a pronoun/ellipsis "
            "pointing back at an earlier turn). Preserve the original language.\n"
            "2. Extract keywords for a keyword search: the salient content words and phrases that "
            "literally appear in the (standalone) question — key nouns, named entities, domain "
            "terms — PLUS 2-3 close synonyms/abbreviations/aliases/alternative spellings of each, "
            "in the SAME language as the question. Maximize recall. Do NOT include terms that would "
            "be part of the answer.\n"
            '   Example — "In which year did Apple acquire Beats?" -> keywords = "Apple, Apple '
            'Inc., AAPL, acquire, acquisition, acquired, Beats, Beats Electronics".\n\n'
            "Output ONLY JSON, no prose, no code fences: "
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

        # Multi-turn: keywords come from the SAME single LLM call (the JSON above
        # outputs both question and keywords) — no second LLM round-trip.
        keywords = data.get("keywords") or ""
        if isinstance(keywords, list):
            keywords = ", ".join(str(k).strip() for k in keywords if str(k).strip())
        keywords = str(keywords).strip()
        # Same source-level compacting as extract_keywords: the multi-turn
        # formalize prompt asks for synonyms/aliases per term, which models over-
        # answer with a long redundant run. Cap + dedupe so it stays a hint.
        return question, _compact_keywords(keywords)

    async def pick_documents(self, question: str) -> list[str] | None:
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

    async def _filter_by_metadata(self, question: str, metas: dict) -> list[str]:
        filters = await gen_meta_filter(self.chat_mdl, metas, question)
        logging.debug(f"Metadata filter(auto) generated: {filters}")
        conditions = filters.get("conditions") or []
        if not conditions:
            return []
        logic = filters.get("logic", "and")
        try:
            # peewee MySQL lookup — call directly to reuse the pool's connection.
            doc_ids = DocMetadataService.filter_doc_ids_by_meta_pushdown(
                self.kb_ids,
                conditions,
                logic,
            )
        except Exception as e:
            logging.error(f"Metadata filter push down errored: {e}")
            return []
        return doc_ids or []

    async def _select_by_titles(self, question: str, max_docs: int = 512) -> list[str]:
        # peewee MySQL lookup — call directly to reuse the pool's connection.
        docs = self._collect_doc_titles(max_docs)
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

    async def _extract_keywords_weighted(self, question: str) -> tuple[str, str]:
        """Extract FOUR weighted keyword aspects for ``question``.

        Returns ``(query, keywords)``: ``query`` is the entity-weighted search
        string (entity terms repeated x3 so BM25 weights them up, other aspects
        once), ``keywords`` is the plain deduped union used to narrow retrieved
        chunks. Thin wrapper over ``harness.keywords.extract_weighted_keywords``.
        """
        return await extract_weighted_keywords(self.chat_mdl, question)

    async def extract_keywords(self, question: str) -> str:
        """Produce a compact keyword string (the deduped union of the four
        weighted aspects, plus close synonyms).

        Replaces the keywords the outer LLM used to hand to the retrieval
        tool. Falls back to the question itself when extraction fails.
        """
        if not question:
            return ""
        _weighted, union = await self._extract_keywords_weighted(question)
        return _compact_keywords(union)

    async def retrieve(
        self,
        question: str,
        keywords: str | list = "",
        doc_scope: list[str] | None = None,
        top_n: int | None = None,
        similarity_threshold: float | None = None,
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
        # Explicit argument wins, then the caller's configuration, then this
        # method's own defaults, which differ from the search tools' on purpose.
        if top_n is None:
            top_n = int(_setting(self, "top_n", 6))
        if similarity_threshold is None:
            similarity_threshold = _setting(self, "similarity_threshold", 0.2)
        logging.info(f"@retrieve: {question}@{keywords}")

        doc_scope = self.scoped_doc_ids(doc_scope)
        if doc_scope == ["-999"]:
            return {"chunks": [], "doc_aggs": []}
        if doc_scope:
            candidates = [d for d in doc_scope if isinstance(d, str)]
            # peewee MySQL lookup — call directly to reuse the pool's connection.
            known = self._filter_known_doc_ids(candidates)
            valid = [d for d in candidates if d in known]
            if valid:
                doc_scope = valid
            else:
                if self.doc_scope is not None:
                    return {"chunks": [], "doc_aggs": []}
                if candidates:
                    logging.warning("retrieve: every supplied doc ID was unknown; falling back to unfiltered retrieval")
                doc_scope = None

        search_terms = keywords.strip() if keywords else ""
        if not search_terms:
            search_terms = question
        else:
            question = question + " " + search_terms

        embd_mdl = self.embed_mdl if using_embedding else None
        vector_weight = _setting(self, "vector_similarity_weight", 0.7) if embd_mdl else 0
        knn_top_k = _resolve_top_k(self)
        rerank_candidates_count = _resolve_rerank_candidates(self, top_n)
        logging.debug(
            "retrieve: top_n=%s threshold=%s vector_weight=%s knn_top_k=%s rerank_candidates_count=%s",
            top_n,
            similarity_threshold,
            vector_weight,
            knn_top_k,
            rerank_candidates_count,
        )
        kbinfos = await settings.retriever.retrieval(
            question,
            embd_mdl,
            self.tenant_ids,
            self.kb_ids,
            1,
            top_n,
            similarity_threshold,
            vector_similarity_weight=vector_weight,
            knn_top_k=knn_top_k,
            aggs=True,
            highlight=True,
            doc_ids=doc_scope,
            rank_feature=label_question(question, self.kbs),
            rerank_candidates_count=rerank_candidates_count,
        )
        if not kbinfos:
            return {"chunks": [], "doc_aggs": []}
        kbinfos["chunks"] = settings.retriever.retrieval_by_children(kbinfos.get("chunks", []), self.tenant_ids)
        return {"chunks": kbinfos.get("chunks", []), "doc_aggs": kbinfos.get("doc_aggs", [])}

    async def web_retrieve(self, query: str) -> dict[str, list]:
        """Retrieve chunks from the public web. Raw kbinfos shape."""
        if self.web_search is None:
            return {"chunks": [], "doc_aggs": []}
        try:
            web_res = await thread_pool_exec(self.web_search.retrieve_chunks, query)
        except Exception:
            logging.exception("web_retrieve failed")
            return {"chunks": [], "doc_aggs": []}
        return {"chunks": web_res.get("chunks", []), "doc_aggs": web_res.get("doc_aggs", [])}

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
            ans = await use_sql(question, self.field_map, tenant_id, self.chat_mdl, quota=True, kb_ids=sql_kb_ids, doc_ids=self.scoped_doc_ids())
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
        stay inside a *bounded* budget.

        ``message_fit_in`` keeps the small side (the question) whole and trims
        the large side (the evidence). We use a FIXED budget (not the full model
        context) so a large retrieval pool can never fill the whole context
        window with evidence — each sufficiency/answer call stays cheap. Callers
        that want snippet-based evidence (``_narrow_by_keywords``) do so at the
        source; this is the final safety cap.
        """
        if not evidence_md:
            return evidence_md
        budget = _EVIDENCE_BUDGET_TOKENS
        _, msg = message_fit_in(form_message(question, evidence_md), budget)
        return msg[-1]["content"]

    @in_phase("sufficiency")
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

    @in_phase("sufficiency")
    async def gen_followups(self, question: str, query: str, missing: list[str], evidence_md: str) -> list[dict]:
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
        if self.doc_scope is not None and doc_id not in self.doc_scope:
            return {"chunks": [], "doc_aggs": []}
        # peewee MySQL lookup — call directly to reuse the pool's connection.
        resolved = self._resolve_doc_tenant(doc_id)
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
            budget_hit = False
            for ck in chunks:
                num = num_tokens_from_string(str(ck["content_with_weight"]))
                if tokens + num > self.chat_mdl.max_length:
                    budget_hit = True
                    break
                tokens += num
                cks.append(ck)
            if budget_hit:
                # Break the OUTER paging loop too. The bare `break` above only
                # exits the inner loop, so a document that hits the token budget
                # still ran all ~79 pages — pure waste, and enough of it (one
                # navigate_tree call used to fan this out over 8 documents) to
                # overwhelm the doc store.
                break
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

        if self.tool_started_sink is not None:
            self.tool_started_sink()
        # Per-call instrumentation: each `rag` invocation gets its own stats
        # object (bound through the per-task ContextVar), so parallel calls
        # report independent usage instead of one shared cumulative sink.
        with using_stats(LLMUsageStats()) as call_stats:
            # P0: reuse a near-identical question's cached answer instead of re-running
            # the whole agentic graph. Significant-keyword overlap (>= min_overlap AND
            # >=2 shared words, and matching numbers) collapses the re-ask pattern
            # while leaving genuinely different questions untouched. Attachments bypass
            # the cache (their content is appended to the question message below).
            if question and not self.text_attachments_content:
                qk = _question_keywords(question)
                # If the last research round was not sufficient, do not reuse a cached
                # (similarly-worded) answer — the outer loop asked again because it
                # needs more evidence, so re-run the graph instead of returning the
                # same incomplete answer (Google "iteration" phase).
                last_status = ""
                v = getattr(self, "_rag_verdict", None)
                if isinstance(v, dict):
                    last_status = str(v.get("status") or "")
                _cache_ok = not last_status or last_status == "SUFFICIENT"
                if _cache_ok and self._rag_cache:
                    for cached_q, (cached_answer, cached_gram) in list(self._rag_cache.items()):
                        if cached_gram and _cache_similar(qk, cached_gram):
                            shared = len(qk[0] & cached_gram[0])
                            _LOG.info("[Agentic RAG] Cache hit — reused prior answer for near-identical question %r (%d shared words); skipped research.", question, shared)
                            return cached_answer

            # Prefer the user's ORIGINAL, complete question over the outer model's
            # rewritten `question` argument (same-turn only). See
            # `_resolve_effective_question` for the rationale — the outer rewrite
            # drops the FINAL target of multi-hop questions (Q317 purchaser death,
            # Q305 "shortest abbreviation").
            effective_q = _resolve_effective_question(question or "", self.original_user_question)
            if effective_q and effective_q != question:
                _LOG.info(
                    "[Agentic RAG] Using original user question over outer rewrite (original=%r → rewrite=%r)",
                    self.original_user_question[:80],
                    (question or "")[:80],
                )
            messages = [{"role": "user", "content": effective_q}] if effective_q else []
            if self.text_attachments_content and messages:
                messages[-1]["content"] += self.text_attachments_content

            # Run the inner research executor (the static LangGraph pipeline).
            research = run_agentic_rag(self, messages)

            final = ""
            async for kind, delta in _split_think_stream(research):
                if kind == "answer":
                    final += delta
                if self.answer_sink is not None:
                    self.answer_sink(delta, kind == "think")
            final = re.sub(r"\(\**(ID:\d+)\**\)", r"[\1]", final)

            # Cache the freshly produced answer for later near-identical questions.
            if question and final and not self.text_attachments_content:
                self._rag_cache[question] = (final, _question_keywords(question))

            # Expose the sufficiency verdict to the outer LLM so it can decide whether
            # to re-run `rag` from the reported gaps (Google "missing pieces" feedback)
            # instead of guessing from the answer text. This is appended to the tool
            # result — it does NOT change the final answer (the graph's formalize_answer
            # composes that independently from kbinfos).
            #
            # Outer-loop guardrail: bound useless re-runs. If consecutive rag calls keep
            # ending insufficient (UNANSWERABLE / INSUFFICIENT / CONFLICTING), tell the
            # outer LLM to STOP re-running and answer from the existing evidence — the
            # corpus likely lacks the data, so re-querying different angles won't help
            # and only inflates latency / risks timeout.
            verdict = getattr(self, "_rag_verdict", None)
            insufficient_statuses = {"UNANSWERABLE", "INSUFFICIENT", "CONFLICTING"}
            if isinstance(verdict, dict):
                status = verdict.get("status")
                missing = verdict.get("missing_claims") or []
                feedback = verdict.get("feedback") or ""
                hard = verdict.get("hard_violations") or []
                conf = verdict.get("agent_confidence")
                if status and status != "SUFFICIENT":
                    status_hint = {
                        "USEFUL_BUT_INCOMPLETE": "evidence is partially sufficient (gaps remain)",
                        "INSUFFICIENT": "evidence is not yet sufficient",
                        "CONFLICTING": "evidence contains conflicts",
                    }.get(status, f"sufficiency status: {status}")
                    missing_txt = ("; missing: " + "; ".join(missing[:3])) if missing else ""
                    hard_txt = ("; hard gaps: " + ", ".join(str(x) for x in hard[:3])) if hard else ""
                    conf_txt = f"; agent confidence: {conf:.2f}" if isinstance(conf, (int, float)) else ""
                    fb_txt = f"; feedback: {feedback[:200]}" if feedback else ""
                    # Guardrail counter.
                    if status in insufficient_statuses:
                        self._consecutive_unanswerable += 1
                    else:
                        self._consecutive_unanswerable = 0
                    _GUA = 2  # consecutive-insufficient threshold
                    if self._consecutive_unanswerable >= _GUA:
                        final = f"{final}\n\n[Research status] {status_hint}{missing_txt}{hard_txt}{conf_txt}{fb_txt}. STOP calling rag again: {self._consecutive_unanswerable} consecutive research rounds returned insufficient evidence. The sources likely lack the required data. Give your best answer from the evidence already gathered; do not re-run rag."
                    else:
                        final = f"{final}\n\n[Research status] {status_hint}{missing_txt}{hard_txt}{conf_txt}{fb_txt}. If these gaps are material, call rag again with a question focused on them."
            call_stats.log()
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
        # peewee MySQL lookup — call directly to reuse the pool's connection.
        self._metas_cache = DocMetadataService.get_flatted_meta_by_kbs(self.kb_ids)
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
