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

import logging
import re
from typing import Any, List

import json_repair
from copy import deepcopy
from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from common import settings
from common.misc_utils import thread_pool_exec
from rag.app.tag import label_question
from rag.llm.tool_decorator import tool
from rag.prompts.generator import citation_prompt, gen_meta_filter, kb_prompt
from api.db.db_models import Document, Knowledgebase
from rag.utils.tavily_conn import Tavily
from common.token_utils import num_tokens_from_string


class RAGTools:
    def __init__(self, 
                 tenant_ids: list[str],
                 chat_mdl: LLMBundle, 
                 embed_mdl: LLMBundle | None = None, 
                 kb_ids: List[str] | None = None,
                 kbs: list[Knowledgebase] | None = [], 
                 tav: Tavily | None = None,
                 meta_data_filter: dict | None = None,
                 user_defined_prompts: dict | None = None,
                 ):
        self.tenant_ids = tenant_ids
        self.chat_mdl = deepcopy(chat_mdl)
        self.embed_mdl = embed_mdl
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
        # Accumulator for chunks/doc_aggs across tool calls within a turn —
        # populated by ``search_knowledge_bases`` and ``search_structured_data``
        # so the final answer can cite everything retrieved so far.
        self.kbinfos: dict[str, list] = {"chunks": [], "doc_aggs": []}
        # Set to True after the first retrieval tool has stamped the citation
        # rules onto its output, so subsequent retrieval calls don't repeat.
        self._citations_injected: bool = False

        tools = [
            self.formalize_question,
            self.decompose_question,
            self.select_documents,
            self.search_knowledge_bases,
        ]
        if self.tav:
            tools.append(self.web_search)
        if meta_data_filter:
            tools.append(self.filter_docs_by_metadata)
        if self.sql_kbs:
            tools.append(self.search_structured_data)
        if self.kb_ids:
            tools.append(self.summarize_document)
            tools.append(self.compare_documents)
        chat_mdl.bind_tools(None, tools)

    def sys_prompt(self) -> str:
        """Return the system instruction the chat model should be initialised with.

        The workflow encoded here mentions ONLY the tools that this
        ``RAGTools`` instance actually registered — optional tools
        (``filter_docs_by_metadata``, ``search_structured_data``,
        ``web_search``) are described only when they are bound, so the
        model is never told to call something it does not have.
        """
        has_meta = bool(self.meta_data_filter)
        has_sql = bool(self.sql_kbs)
        has_web = self.tav is not None
        has_unstructured = bool(self.kb_ids)
        has_embedding = self.embed_mdl is not None
        has_summarize = has_unstructured  # tool gated on kb_ids in __init__

        # Step 2 — document-scope narrowing bullets
        narrow_bullets = [
            "- Call `select_documents` when the question names or strongly "
            "implies a particular document or a small subset by title or topic."
        ]
        if has_meta:
            narrow_bullets.append(
                "- Call `filter_docs_by_metadata` when the question references "
                "structured attributes (year, author, department, product, ...) "
                "that are carried as document metadata."
            )

        # Step 3 — retrieval paragraph, depending on which KB shapes are bound
        summarize_special_case = (
            " SPECIAL CASE — summarisation: if the user EXPLICITLY asked you "
            "to summarise a specific document (phrasings like 'summarise the "
            "security audit', 'give me a summary of doc X', 'tldr the "
            "onboarding guide'), call `summarize_document` with the doc ID "
            "obtained in step 2 INSTEAD OF `search_knowledge_bases`. Use this "
            "tool ONLY for explicit summarisation requests — not for general "
            "Q&A about a document's contents, which still goes through "
            "`search_knowledge_bases`."
            if has_summarize
            else ""
        )

        embedding_retry = (
            " Inspect the chunks `search_knowledge_bases` returns. If they are "
            "not fully relevant — they only hit on incidental keyword overlap, "
            "or the keywords clearly miss the concept the user is after — call "
            "`search_knowledge_bases` ONCE MORE with the SAME `question` and "
            "`keywords` but with `using_embedding=True`. Do this at most one "
            "extra time per turn; if neither keyword nor embedding mode finds "
            "relevant content, the KB likely does not cover this question."
            if has_embedding
            else ""
        )

        if has_unstructured and has_sql:
            retrieval_para = (
                "If the question is naturally an aggregate or filter over tabular "
                "data ('how many ...', 'list the top N by ...', 'sum of ... grouped "
                "by ...'), call `search_structured_data` with the formalized "
                "question — the structured knowledge base has a typed schema and "
                "the tool translates your question into SQL. Otherwise call "
                "`search_knowledge_bases` with the formalized question AND a short "
                "keyword string (3-8 keywords plus 1-2 close synonyms for ambiguous "
                "terms, in the same language as the question). Pass any doc IDs "
                "collected in step 2 as `docid_scope`." + embedding_retry + summarize_special_case
            )
        elif has_sql and not has_unstructured:
            retrieval_para = (
                "Call `search_structured_data` with the formalized question. The "
                "knowledge base has a typed schema (`field_map`), so the tool will "
                "translate your question into SQL and execute it."
            )
        else:
            retrieval_para = (
                "Call `search_knowledge_bases` with the formalized question AND a "
                "short keyword string (3-8 keywords plus 1-2 close synonyms for "
                "ambiguous terms, in the same language as the question). Pass any "
                "doc IDs collected in step 2 as `docid_scope`." + embedding_retry
            )

        steps: list[str] = []
        steps.append(
            "**Formalize the question.** If the latest user message is a "
            "follow-up that depends on earlier turns (pronouns, 'and X?', "
            "'what about ...'), call `formalize_question` to produce a "
            "self-contained question and use that for every subsequent step. "
            "Otherwise use the latest user message as-is."
        )
        steps.append(
            "**Narrow the document scope.** Before retrieving, try to limit "
            "which documents you'll search:\n   "
            + "\n   ".join(narrow_bullets)
            + "\n   Collect the doc IDs these tools return VERBATIM and pass "
            "them to the next step as `docid_scope`. You DO NOT know any doc "
            "IDs on your own — they are 32-character hex strings (e.g. "
            "`41a5271858ca11f1bbb9047c16ec874f`) that only these tools can "
            "produce. Skip this step entirely when the question gives you no "
            "signal to narrow down, and in that case pass `null` (NOT an "
            "invented list) for `docid_scope` in the next step."
        )
        steps.append("**Retrieve evidence from the knowledge bases.** " + retrieval_para)
        if has_web:
            steps.append(
                "**Fall back to web search.** If `search_knowledge_bases` "
                "returned no relevant chunks AND the question is about "
                "generally public information, call `web_search`. Skip this "
                "step whenever the KB retrieval succeeded — prefer "
                "KB-grounded answers."
            )
        steps.append(
            "**Compose the final answer with citations.** Citation rules will "
            "be delivered to you inline with the FIRST retrieval result of "
            "this turn (look for a `# Citation rules` block at the top of the "
            "tool output). Apply those rules VERBATIM. Do NOT invent your own "
            "citation style, and NEVER cite a source you did not actually "
            "retrieve in this turn."
        )

        numbered = "\n\n".join(f"{i}. {step}" for i, step in enumerate(steps, 1))

        return (
            "You are a Retrieval-Augmented-Generation (RAG) agent. Answer the "
            "user's question using ONLY evidence you retrieved through the "
            "tools available to you. Do not invent facts: if the evidence "
            "cannot support a claim, say so plainly instead of guessing.\n\n"
            "# Workflow\n\n"
            "Work through the following steps in order. Skip a step when it "
            "is obviously inapplicable.\n\n"
            f"{numbered}\n\n"
            "# Hard rules\n\n"
            "- DO NOT make anything up. If the retrieved evidence does not "
            "answer the question, reply with an explicit \"I don't have "
            "enough information based on the available sources\" (in the "
            "user's language).\n"
            "- DO NOT cite sources that were not returned by your tool calls "
            "in this turn.\n"
            "- DO NOT invent identifiers. Every doc ID you pass to a tool "
            "MUST be a value some other tool returned earlier IN THIS SAME "
            "TURN. If you have no IDs from a prior tool, pass `null` — never "
            "a fabricated 32-character string.\n"
            "- **Answer in the user's language.** The prose of the final "
            "answer MUST be in the SAME language as the user's question. If "
            "the user wrote in Chinese, you answer in Chinese; if Japanese, "
            "Japanese; and so on for every other non-English language. "
            "Answering in English when the user did NOT write in English is "
            "FORBIDDEN — translate retrieved evidence into the user's "
            "language as part of composing the answer. The single exception "
            "is verbatim quoted snippets from the knowledge base that you "
            "cite as evidence: those may stay in the source's original "
            "language so the citation remains faithful. Everything OUTSIDE "
            "those quoted snippets — your prose, your headings, your "
            "summaries, the \"I don't have enough information\" fallback — "
            "must be in the user's language."
        )

    @tool
    async def formalize_question(self, messages: List[str]) -> str:
        """Rewrite the latest user message if it's not suitable for searching into a complete, standalone question
        by resolving pronouns and elliptical references against earlier turns.

        Args:
            messages: the conversation so far, oldest first. Each item should be
                prefixed with the speaker, e.g.
                  ["User: what's the population of Beijing?",
                   "Assistant: About 50 million.",
                   "User: New york?"]

        Returns:
            A single self-contained question, e.g.
              "What's the population of New York?"
            If the latest user message is already standalone, it's returned unchanged.
        """
        if not messages:
            return ""

        transcript = "\n".join(messages)
        system = (
            "You rewrite the LAST user message into a single, self-contained question "
            "that can be understood without seeing the prior conversation. "
            "Resolve pronouns, ellipses, and follow-up shortcuts using earlier turns. "
            "Preserve the original language of the last user message. "
            "Output ONLY the rewritten question — no preamble, no quotes, no explanation. "
            "If the last user message is already a complete standalone question, return it unchanged."
        )
        user = (
            f"Conversation:\n{transcript}\n\n"
            "Rewritten standalone question:"
        )

        ans = await self.chat_mdl.async_chat(
                system=system,
                history=[{"role": "user", "content": user}],
                gen_conf={"temperature": 0.1},
            )

        return ans.strip().strip('"').strip("'")

    @tool(timeout=60)
    async def decompose_question(self, question: str) -> List[str]:
        """Break a complex, multi-part question into independent sub-questions.

        Use this ONLY when the (already formalized) question bundles several
        distinct information needs that would each be answered by a separate
        retrieval — e.g. "compare the 2023 and 2024 revenue AND explain the
        drop in Q3", or "what are the onboarding steps and how do they differ
        from the enterprise flow". Each sub-question should be self-contained
        and independently searchable.

        Do NOT decompose a question that is already a single information need
        ("what is the refund policy?") — return it unchanged as a one-element
        list. Over-decomposing wastes retrieval round-trips.

        :param question: the self-contained question to decompose (run
            ``formalize_question`` first if the latest user message was a
            follow-up).

        :returns: a JSON list of sub-questions in the SAME language as the
            input. A single-need question yields ``[question]`` unchanged.
        """
        if not question or not question.strip():
            return []

        system = (
            "You split a user's question into the minimal set of independent, "
            "self-contained sub-questions needed to answer it fully. "
            "Preserve the original language. If the question is already a "
            "single information need, return it unchanged as the only element. "
            "Output ONLY a JSON array of strings — no prose, no code fences."
        )
        user = f"Question:\n{question}\n\nSub-questions (JSON array):"

        ans = await self.chat_mdl.async_chat(
            system=system,
            history=[{"role": "user", "content": user}],
            gen_conf={"temperature": 0.1},
        )
        if isinstance(ans, tuple):
            ans = ans[0]
        cleaned = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
        cleaned = re.sub(r"```(?:json)?\s*|\s*```", "", cleaned).strip()
        try:
            parts = json_repair.loads(cleaned)
        except Exception as e:
            logging.warning(f"decompose_question could not parse LLM output: {e!r} raw={ans[:200]!r}")
            return [question]
        if not isinstance(parts, list):
            return [question]
        subs = [p.strip() for p in parts if isinstance(p, str) and p.strip()]
        return subs or [question]

    async def _get_cached_metas(self) -> dict:
        """Lazy-load the flattened metadata map for the bound KBs and cache it
        on the instance so repeat tool calls in the same session don't re-hit
        the DB. Returns an empty dict when no KBs are bound.
        """
        cached = getattr(self, "_metas_cache", None)
        if cached is not None:
            return cached
        if not self.kb_ids:
            self._metas_cache = {}
            return self._metas_cache
        self._metas_cache = await thread_pool_exec(
            DocMetadataService.get_flatted_meta_by_kbs, self.kb_ids
        )
        return self._metas_cache or {}

    @tool(timeout=60)
    async def filter_docs_by_metadata(self, question: str) -> List[str]:
        """Narrow the search to a smaller document set using structured metadata.

        If the bound knowledge bases carry document-level metadata (e.g.
        ``author``, ``year``, ``department``, ``product``), this tool asks an
        LLM to translate the question into a metadata filter and runs it
        against the index, returning the matching document IDs.

        Call this BEFORE ``search_knowledge_bases`` whenever the user's
        question references such structured attributes ("documents from 2024",
        "papers by Alice on X"), then pass the returned IDs to
        ``search_knowledge_bases`` via its ``attachments`` parameter so
        retrieval is restricted to those docs.

        Skip this tool when the question is pure free-text with no obvious
        metadata predicate — running it then wastes an LLM call and may
        produce an over-restrictive filter.

        :param question: the self-contained natural-language question.

        :returns: list of document IDs matching the metadata filter. An
            empty list means one of: no metadata is defined on the KBs, no
            filter could be generated from the question, no docs matched,
            or the filter couldn't be pushed down to the index. In any of
            those cases the caller should fall back to unfiltered retrieval
            (i.e. call ``search_knowledge_bases`` without ``attachments``).
        """
        if not self.kb_ids:
            return []

        metas = await self._get_cached_metas()
        if not metas:
            return []

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
        logging.debug(f"Doc ids filtered by metadata: {doc_ids}")
        # ``filter_doc_ids_by_meta_pushdown`` returns None when push-down isn't
        # viable; treat that as "no filter applied" and surface as empty list
        # so the caller falls back to unfiltered retrieval.
        return doc_ids or []

    def _collect_doc_titles(self, max_docs:int = 512) -> list[tuple[str, str]]:
        """Return ``[(doc_id, name)]`` for every document in the bound KBs.

        Lightweight sync DB read — meant to be wrapped in ``thread_pool_exec``
        by the tool entry-point so the event loop isn't blocked.
        """
        result: list[tuple[str, str]] = []
        for kb_id in self.kb_ids:
            for doc in DocumentService.query(kb_id=kb_id):
                result.append((doc.id, doc.name))
                if len(result) >= max_docs:
                    return None
        return result

    def _with_citation_guidelines(self, output: Any) -> Any:
        """Stamp the citation rules onto the FIRST retrieval-tool output of the
        turn, then short-circuit on subsequent calls.

        The citation policy is static and applies to every final answer, but
        the model only needs to see it once — and only when retrieval has
        actually happened (otherwise there's nothing to cite). Injecting
        inline with the tool result keeps the system prompt small and avoids
        a separate tool round-trip that the model would routinely skip.

        Accepts either ``str`` or ``list[str]`` (the two shapes the retrieval
        tools currently return) and prepends a ``# Citation rules`` block.
        """
        if self._citations_injected:
            return output
        self._citations_injected = True
        rules = citation_prompt(self.user_defined_prompts).strip()
        header = (
            "# Citation rules\n"
            "Apply the following rules VERBATIM to your final answer. "
            "They are stated here in full and apply for the rest of this "
            "turn.\n\n"
            f"{rules}\n\n"
            "----\n\n"
        )
        if isinstance(output, list):
            return [header] + output
        return header + str(output)

    def _filter_known_doc_ids(self, candidate_ids: list[str]) -> set[str]:
        """Return the subset of ``candidate_ids`` that actually belong to a
        bound unstructured KB.

        Single targeted ``WHERE id IN (...) AND kb_id IN (...)`` query —
        size bounded by ``len(candidate_ids)`` (which is at most the
        LLM-supplied ``docid_scope``), not by the KB size. Used to catch
        hallucinated 32-char hex IDs before they reach the retriever, which
        would silently return zero chunks for unknown IDs.

        Sync DB call — wrap in ``thread_pool_exec`` at the call site so the
        event loop isn't blocked.
        """
        if not candidate_ids or not self.kb_ids:
            return set()
        rows = Document.select(Document.id).where(
            (Document.id.in_(list(candidate_ids)))
            & (Document.kb_id.in_(self.kb_ids))
        )
        return {row.id for row in rows}

    @tool(timeout=60)
    async def select_documents(self, question: str, max_docs:int=512) -> List[str]:
        """Ask an LLM to pick the document IDs whose titles look relevant to the question.

        Every document in the bound knowledge bases is listed to the LLM in
        the format ``docID: <id>, title: <title>`` and the LLM returns a JSON
        array of the IDs it considers relevant.

        Use this BEFORE ``search_knowledge_bases`` when the user's question
        names a specific document or refers to a small subset by topic
        ("summarize the security audit", "what does the 2024 onboarding guide
        say about X"). Pass the returned IDs to ``search_knowledge_bases`` via
        its ``docid_scope`` parameter so retrieval is restricted to those
        documents.

        Skip this tool for broad free-form questions — running it then wastes
        an LLM round-trip on a list the model would mostly discard.

        :param question: the self-contained natural-language question.

        :returns: list of document IDs the LLM judged relevant, or an empty
            list when no KBs are bound, no documents exist, or none look
            relevant title. IDs returned by the LLM that are not in the catalogue
            are filtered out defensively.
        """
        if not self.kb_ids:
            return []

        docs = await thread_pool_exec(self._collect_doc_titles)
        if docs is None:
            return "Too much documents for LLM to judge."
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
        user = (
            f"Question:\n{question}\n\n"
            f"Documents:\n{catalogue}\n\n"
            "Relevant docIDs (JSON array):"
        )

        ans = await self.chat_mdl.async_chat(
            system=system,
            history=[{"role": "user", "content": user}],
            gen_conf={"temperature": 0.1},
        )
        if isinstance(ans, tuple):
            ans = ans[0]

        # Strip <think> reasoning prefixes and ```json fences before parsing.
        cleaned = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
        cleaned = re.sub(r"```(?:json)?\s*|\s*```", "", cleaned).strip()
        try:
            ids = json_repair.loads(cleaned)
        except Exception as e:
            logging.warning(f"select_documents could not parse LLM output: {e!r} raw={ans[:200]!r}")
            return []
        if not isinstance(ids, list):
            return []

        # Defensive: drop anything the LLM hallucinated outside our catalogue.
        known = {doc_id for doc_id, _ in docs}
        res = [doc_id for doc_id in ids if isinstance(doc_id, str) and doc_id in known]
        if not res:
            return "Fail to pick the document IDs. Try other methods."
        return res

    @tool
    async def search_knowledge_bases(
        self,
        question: str,
        keywords: str,
        top_n: int = 6,
        similarity_threshold: float = 0.2,
        docid_scope: List[str] | None = None,
        using_embedding : bool = False
    ) -> dict[str, Any]:
        """Search the user's knowledge bases for chunks relevant to a question.

        You (the calling LLM) must supply BOTH the natural-language question
        and the keyword string used for retrieval — the tool does NOT run a
        second LLM pass to extract keywords.

        Two retrieval modes, controlled by ``using_embedding``:

        - **Keyword (default)**: pure sparse / BM25-style search driven by
          the ``keywords`` argument. Fast, cheap, exact-term match. Result
          quality is bounded by the keywords you provide.
        - **Embedding**: dense semantic retrieval over the full question
          (the ``keywords`` argument is ignored in this mode). Better at
          catching paraphrases and concepts the keywords miss, at the cost
          of an extra embedding pass.

        Recommended call pattern: call ONCE with ``using_embedding=False``.
        Inspect the returned chunks. If they are not fully relevant — e.g.
        they only hit on incidental keyword overlap, or the keywords clearly
        miss the concept the user is after — call AGAIN with
        ``using_embedding=True`` (same question, same keywords). Do NOT
        retry more than twice; if neither mode finds relevant content, the
        KB likely does not cover this question and you should consider
        ``web_search`` (when available) or admit you cannot answer.

        Prefer this tool for grounded answers; only fall back to
        ``web_search`` when both retrieval modes return nothing useful.

        :param question: the self-contained natural-language question (run
            ``formalize_question`` first if the latest user message is a
            follow-up that depends on earlier turns).
        :param keywords: 3-8 of the most important content terms from the
            question, plus 1-2 close synonyms or alternative phrasings for
            any ambiguous or polysemous term. Single words or short noun
            phrases, space-separated. The language MUST match the language of
            ``question``. If you cannot produce better keywords, repeat the
            question verbatim. Ignored when ``using_embedding=True``.
        :param top_n: maximum number of chunks to return (default 6).
        :param similarity_threshold: minimum similarity score for a chunk
            to be returned (default 0.2).
        :param docid_scope: OPTIONAL list of document IDs. Each ID is a
            32-character lowercase hex string such as
            ``41a5271858ca11f1bbb9047c16ec874f``. You DO NOT know any doc IDs
            on your own and you MUST NOT invent, guess, modify, or
            reconstruct one — not even if a 32-character string in your
            context happens to look like a doc ID. The ONLY acceptable
            sources for values here are doc IDs returned VERBATIM from a
            previous call to ``select_documents`` or
            ``filter_docs_by_metadata`` IN THIS SAME TURN. If neither tool
            has returned any IDs yet, pass null (the default) — that
            searches across all documents in the bound knowledge bases.
            Passing an invented ID will silently return zero chunks.
        :param using_embedding: set to ``True`` to switch from keyword
            search to dense embedding search. Default ``False``. Use this
            ONLY on a follow-up call when the previous keyword-only call
            returned chunks that were not fully relevant — not on the first
            attempt. Has no effect when this agent was constructed without
            an embedding model.

        :returns: a ``SearchResult``-shaped dict with the matched chunks,
            ``doc_aggs``, and the keywords actually used for retrieval. An
            empty result is returned when no knowledge bases are bound.
        """
        if not self.kb_ids:
            return {"total": 0, "chunks": [], "doc_aggs": {}}

        # Validate docid_scope against the actual catalogue — models often
        # hallucinate 32-char hex strings, and the retriever silently returns
        # zero chunks for unknown IDs (which then looks like "KB has nothing"
        # to the calling LLM, when really the filter was bogus).
        if docid_scope:
            candidates = [d for d in docid_scope if isinstance(d, str)]
            known = await thread_pool_exec(self._filter_known_doc_ids, candidates)
            valid = [d for d in candidates if d in known]
            if len(valid) != len(docid_scope):
                dropped = [d for d in docid_scope if d not in known]
                logging.warning(
                    f"search_knowledge_bases: dropping {len(dropped)}/{len(docid_scope)} "
                    f"unknown doc IDs from docid_scope (samples: {dropped[:3]})"
                )
            if valid:
                docid_scope = valid
            else:
                # Every supplied ID was bogus. Falling back to unfiltered
                # retrieval is safer than returning zero chunks and forcing
                # the LLM into a retry loop.
                logging.warning(
                    "search_knowledge_bases: every supplied doc ID was unknown; "
                    "falling back to unfiltered retrieval"
                )
                docid_scope = None

        search_terms = keywords.strip() if keywords else ""
        if not search_terms or using_embedding:
            search_terms = question

        embd_mdl = self.embed_mdl if using_embedding else None
        # Vector contributes to ranking only when an embedding model is actually
        # in play. With ``embd_mdl=None`` we MUST keep weight at 0 — otherwise
        # the retriever silently falls back to whatever embedding it can find.
        vector_weight = 0.7 if embd_mdl else 0
        kbinfos = await settings.retriever.retrieval(
            search_terms,
            embd_mdl,
            self.tenant_ids,
            self.kb_ids,
            1,
            top_n,
            similarity_threshold,
            vector_similarity_weight=vector_weight,
            aggs=True,
            doc_ids=docid_scope,
            rank_feature=label_question(question, self.kbs),
        )
        kbinfos["chunks"] = settings.retriever.retrieval_by_children(kbinfos["chunks"], self.tenant_ids)
        start_idx = len(self.kbinfos.get("chunks", []))
        if kbinfos:
            self.kbinfos["chunks"].extend(kbinfos.get("chunks", []))
            self.kbinfos["doc_aggs"].extend(kbinfos.get("doc_aggs", []))
        return self._with_citation_guidelines(
            kb_prompt(self.kbinfos, self.chat_mdl.max_length, start_idx)
        )

    def _resolve_doc_tenant(self, doc_id: str) -> tuple[str, str] | None:
        """Return ``(kb_id, tenant_id)`` for ``doc_id`` if and only if the
        document belongs to one of the agent's bound unstructured KBs.

        Returns ``None`` otherwise — used by ``summarize_document`` both as
        a hallucination guard against fabricated 32-char hex IDs and as the
        tenant-resolution step needed to query the doc store.

        Sync DB call — wrap in ``thread_pool_exec`` at the call site.
        """
        rows = list(
            Document.select(Document.kb_id).where(
                (Document.id == doc_id) & (Document.kb_id.in_(self.kb_ids))
            )
        )
        if not rows:
            return None
        kb_id = rows[0].kb_id
        for kb in self.kbs:
            if kb.id == kb_id:
                return kb_id, kb.tenant_id
        return None

    @tool
    async def summarize_document(self, doc_id: str) -> list[str]:
        """Return a single document's content, position-ordered, ready to be summarised.

        Call this tool ONLY when the user EXPLICITLY asks for a summary of a
        specific document — phrasings like "summarize the security audit",
        "give me a summary of doc X", "tldr the onboarding guide". Do NOT
        call it for general Q&A: use ``search_knowledge_bases`` for that.

        The tool fetches every chunk of the named document from the doc
        store, sorted by page / position so reading order is preserved, and
        formats them with ``kb_prompt`` so the result already respects the
        chat model's context-length budget (chunks past the budget are
        dropped with a warning). The output is the full chunk-formatted
        text that you, the calling LLM, should then turn into a natural-
        language summary in the user's language — applying the citation
        rules from the system prompt to attribute claims to chunk IDs.

        :param doc_id: a 32-character lowercase hex string (e.g.
            ``41a5271858ca11f1bbb9047c16ec874f``). You DO NOT know any doc
            IDs on your own and you MUST NOT invent one. Acceptable sources
            are doc IDs returned VERBATIM from a previous
            ``select_documents`` or ``filter_docs_by_metadata`` call in
            this same turn — typically you will call ``select_documents``
            first to map the user's spoken document title to an ID.

        :returns: a list of formatted chunk blocks (one per chunk, in
            document order, each carrying its ID / title / content) that
            collectively fit within the chat model's context budget. An
            empty list is returned when the doc ID is unknown to the bound
            KBs or the document has no chunks indexed.
        """
        if not self.kb_ids:
            return []

        resolved = await thread_pool_exec(self._resolve_doc_tenant, doc_id)
        if resolved is None:
            logging.warning(
                f"summarize_document: doc_id {doc_id!r} is not in any bound "
                "knowledge base — refusing to fetch (likely an LLM hallucination)"
            )
            return []
        kb_id, tenant_id = resolved

        cks = []
        tokens = 0
        for offset in range(0, 10000, 128):
            chunks = await thread_pool_exec(
                settings.retriever.chunk_list,
                doc_id,
                tenant_id,
                [kb_id],
                max_count=offset+128,
                offset=offset,
                fields=["content_with_weight", "docnm_kwd", "doc_id"],
                sort_by_position=True,
                retrieve_all=False,
            )
            for ck in chunks:
                num = num_tokens_from_string(str(ck["content_with_weight"]))
                if tokens + num > self.chat_mdl.max_length:
                    break
                tokens += num
                cks.append(ck)

        if not cks:
            return []
        doc_name = next(
            (c.get("docnm_kwd") or "" for c in cks if c.get("docnm_kwd")),
            "",
        )
        kbinfos = {
            "chunks": cks,
            "doc_aggs": [
                {
                    "doc_name": doc_name,
                    "doc_id": doc_id,
                    "count": len(cks),
                }
            ],
        }
        start_idx = len(self.kbinfos.get("chunks", []))
        if kbinfos:
            self.kbinfos["chunks"].extend(kbinfos.get("chunks", []))
            self.kbinfos["doc_aggs"].extend(kbinfos.get("doc_aggs", []))
        return self._with_citation_guidelines(
            kb_prompt(self.kbinfos, self.chat_mdl.max_length, start_idx)
        )

    @tool(timeout=120)
    async def compare_documents(self, doc_ids: List[str], focus: str = "") -> list[str]:
        """Load two or more documents side by side so their contents can be compared.

        Call this when the user asks to CONTRAST or find DIFFERENCES between
        specific documents — phrasings like "compare the 2023 and 2024 audit",
        "what changed between the v1 and v2 spec", "how does doc A differ from
        doc B". You (the calling LLM) then produce the comparison prose from
        the returned content, applying the citation rules.

        Each doc is fetched position-ordered (same as ``summarize_document``)
        and the per-doc blocks are labelled so you can attribute each claim to
        the right document. The combined content is trimmed to the chat
        model's context budget, split proportionally across the documents so
        no single doc starves the others.

        :param doc_ids: 2+ document IDs to compare. Each is a 32-character
            lowercase hex string. You MUST NOT invent IDs — acceptable values
            are those returned VERBATIM from ``select_documents`` /
            ``filter_docs_by_metadata`` IN THIS SAME TURN.
        :param focus: OPTIONAL free-text hint about which dimension to compare
            (e.g. "pricing", "security posture"). Passed through to you as
            context; the tool itself does not filter on it.

        :returns: a list of formatted, per-document labelled chunk blocks that
            collectively fit the context budget. An empty list is returned
            when fewer than two of the IDs resolve to bound documents.
        """
        if not self.kb_ids:
            return []
        ids = [d for d in (doc_ids or []) if isinstance(d, str) and d]
        # Resolve + dedup while preserving order; drop hallucinated ids.
        resolved: list[tuple[str, str, str]] = []
        seen: set[str] = set()
        for doc_id in ids:
            if doc_id in seen:
                continue
            seen.add(doc_id)
            pair = await thread_pool_exec(self._resolve_doc_tenant, doc_id)
            if pair is not None:
                resolved.append((doc_id, pair[0], pair[1]))
        if len(resolved) < 2:
            logging.warning(
                "compare_documents: fewer than two resolvable doc ids (%s) — "
                "refusing (likely hallucinated ids or single-doc request)",
                ids,
            )
            return []

        # Split the context budget evenly across the docs so one large doc
        # can't crowd out the others. Reserve a little headroom for the
        # per-doc labels + the citation header.
        per_doc_budget = max(512, int(self.chat_mdl.max_length * 0.9) // len(resolved))
        start_idx = len(self.kbinfos.get("chunks", []))
        blocks: list[str] = []
        if focus.strip():
            blocks.append(f"# Comparison focus\n{focus.strip()}\n\n----\n")
        for doc_id, kb_id, tenant_id in resolved:
            cks = []
            tokens = 0
            for offset in range(0, 10000, 128):
                chunks = await thread_pool_exec(
                    settings.retriever.chunk_list,
                    doc_id, tenant_id, [kb_id],
                    max_count=offset + 128, offset=offset,
                    fields=["content_with_weight", "docnm_kwd", "doc_id"],
                    sort_by_position=True, retrieve_all=False,
                )
                if not chunks:
                    break
                stop = False
                for ck in chunks:
                    num = num_tokens_from_string(str(ck["content_with_weight"]))
                    if tokens + num > per_doc_budget:
                        stop = True
                        break
                    tokens += num
                    cks.append(ck)
                if stop or len(chunks) < 128:
                    break
            if not cks:
                continue
            doc_name = next((c.get("docnm_kwd") or "" for c in cks if c.get("docnm_kwd")), doc_id)
            self.kbinfos["chunks"].extend(cks)
            self.kbinfos["doc_aggs"].append(
                {"doc_name": doc_name, "doc_id": doc_id, "count": len(cks)}
            )
            blocks.append(f"## Document: {doc_name} (id={doc_id})")
        # Render the full accumulated chunk pool once, labelled per-doc above.
        rendered = kb_prompt(self.kbinfos, self.chat_mdl.max_length, start_idx)
        if isinstance(rendered, list):
            return self._with_citation_guidelines(blocks + rendered)
        return self._with_citation_guidelines(blocks + [rendered])

    @tool(timeout=60)
    async def search_structured_data(self, question: str) -> str:
        """Query the structured (tabular) knowledge bases by translating the
        question into SQL and executing it.

        This tool is only registered when at least one bound knowledge base
        carries a ``field_map`` (i.e. its documents are spreadsheets / tables
        with a typed schema). It asks an LLM to generate SQL against that
        schema, executes it via the document engine (Elasticsearch / Infinity
        / OceanBase), and returns the formatted answer with citation markers.

        Use this tool when the question is naturally answered by an aggregate
        or filter over tabular data ("how many orders in 2024?", "list the
        top-5 vendors by spend"). For free-text questions, prefer
        ``search_knowledge_bases``.

        Matching chunks and doc aggregations are appended to the running
        ``self.kbinfos`` accumulator, so any subsequent retrieval tool can
        share the citation pool.

        :param question: the self-contained natural-language question (run
            ``formalize_question`` first if the latest user message is a
            follow-up that depends on earlier turns).

        :returns: the natural-language answer produced from the SQL result,
            already including the citation markers required by the citation
            rules in the system prompt. An empty string is returned when no
            structured KB is bound or SQL generation/execution fails.
        """
        if not self.sql_kbs or not self.field_map:
            return ""

        # Imported lazily to avoid a circular import:
        # dialog_service constructs ``RAGTools``.
        from api.db.services.dialog_service import use_sql

        sql_kb_ids = [kb.id for kb in self.sql_kbs]
        tenant_id = self.sql_kbs[0].tenant_id
        try:
            ans = await use_sql(
                question,
                self.field_map,
                tenant_id,
                self.chat_mdl,
                quota=True,
                kb_ids=sql_kb_ids,
            )
        except Exception as e:
            logging.exception(f"search_structured_data: use_sql failed: {e}")
            return ""

        if not ans:
            return ""

        reference = ans.get("reference") or {}
        new_chunks = reference.get("chunks") or []
        new_doc_aggs = reference.get("doc_aggs") or []
        if new_chunks:
            self.kbinfos["chunks"].extend(new_chunks)
        if new_doc_aggs:
            self.kbinfos["doc_aggs"].extend(new_doc_aggs)

        return self._with_citation_guidelines(ans.get("answer", "") or "")

    @tool
    async def web_search(self, query: str) -> List[dict[str, Any]]:
        """Search the public web for information not available in the knowledge base.

        Use this tool ONLY as a fallback when the knowledge-base retrieval
        tool returned no relevant chunks for the user's question. Prefer
        KB-grounded answers whenever the KB has the information.

        :param query: a self-contained natural-language search query
            (resolve pronouns / follow-up references before calling)

        :returns: a list of search results, each shaped as
            ``{"url": str, "title": str, "content": str, "score": float}``,
            or an empty list when web search is not configured or fails.
        """
        if self.tav is None:
            return []
        tav_res = await thread_pool_exec(self.tav.retrieve_chunks, query)
        start_idx = len(self.kbinfos.get("chunks", []))
        self.kbinfos["chunks"].extend(tav_res["chunks"])
        self.kbinfos["doc_aggs"].extend(tav_res["doc_aggs"])
        return self._with_citation_guidelines(
            kb_prompt(self.kbinfos, self.chat_mdl.max_length, start_idx)
        )

    def get_citation_guidelines(self) -> str:
        """Return the citation guidelines this agent uses.

        Plain method (NOT registered as a tool): the guidelines are static
        and are embedded directly in ``sys_prompt()`` so the chat model sees
        them from token zero. Letting the model decide whether to fetch them
        via a tool call was unreliable — the call was routinely skipped.
        Kept as a public helper so callers can introspect / override the
        text from outside the class.
        """
        return citation_prompt(self.user_defined_prompts)
