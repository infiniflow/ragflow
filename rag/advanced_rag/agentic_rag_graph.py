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

"""Agentic-RAG orchestration as a LangGraph state machine.

The graph wraps the existing :class:`rag.advanced_rag.agentic_rag.RAGTools`
methods — it does NOT replace them. Each node calls one or more of those
``@tool`` methods; LangGraph handles the plan → act → observe → replan
loop and, crucially, *checkpoints state at every node boundary* so a
mid-run failure resumes from the last committed node instead of
re-running the whole procedure (per-turn resume, keyed by
``f"{conv_id}:{turn}"``).

Flow (mirrors the user's spec):

    START → plan
    plan ─▶ answer                       (intent == "answer" | iters exhausted)
         ─▶ select_docs                  (intent == "search_kb")
         ─▶ select_docs                  (intent == "compare")
         ─▶ web_search                   (intent == "web_search")
         ─▶ summarize                    (intent == "summarize")
         ─▶ structured                   (intent == "structured")

    select_docs → load_hints → retrieve → plan
    web_search / summarize / compare / structured → plan
    answer → END

Two orthogonal output channels reach the caller through one async queue:
  * ``{"status": <node>}`` frames — cheap step indicators, safe to replay.
  * ``{"answer": <token>}`` frames — streamed by ``answer`` node only.
    These live OUTSIDE checkpointed state, so a resume that re-enters the
    answer node simply re-streams; the retrieved evidence it reads comes
    from checkpointed ``kbinfos``, so no retrieval is repeated.

LangGraph / langgraph-checkpoint-redis are optional deps: this module is
only imported from the feature-flagged entry point, so the app still
starts without them.
"""

from __future__ import annotations

import asyncio
import json
import logging
import re
from copy import deepcopy
from typing import Any, AsyncIterator, Optional, TypedDict

import json_repair


# --------------------------------------------------------------------
# State
# --------------------------------------------------------------------


class AgentState(TypedDict, total=False):
    # Inputs (set once at START)
    messages: list[dict]          # [{role, content}, ...]
    max_iterations: int

    # Planner outputs
    formalized_question: str
    intent: str                   # answer|search_kb|web_search|summarize|compare|structured
    sub_questions: list[str]
    iteration: int

    # Retrieval progress
    selected_doc_ids: list[str]
    doc_compiled_hints: dict[str, str]     # doc_id → tree/graph outline
    dataset_hint: dict[str, str]           # {skill_outline, dataset_nav}
    # Cumulative citation pool snapshot copied from RAGTools.kbinfos so a
    # resume that re-enters ``answer`` can rebuild context without re-running
    # any retrieval node.
    kbinfos: dict[str, list]

    # Final
    final_answer: str

    # Diagnostics
    node_errors: list[dict]


# Intent → downstream node name. ``answer`` is terminal.
_INTENT_ROUTES = {
    "answer": "answer",
    "search_kb": "select_docs",
    "compare": "select_docs",
    "web_search": "web_search",
    "summarize": "summarize",
    "structured": "structured",
}
_VALID_INTENTS = set(_INTENT_ROUTES)


def _messages_to_transcript(messages: list[dict]) -> list[str]:
    """Render ``[{role, content}]`` into the ``["User: ...", ...]`` shape
    ``RAGTools.formalize_question`` expects."""
    out: list[str] = []
    for m in messages or []:
        role = (m.get("role") or "user").capitalize()
        content = m.get("content") or ""
        if content:
            out.append(f"{role}: {content}")
    return out


def _latest_user_text(messages: list[dict]) -> str:
    for m in reversed(messages or []):
        if (m.get("role") or "") == "user" and m.get("content"):
            return m["content"]
    return ""


def _parse_json_object(text: str) -> dict:
    """Best-effort JSON-object parse from an LLM answer (strips think tags
    and code fences)."""
    if isinstance(text, tuple):
        text = text[0]
    cleaned = re.sub(r"^.*</think>", "", text or "", flags=re.DOTALL)
    cleaned = re.sub(r"```(?:json)?\s*|\s*```", "", cleaned).strip()
    try:
        obj = json_repair.loads(cleaned)
    except Exception:
        return {}
    return obj if isinstance(obj, dict) else {}


# --------------------------------------------------------------------
# Graph builder
# --------------------------------------------------------------------


def build_graph(tools, checkpointer, token_queue: "asyncio.Queue"):
    """Compile the agentic-RAG graph.

    :param tools: a live ``RAGTools`` instance (its ``@tool`` methods are
        the node bodies; not serializable, so it is captured in the node
        closures rather than stored in graph state).
    :param checkpointer: a LangGraph checkpointer (Redis) for per-turn
        resume.
    :param token_queue: async queue the ``answer`` node streams tokens
        into. Lives outside checkpointed state.
    :returns: a compiled ``StateGraph``.
    """
    from langgraph.graph import StateGraph, START, END

    chat_mdl = tools.chat_mdl

    # ----- plan -----------------------------------------------------
    async def plan_node(state: AgentState) -> dict:
        messages = state.get("messages") or []
        iteration = int(state.get("iteration") or 0)
        max_iters = int(state.get("max_iterations") or 4)

        # Formalize once (iteration 0). ``formalize_question`` resolves
        # follow-up references against the full transcript (spec 1.1).
        formalized = state.get("formalized_question") or ""
        if not formalized:
            try:
                formalized = await tools.formalize_question(_messages_to_transcript(messages))
            except Exception:
                logging.exception("plan_node: formalize failed; using latest message")
                formalized = _latest_user_text(messages)
            formalized = formalized or _latest_user_text(messages)

        # Loop budget exhausted → force an answer from what we have.
        if iteration >= max_iters:
            return {"formalized_question": formalized, "intent": "answer",
                    "iteration": iteration + 1}

        have_evidence = bool((state.get("kbinfos") or {}).get("chunks"))
        planner_system = (
            "You are the planner of a retrieval agent. Decide the SINGLE next "
            "step for the question below. Choose ONE intent:\n"
            "- search_kb: retrieve chunks from the knowledge base\n"
            "- web_search: search the public web (only if KB won't have it)\n"
            "- summarize: the user explicitly asked to summarize one document\n"
            "- compare: the user asked to contrast/diff specific documents\n"
            "- structured: the question is an aggregate/filter over tabular data\n"
            "- answer: enough evidence already gathered — compose the answer\n\n"
            "If evidence has ALREADY been retrieved and it is sufficient, choose "
            "answer. Output ONLY a JSON object: "
            '{"intent": "...", "sub_questions": ["..."]}. '
            "sub_questions is optional; include it only when the question bundles "
            "several independent information needs."
        )
        planner_user = (
            f"Question: {formalized}\n"
            f"Evidence already retrieved: {'yes' if have_evidence else 'no'}\n"
            f"Iteration: {iteration + 1}/{max_iters}\n\n"
            "Next step (JSON):"
        )
        try:
            raw = await chat_mdl.async_chat(
                system=planner_system,
                history=[{"role": "user", "content": planner_user}],
                gen_conf={"temperature": 0.1},
            )
        except Exception:
            logging.exception("plan_node: planner LLM failed; defaulting to search_kb")
            raw = ""
        plan = _parse_json_object(raw)
        intent = str(plan.get("intent") or "").strip()
        if intent not in _VALID_INTENTS:
            # No evidence yet → search; otherwise answer.
            intent = "answer" if have_evidence else "search_kb"

        subs = plan.get("sub_questions")
        sub_questions = state.get("sub_questions") or []
        if isinstance(subs, list) and subs and not sub_questions:
            # Only decompose once; subsequent loops reuse the list.
            try:
                sub_questions = await tools.decompose_question(formalized)
            except Exception:
                sub_questions = [s for s in subs if isinstance(s, str)]

        return {
            "formalized_question": formalized,
            "intent": intent,
            "sub_questions": sub_questions,
            "iteration": iteration + 1,
        }

    def route_from_plan(state: AgentState) -> str:
        return _INTENT_ROUTES.get(state.get("intent") or "answer", "answer")

    # ----- select_docs ---------------------------------------------
    async def select_docs_node(state: AgentState) -> dict:
        question = state.get("formalized_question") or ""
        # Spec 4.1: if the KB has a skill tree or dataset nav, let the LLM
        # pick docs from that hierarchical/flat markdown. Otherwise 4.2:
        # fall back to title-based ``select_documents`` (doc_aggs-adjacent).
        from rag.advanced_rag import agentic_rag_hints as hints

        dataset_hint = state.get("dataset_hint") or {}
        if not dataset_hint:
            merged = {"skill_outline": "", "dataset_nav": ""}
            for tenant_id, kb in zip(tools.tenant_ids, tools.kbs):
                h = await hints.gather_dataset_hint(tenant_id, kb.id)
                merged["skill_outline"] = merged["skill_outline"] or h.get("skill_outline", "")
                merged["dataset_nav"] = merged["dataset_nav"] or h.get("dataset_nav", "")
            dataset_hint = merged

        selected: list[str] = []
        hint_md = dataset_hint.get("skill_outline") or dataset_hint.get("dataset_nav") or ""
        if hint_md:
            selected = await _select_docs_from_hint(question, hint_md)

        if not selected:
            try:
                picked = await tools.select_documents(question)
                if isinstance(picked, list):
                    selected = [d for d in picked if isinstance(d, str)]
            except Exception:
                logging.exception("select_docs_node: select_documents failed")

        return {"selected_doc_ids": selected, "dataset_hint": dataset_hint}

    async def _select_docs_from_hint(question: str, hint_md: str) -> list[str]:
        """Ask the LLM to pick doc ids from the dataset hint markdown.

        The nav/skill markdown carries ``**<doc_id>**`` bullets; we let the
        LLM return the ids it judges relevant, then defensively keep only
        ids that actually appear in the markdown.
        """
        system = (
            "You are given a knowledge base's document navigation outline. "
            "Pick the document IDs whose entries are relevant to the question. "
            "Use ONLY IDs that appear in the outline. Output ONLY a JSON array "
            "of IDs, e.g. [\"abc\",\"def\"]. If none are relevant, output []."
        )
        user = f"Question:\n{question}\n\nOutline:\n{hint_md}\n\nRelevant document IDs (JSON array):"
        try:
            raw = await chat_mdl.async_chat(
                system=system,
                history=[{"role": "user", "content": user}],
                gen_conf={"temperature": 0.1},
            )
        except Exception:
            logging.exception("_select_docs_from_hint: LLM failed")
            return []
        if isinstance(raw, tuple):
            raw = raw[0]
        cleaned = re.sub(r"^.*</think>", "", raw or "", flags=re.DOTALL)
        cleaned = re.sub(r"```(?:json)?\s*|\s*```", "", cleaned).strip()
        try:
            ids = json_repair.loads(cleaned)
        except Exception:
            return []
        if not isinstance(ids, list):
            return []
        # Keep only ids that literally occur in the outline text.
        return [d for d in ids if isinstance(d, str) and d and d in hint_md]

    # ----- load_hints ----------------------------------------------
    async def load_hints_node(state: AgentState) -> dict:
        from rag.advanced_rag.agentic_rag_hints import gather_doc_hint

        doc_ids = state.get("selected_doc_ids") or []
        hints: dict[str, str] = {}
        # Resolve each doc's tenant via the bound KBs.
        tenant_by_kb = {kb.id: tid for tid, kb in zip(tools.tenant_ids, tools.kbs)}
        for doc_id in doc_ids:
            pair = None
            try:
                from common.misc_utils import thread_pool_exec
                pair = await thread_pool_exec(tools._resolve_doc_tenant, doc_id)
            except Exception:
                pair = None
            if not pair:
                continue
            kb_id, tenant_id = pair
            md = await gather_doc_hint(tenant_id, kb_id, doc_id)
            if md:
                hints[doc_id] = md
        return {"doc_compiled_hints": hints}

    # ----- retrieve -------------------------------------------------
    async def retrieve_node(state: AgentState) -> dict:
        question = state.get("formalized_question") or ""
        scope = state.get("selected_doc_ids") or None
        # PR1: classic ES retrieval, doc-scoped when we selected docs. The
        # compiled-hint-driven chunk pick (spec 5.1) lands in PR2; the hints
        # are already carried in state so the answer node can surface them.
        keywords = question
        try:
            await tools.search_knowledge_bases(
                question=question, keywords=keywords, docid_scope=scope,
            )
        except Exception:
            logging.exception("retrieve_node: search_knowledge_bases failed")
        return {"kbinfos": deepcopy(tools.kbinfos)}

    # ----- web_search ----------------------------------------------
    async def web_search_node(state: AgentState) -> dict:
        question = state.get("formalized_question") or ""
        try:
            await tools.web_search(question)
        except Exception:
            logging.exception("web_search_node: web_search failed")
        return {"kbinfos": deepcopy(tools.kbinfos)}

    # ----- summarize -----------------------------------------------
    async def summarize_node(state: AgentState) -> dict:
        doc_ids = state.get("selected_doc_ids") or []
        if not doc_ids:
            # Try to pick the single doc the user meant.
            try:
                picked = await tools.select_documents(state.get("formalized_question") or "")
                if isinstance(picked, list):
                    doc_ids = [d for d in picked if isinstance(d, str)]
            except Exception:
                doc_ids = []
        if doc_ids:
            try:
                await tools.summarize_document(doc_ids[0])
            except Exception:
                logging.exception("summarize_node: summarize_document failed")
        return {"selected_doc_ids": doc_ids, "kbinfos": deepcopy(tools.kbinfos)}

    # ----- compare -------------------------------------------------
    async def compare_node(state: AgentState) -> dict:
        doc_ids = state.get("selected_doc_ids") or []
        if len(doc_ids) >= 2:
            try:
                await tools.compare_documents(doc_ids)
            except Exception:
                logging.exception("compare_node: compare_documents failed")
        return {"kbinfos": deepcopy(tools.kbinfos)}

    # ----- structured ----------------------------------------------
    async def structured_node(state: AgentState) -> dict:
        question = state.get("formalized_question") or ""
        try:
            await tools.search_structured_data(question)
        except Exception:
            logging.exception("structured_node: search_structured_data failed")
        return {"kbinfos": deepcopy(tools.kbinfos)}

    # ----- answer ---------------------------------------------------
    async def answer_node(state: AgentState) -> dict:
        from rag.prompts.generator import kb_prompt

        question = state.get("formalized_question") or _latest_user_text(state.get("messages") or [])
        kbinfos = state.get("kbinfos") or {"chunks": [], "doc_aggs": []}

        # Build the grounding context from the CHECKPOINTED evidence, not
        # from tools.kbinfos — on a resume the tools instance is fresh but
        # the state carries everything retrieved before the failure.
        try:
            context = kb_prompt(kbinfos, chat_mdl.max_length, 0)
        except Exception:
            context = []
        context_text = "\n\n".join(context) if isinstance(context, list) else str(context)

        citation_rules = ""
        try:
            citation_rules = tools.get_citation_guidelines()
        except Exception:
            pass

        system = (
            "You are a RAG assistant. Answer the question using ONLY the "
            "evidence below. Answer in the user's language. If the evidence "
            "is insufficient, say so plainly. Apply the citation rules "
            "verbatim.\n\n"
            f"# Citation rules\n{citation_rules}\n\n"
            f"# Evidence\n{context_text}"
        )
        user = f"Question: {question}"

        final = ""
        try:
            async for cumulative in chat_mdl.async_chat_streamly(
                system=system,
                history=[{"role": "user", "content": user}],
                gen_conf={"temperature": 0.3},
            ):
                if isinstance(cumulative, str):
                    # ``async_chat_streamly`` yields the cumulative answer;
                    # emit only the newly-appended delta.
                    delta = cumulative[len(final):]
                    final = cumulative
                    if delta:
                        token_queue.put_nowait({"answer": delta})
        except Exception:
            logging.exception("answer_node: streaming failed")
            if not final:
                final = "I couldn't compose an answer due to an internal error."
                token_queue.put_nowait({"answer": final})

        return {"final_answer": final}

    # ----- wire -----------------------------------------------------
    g = StateGraph(AgentState)
    g.add_node("plan", plan_node)
    g.add_node("select_docs", select_docs_node)
    g.add_node("load_hints", load_hints_node)
    g.add_node("retrieve", retrieve_node)
    g.add_node("web_search", web_search_node)
    g.add_node("summarize", summarize_node)
    g.add_node("compare", compare_node)
    g.add_node("structured", structured_node)
    g.add_node("answer", answer_node)

    g.add_edge(START, "plan")
    g.add_conditional_edges("plan", route_from_plan, _INTENT_ROUTES)

    # Retrieval sub-chain. Both ``search_kb`` and ``compare`` route through
    # select_docs → load_hints (to gather doc ids + compiled hints). After
    # hints load, branch on intent: ``compare`` goes to the compare tool
    # (needs 2+ docs side by side); everything else does ordinary chunk
    # retrieval. Both loop back to plan for the next decision.
    def route_after_hints(state: AgentState) -> str:
        return "compare" if (state.get("intent") == "compare") else "retrieve"

    g.add_edge("select_docs", "load_hints")
    g.add_conditional_edges(
        "load_hints", route_after_hints,
        {"compare": "compare", "retrieve": "retrieve"},
    )
    g.add_edge("retrieve", "plan")
    g.add_edge("web_search", "plan")
    g.add_edge("summarize", "plan")
    g.add_edge("compare", "plan")
    g.add_edge("structured", "plan")
    g.add_edge("answer", END)

    return g.compile(checkpointer=checkpointer)


# --------------------------------------------------------------------
# Entry point
# --------------------------------------------------------------------


async def run_agentic_rag(
    tools,
    messages: list[dict],
    thread_id: str,
    max_iterations: int = 4,
) -> AsyncIterator[dict]:
    """Drive the agentic-RAG graph, yielding SSE-ready frames.

    Yields dicts of the shape:
      * ``{"status": <node_name>}``  — step indicator
      * ``{"answer": <delta>}``      — streamed answer token(s)
      * ``{"error": <message>}``     — terminal error

    :param tools: a live ``RAGTools`` instance (already ``bind_tools``-ed).
    :param messages: conversation history ``[{role, content}, ...]``.
    :param thread_id: per-turn resume key (e.g. ``f"{conv_id}:{turn}"``).
    :param max_iterations: plan-loop budget.
    """
    from rag.advanced_rag.agentic_rag_checkpoint import open_checkpointer

    token_queue: asyncio.Queue = asyncio.Queue()

    async def _drive(graph, config, init_state):
        try:
            async for update in graph.astream(init_state, config, stream_mode="updates"):
                # ``update`` is {node_name: partial_state}. Emit a status
                # frame per committed node so the UI can render progress.
                if isinstance(update, dict):
                    for node_name in update.keys():
                        token_queue.put_nowait({"status": node_name})
        except Exception as e:
            logging.exception("run_agentic_rag: graph drive failed")
            token_queue.put_nowait({"error": str(e)})
        finally:
            token_queue.put_nowait(None)  # sentinel

    async with open_checkpointer() as checkpointer:
        graph = build_graph(tools, checkpointer, token_queue)
        config = {"configurable": {"thread_id": thread_id}}
        init_state: AgentState = {
            "messages": messages,
            "max_iterations": max_iterations,
            "iteration": 0,
            "kbinfos": {"chunks": [], "doc_aggs": []},
        }
        drive_task = asyncio.create_task(_drive(graph, config, init_state))
        try:
            while True:
                item = await token_queue.get()
                if item is None:
                    break
                yield item
        finally:
            if not drive_task.done():
                drive_task.cancel()
                try:
                    await drive_task
                except (asyncio.CancelledError, Exception):
                    pass
