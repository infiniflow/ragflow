#
#  Copyright 2026 InfiniFlow, Inc. All Rights Reserved.
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

"""Slot-table research primitives + the graph-edge action session (方案 B).

This module is the single home of the unified react/tree architecture after the
``tree/`` package was removed:

* ``Variable`` / ``State`` (slot table) / ``Result`` (session outcome) — the
  shared intermediate representation.
* ``run_action_session`` — ONE DeepSearch ``run_action`` session as a LangGraph
  graph-edge loop (RUN_ACTION -> TOOL -> RUN_ACTION), backed by a whitelisted
  tool map (retrieve / search_chunks / list_chunks).
* ``initialize_state`` — LLM decomposition of a question into a typed slot
  table, used by the outer graph's planner after formalize.

Graph structure (per session)
-----------------------------
    START -> run_action
    run_action -(tool_calls)-> tool -(done)-> run_action
    run_action -(terminal/<state>|<answer>)-> END
    run_action -(attempts>=budget)-> finalize -> END

Provider agnosticism without touching chat_model.py:
* OpenAI-style wrappers expose ``async_client`` → native create();
* LiteLLMBase → reuse its own public ``_construct_completion_args`` and call
  ``litellm.acompletion`` with our schema injected.
"""

from __future__ import annotations

import asyncio
import json
import logging
import re
import secrets
import time
from dataclasses import dataclass, field
from typing import Annotated, Any, TypedDict

from langchain_core.messages import HumanMessage, SystemMessage, convert_to_openai_messages
from langgraph.graph import END, START, StateGraph
from langgraph.graph.message import add_messages

_LOG = logging.getLogger(__name__)

_INIT_TIMEOUT_S = 45.0
_ACTION_MAX_TURNS = 4
_ACTION_TIMEOUT_S = 75.0
_SNIPPETS_PER_QUERY = 4
_MAX_TOOL_RESPONSE_CHARS = 12000


# ── Slot-table models (ex-tree/models.py) ─────────────────────────────────
@dataclass
class Variable:
    """One unknown entity to resolve. `id` is immutable across patches."""

    id: int
    type: str
    question_clues: list = field(default_factory=list)
    discovered_clues: list = field(default_factory=list)
    candidate: str | None = None
    candidate_strength: float | None = None

    def brief(self) -> str:
        if self.candidate:
            cs = f"{self.candidate_strength:.2f}" if self.candidate_strength is not None else "?"
            return f"[{self.id}] {self.type}: {self.candidate} ({cs})"
        return f"[{self.id}] {self.type}: EMPTY"

    def filled(self) -> bool:
        return bool(self.candidate)


@dataclass
class State:
    state: list
    depth: int = 0
    id: str = ""
    answer_variable: int | None = None
    retrieved_evidence_ids: list = field(default_factory=list)

    def __post_init__(self):
        if not self.id:
            self.id = f"{self.depth:03x}_{int(time.time() * 1000) % 100000000:08x}{secrets.token_hex(1)}"

    def unresolved(self) -> list:
        return [v for v in self.state if not v.filled()]

    def by_id(self, vid):
        for v in self.state:
            if v.id == vid:
                return v
        return None

    def brief(self) -> str:
        marks = ["+" if v.filled() else "." for v in self.state]
        return f"d{self.depth}({''.join(marks)})"

    def render_slots(self) -> str:
        lines = []
        for v in self.state:
            line = f"- id={v.id} type={v.type}"
            if v.question_clues:
                line += "\n  question_clues: " + "; ".join(v.question_clues)
            if v.discovered_clues:
                line += "\n  discovered_clues: " + "; ".join(v.discovered_clues[-4:])
            if v.candidate:
                cs = f"{v.candidate_strength:.2f}" if v.candidate_strength is not None else "?"
                line += f"\n  CANDIDATE: {v.candidate} (strength={cs})"
            lines.append(line)
        return "\n".join(lines)


@dataclass
class Result:
    """Outcome of ONE run_action session (one of new_states / found_answer)."""

    messages: list = field(default_factory=list)
    new_states: list = field(default_factory=list)
    found_answer: str | None = None
    retrieved_evidence_ids: list = field(default_factory=list)


# ── Tool surface ─────────────────────────────────────────────────────────
_RETRIEVE_TOOL_SPEC = {
    "type": "function",
    "function": {
        "name": "retrieve",
        "description": (
            "Keyword-first search of the fixed document corpus. Pass natural-"
            "language queries; returns SHORT snippets of the most relevant "
            "passages (exact-term matched where possible). Use multiple queries "
            "to cover different aspects. Supports 1-3 queries per call."
        ),
        "parameters": {
            "type": "object",
            "properties": {
                "query": {
                    "type": "array",
                    "items": {"type": "string"},
                    "minItems": 1,
                    "maxItems": 3,
                }
            },
            "required": ["query"],
        },
    },
}

_LIST_CHUNKS_TOOL_SPEC = {
    "type": "function",
    "function": {
        "name": "list_chunks",
        "description": (
            "Deep-read the FULL text of one document by doc_id (returned in "
            "retrieve snippets). Use for enumeration / count / arithmetic answers "
            "when snippets are insufficient. Returns all chunks of the document "
            "in reading order. One doc_id per call."
        ),
        "parameters": {
            "type": "object",
            "properties": {
                "doc_id": {
                    "type": "string",
                    "description": "document id seen in a retrieve snippet",
                }
            },
            "required": ["doc_id"],
        },
    },
}

_SEARCH_CHUNKS_TOOL_SPEC = {
    "type": "function",
    "function": {
        "name": "search_chunks",
        "description": (
            "Semantic retrieval (hybrid vector+BM25) for a natural-language "
            "query. Use when exact-term ``retrieve`` returns nothing useful — the "
            "answer passage may share NO surface words with the query. Returns "
            "snippet chunks ranked by relevance. 1-2 queries per call."
        ),
        "parameters": {
            "type": "object",
            "properties": {
                "query": {
                    "type": "array",
                    "items": {"type": "string"},
                    "minItems": 1,
                    "maxItems": 2,
                }
            },
            "required": ["query"],
        },
    },
}

# Multi-tool registry. ``execute_tool`` dispatches by name; add new tools by
# registering their callable + schema here (DeepSearch ToolNode equivalent).
# 方案 B (react unify): search_chunks gives the session the SEMANTIC reach that
# react's model-driven loop had — answer passages often carry no surface terms.
_TOOL_MAP = {"retrieve": _RETRIEVE_TOOL_SPEC, "list_chunks": _LIST_CHUNKS_TOOL_SPEC, "search_chunks": _SEARCH_CHUNKS_TOOL_SPEC}


# ── JSON / terminal parsing helpers ──────────────────────────────────────
def extract_json(text: str):
    """Return the first parseable JSON object found in ``text``.

    Brace-matches to isolate ONE complete object (a greedy regex over-captures
    when the model emits a second object after the first, and ``json.loads``
    then fails with "Extra data"). Falls back to ``json_repair`` per candidate,
    then tries the next opening brace.
    """
    if not text:
        return None
    i = 0
    n = len(text)
    while i < n:
        start = text.find("{", i)
        if start < 0:
            return None
        depth = 0
        for j in range(start, n):
            ch = text[j]
            if ch == "{":
                depth += 1
            elif ch == "}":
                depth -= 1
                if depth == 0:
                    candidate = text[start : j + 1]
                    try:
                        return json.loads(candidate, strict=False)
                    except Exception:
                        pass
                    try:
                        import json_repair

                        return json_repair.loads(candidate)
                    except Exception:
                        break  # invalid object; try the next "{"
        i = start + 1
    return None


def extract_tag(text: str, tag: str):
    """Exact-tag extraction first; then lenient fallbacks for models that wrap
    the JSON in code fences or emit bare objects (observed with DeepSeek-class
    models ignoring the XML protocol)."""
    s, e = text.rfind(f"<{tag}>"), text.rfind(f"</{tag}>")
    if s != -1 and e != -1 and e > s:
        return text[s + len(tag) + 2 : e].strip()
    # fenced ```json ... ``` blocks mentioning the tag name
    fenced = re.findall(r"```(?:json)?\s*(\{.*?\}|\[.*?\])\s*```", text or "", re.DOTALL)
    if not fenced and re.search(rf"<{tag}\b", text or "") is None:
        return None
    for frag in reversed(fenced):  # prefer the LAST block (most recent decision)
        try:
            obj = json.loads(frag)
            keys = set(obj.keys()) if isinstance(obj, dict) else set()
            want = {"new_states"} if tag == "state" else {"answer", "new_state"}
            if isinstance(obj, dict) and (keys & want):
                return frag.strip()
        except Exception:
            continue
    return None


def apply_patch(base: State, branch_patches: list) -> State | None:
    """DeepSearch apply_patch: ONLY existing ids; mutable fields are candidate /
    candidate_strength / discovered_clues. `id` is immutable — patches may not
    add variables."""
    new_vars = []
    for v in base.state:
        new_vars.append(
            Variable(
                id=v.id,
                type=v.type,
                question_clues=list(v.question_clues),
                discovered_clues=list(v.discovered_clues),
                candidate=v.candidate,
                candidate_strength=v.candidate_strength,
            )
        )
    changed = False
    for pv in branch_patches or []:
        if not isinstance(pv, dict) or "id" not in pv:
            return None
        idx = next((i for i, nv in enumerate(new_vars) if nv.id == pv["id"]), None)
        if idx is None:
            continue
        nv = new_vars[idx]
        if "candidate" in pv:
            nv.candidate = str(pv["candidate"]) if pv.get("candidate") else None
            changed = True
        if pv.get("candidate_strength") is not None:
            try:
                nv.candidate_strength = min(max(float(pv["candidate_strength"]), 0.0), 1.0)
                changed = True
            except Exception:
                pass
        if isinstance(pv.get("discovered_clues"), list):
            nv.discovered_clues.extend(str(c)[:160] for c in pv["discovered_clues"][-4:])
            changed = True
    if not changed:
        return None
    return State(
        state=new_vars,
        depth=base.depth + 1,
        answer_variable=base.answer_variable,
        retrieved_evidence_ids=list(base.retrieved_evidence_ids),
    )


# ── Tool executors ───────────────────────────────────────────────────────
def _kb_ids(tools):
    from rag.advanced_rag.harness.tools.search import _get_kb_ids

    return _get_kb_ids(tools) or None


def _seed_evidence(tools):
    kbinfos = getattr(tools, "kbinfos", None)
    if kbinfos is None:
        kbinfos = {}
        try:
            tools.kbinfos = kbinfos
        except Exception:
            pass
    kbinfos.setdefault("chunks", [])
    return kbinfos


def _admit_evidence(kbinfos, kb_seen, c, out, ids, seen, include_doc_id=True):
    """Register one chunk into the session output AND the shared evidence pool.

    Accumulating raw chunks into ``tools.kbinfos`` is what lets the downstream
    formalize_answer / compose stage cite them (DeepSearch search-context
    parity) — without it the research retrieves plenty but Composing reports
    "0 gathered passage(s)" → partial answer → outer loop re-invokes research.

    Imports the chunk helpers locally to keep ``tools.search`` (and its heavy
    deepdoc dependency chain) out of module-import time.
    """
    from rag.advanced_rag.harness.tools.search import _chunk_id, _chunk_text, _doc_id

    cid = _chunk_id(c)
    if cid in seen:
        return
    seen.add(cid)
    ids.append(cid)
    entry = {"id": str(cid), "content": (_chunk_text(c) or "")[:1200]}
    if include_doc_id:
        entry["doc_id"] = _doc_id(c)
    out.append(entry)
    if isinstance(c, dict) and cid not in kb_seen:
        kb_seen.add(cid)
        kbinfos["chunks"].append(c)


async def _run_search(tools, search_fn, queries: list, top_n: int, max_q: int) -> tuple:
    """Run a corpus search fn per query and admit hits to output + evidence pool.

    Shared driver for every query-based tool (retrieve / search_chunks): the
    only differences are the search backend (grep vs hybrid), ``top_n`` and the
    per-call query cap — all passed in.
    """
    from rag.advanced_rag.harness.tools.search import _chunk_id

    kb_ids = _kb_ids(tools)
    out, ids = [], []
    seen = set()
    kbinfos = _seed_evidence(tools)
    kb_seen = {_chunk_id(c) for c in kbinfos["chunks"] if isinstance(c, dict)}
    for fq in queries[:max_q]:
        try:
            res = await search_fn(tools, fq, kb_ids=kb_ids, top_n=top_n)
            cands = res.get("chunks", []) or []
        except Exception:
            _LOG.warning("[action_session] %s failed for %r", getattr(search_fn, "__name__", "search"), fq, exc_info=True)
            continue
        for c in cands[:_SNIPPETS_PER_QUERY]:
            _admit_evidence(kbinfos, kb_seen, c, out, ids, seen)
    return out, ids


async def _exec_retrieve(tools, queries: list) -> tuple:
    """Corpus search via grep_search (exact-term locate, compact snippets)."""
    from rag.advanced_rag.harness.tools.search import grep_search

    return await _run_search(tools, grep_search, queries, top_n=10, max_q=3)


async def _exec_search_chunks(tools, queries: list) -> tuple:
    """Semantic retrieval (hybrid vector+BM25, narrow bypass) — the react-style
    channel that finds passages sharing NO surface words with the query."""
    from rag.advanced_rag.harness.tools.search import hybrid_search

    return await _run_search(tools, hybrid_search, queries, top_n=20, max_q=2)


async def _exec_list_chunks(tools, doc_id: str) -> tuple:
    """Deep-read one document's full text (list_chunks tool)."""
    from rag.advanced_rag.harness.tools.search import _chunk_id, list_chunks

    try:
        res = await list_chunks(tools, doc_id)
    except Exception:
        _LOG.warning("[action_session] list_chunks failed doc=%r", doc_id, exc_info=True)
        return [], []
    out, ids = [], []
    kbinfos = _seed_evidence(tools)
    kb_seen = {_chunk_id(c) for c in kbinfos["chunks"] if isinstance(c, dict)}
    seen = set()
    for c in (res.get("chunks") or [])[:30]:
        cid = _chunk_id(c)
        if not cid:
            continue
        _admit_evidence(kbinfos, kb_seen, c, out, ids, seen, include_doc_id=False)
    return out, ids


def _arg_query_list(args, max_q: int) -> list:
    """Normalize a tool call's ``query`` argument (string | list | absent) and
    cap it — shared by every query-based tool."""
    q = args.get("query") if isinstance(args, dict) else None
    if isinstance(q, str):
        q = [q]
    return [str(x) for x in (q or [])][:max_q]


async def execute_tool(tools, name: str, args: dict) -> tuple:
    """DeepSearch ToolNode: dispatch ONE native tool call by name.

    Returns ``(results, evidence_ids)`` where results is a JSON-serializable
    payload and evidence_ids are any doc/chunk ids worth tracking.
    """
    if name == "retrieve":
        return await _exec_retrieve(tools, _arg_query_list(args, 3))
    if name == "search_chunks":
        return await _exec_search_chunks(tools, _arg_query_list(args, 2))
    if name == "list_chunks":
        return await _exec_list_chunks(tools, str(args.get("doc_id") or ""))
    _LOG.warning("[action_session] unknown tool %r; ignored.", name)
    return [], []


# ── LangGraph session graph ──────────────────────────────────────────────
async def _acompletion(mdl, messages: list, tools_list=None, temperature: float = 0.3, timeout_s: float = 60.0):
    """Provider-agnostic ONE raw completion (shared by the tool loop and the
    salvage call).

    ``messages`` is a list of langchain messages (reducer output); we convert
    them back to OpenAI-dict form for whichever provider path we take.
    ``tools_list`` is None for a plain completion (finalize salvage) or the tool
    schema list to advertise tool-calling. OpenAI-style wrappers expose
    ``async_client`` → native create(); LiteLLMBase reuses its own public
    ``_construct_completion_args`` and calls ``litellm.acompletion`` with our
    schema injected. ``timeout_s`` is the per-call wall-clock bound.
    """
    oai_messages = convert_to_openai_messages(messages)
    if getattr(mdl, "async_client", None) is not None:
        client = mdl.async_client
        chat_obj = getattr(client, "chat", client)
        completions = getattr(chat_obj, "completions", chat_obj)
        create = getattr(completions, "create")
        kwargs = {"model": mdl.model_name, "messages": oai_messages, "temperature": temperature}
        if tools_list:
            kwargs["tools"] = tools_list
        return await create(**kwargs)
    # LiteLLMBase path — construct args via its own public method so provider
    # prefix / api_key / retries all ride along; then swap OUR tool schema in.
    import litellm

    args = mdl._construct_completion_args(
        history=oai_messages,
        stream=False,
        tools=bool(tools_list),
        temperature=temperature,
    )
    if tools_list:
        args["tools"] = tools_list
        args["tool_choice"] = "auto"
    args.setdefault("num_retries", 0)
    return await litellm.acompletion(**args, drop_params=True, timeout=timeout_s)


async def _llm_once_with_tools(mdl, messages: list):
    """ONE native-tool completion with the DeepSearch tool schema."""
    return await _acompletion(mdl, messages, tools_list=list(_TOOL_MAP.values()), temperature=0.3)


def _parse_tool_calls(msg) -> list:
    """Normalize provider-native tool_calls to [{"id", "name", "args"}].

    ``name`` is the tool name the model emitted (retrieve / list_chunks);
    unknown names are filtered so execute_tool never sees them.

    Some providers (MiniMax) return tool_calls WITHOUT a stable ``id``; the
    assistant message must pair each tool_call with a unique id that the
    ``tool`` response then references, otherwise the API rejects with
    "tool call result does not follow tool call". We synthesize a stable
    ``call_N`` id whenever the provider omitted one.
    """
    calls = []
    for i, tc in enumerate(msg.tool_calls or []):
        fn = tc.function
        name = getattr(fn, "name", None) if fn else None
        raw_args = fn.arguments if fn else "{}"
        try:
            args = json.loads(raw_args) if isinstance(raw_args, str) and raw_args.strip() else (raw_args or {})
        except Exception:
            args = {}
        if not isinstance(args, dict):
            args = {}
        if name not in _TOOL_MAP:
            _LOG.warning("[action_session] tool_call to unknown tool %r ignored", name)
            continue
        calls.append({"id": tc.id or f"call_{i}", "name": name, "args": args})
    return calls


def _parse_terminal(content: str, parent_state: State) -> tuple:
    """Parse the two DeepSearch terminal output blocks.

    Returns ``(new_states, found_answer)`` — exactly one of which may be
    non-empty. ``<state>`` patches produce new-state branches; ``<answer>``
    ends the tree with a final answer (optionally patching the state too).
    """
    if "<state>" in content:
        block = extract_tag(content, "state") or "{}"
        data = extract_json(block) or {}
        raw_branches = data.get("new_states") or []
        if raw_branches and all(isinstance(b, dict) and "state" not in b for b in raw_branches):
            raw_branches = [{"state": [b]} for b in raw_branches]
        branches = []
        for br in raw_branches:
            ns = apply_patch(parent_state, br.get("state", []) if isinstance(br, dict) else [])
            if ns is not None:
                branches.append(ns)
        return branches, None
    if "<answer>" in content:
        block = extract_tag(content, "answer") or "{}"
        data = extract_json(block) or {}
        answer = str(data.get("answer", "")).strip() or None
        final_state = parent_state
        if isinstance(data.get("new_state"), list):
            patched = apply_patch(parent_state, data["new_state"])
            if patched is not None:
                final_state = patched
        return [final_state], answer
    return [], None


# ── LangGraph subgraph state ─────────────────────────────────────────────
class _SessionState(TypedDict, total=False):
    messages: Annotated[list, add_messages]
    parent_state: State
    tools: Any
    mdl: Any
    # routing signal
    _pending_calls: list
    _done: bool
    _tool_cache: dict
    # terminal outputs
    new_states: list
    found_answer: Any
    retrieved_evidence_ids: list
    # budget
    attempts: int
    deadline_left: float
    _ctx_budget: int
    _tool_chars: int


async def _run_action_node(state: _SessionState) -> dict:
    """One native-tool LLM turn; route on tool_calls / terminal / nudge."""
    mdl = state["mdl"]
    wall = max(
        15.0,
        min(_ACTION_TIMEOUT_S, state.get("deadline_left") or _ACTION_TIMEOUT_S),
    )
    attempts = state.get("attempts", 0) + 1
    try:
        async with asyncio.timeout(wall):
            resp = await _llm_once_with_tools(mdl, state["messages"])
    except TimeoutError:
        _LOG.warning("[action_session] turn timed out after %.0fs", wall)
        return {"_done": True, "new_states": [], "found_answer": None, "attempts": attempts}
    except Exception as e:  # noqa: BLE001 - 422 content-moderation / conn errors
        _LOG.warning("[action_session] LLM call failed (%s); converging session empty", type(e).__name__)
        return {"_done": True, "new_states": [], "found_answer": None, "attempts": attempts}
    msg = resp.choices[0].message
    content = msg.content or ""

    calls = _parse_tool_calls(msg)
    if calls:
        # pass the model's native tool_calls through verbatim so langgraph pairs
        # them with the tool responses; _pending_calls drives the tool node
        return {
            "messages": [
                {
                    "role": "assistant",
                    "content": content,
                    "tool_calls": [
                        {
                            "id": c["id"],
                            "type": "function",
                            "function": {
                                "name": c["name"],
                                "arguments": json.dumps(c["args"], ensure_ascii=False),
                            },
                        }
                        for c in calls
                    ],
                }
            ],
            "_pending_calls": calls,
            "attempts": attempts,
        }

    new_states, found_answer = _parse_terminal(content, state["parent_state"])
    if found_answer is not None or new_states:
        return {
            "new_states": new_states,
            "found_answer": found_answer,
            "_done": True,
            "attempts": attempts,
        }

    # Neither a call nor a terminal block: nudge once per turn.
    return {
        "messages": [
            {"role": "assistant", "content": content},
            {
                "role": "user",
                "content": "Call the retrieve tool, or output a <state> patch, or emit <answer>.",
            },
        ],
        "_pending_calls": [],
        "attempts": attempts,
    }


async def _tool_node(state: _SessionState) -> dict:
    """Execute pending native tool calls via execute_tool; append ``tool``
    responses (DeepSearch ToolNode equivalent) with a context budget that keeps
    the running conversation within bounds (DeepSearch context_limit strategy)."""
    tools = state["tools"]
    tool_msgs = []
    evidence_ids = list(state.get("retrieved_evidence_ids") or [])
    # crude context budget: track total chars of tool payloads so far; when
    # over budget we shorten each response instead of growing unboundedly.
    budget_chars = int(state.get("_ctx_budget", _MAX_TOOL_RESPONSE_CHARS * 4))
    # O(1) running budget: cumulative tool-payload chars tracked in _tool_chars
    # (a per-node rescan of every tool message was O(n) each call).
    used = int(state.get("_tool_chars", 0))
    cache = state.get("_tool_cache") or {}
    # The assistant message declared EVERY tool_call verbatim (DeepSearch
    # run_action passes raw_tool_calls through), so we MUST return a matching
    # tool response for each and every one of them — a malformed/partial
    # history (assistant.tool_calls without a tool response) is exactly what
    # makes the model re-issue redundant retrieves. The same-session cache only
    # avoids RE-EXECUTING a repeated query, it still returns a response for it.
    pending = state.get("_pending_calls") or []
    for c in pending:
        # same-session tool cache: avoid re-grep/list_chunks on the same target
        cache_key = (c["name"], json.dumps(c["args"], sort_keys=True, ensure_ascii=False))
        if cache_key in cache:
            chunks, ids = cache[cache_key]
        else:
            chunks, ids = await execute_tool(tools, c["name"], c["args"])
            cache[cache_key] = (chunks, ids)
        evidence_ids.extend(ids)
        payload = json.dumps({"passages": chunks}, ensure_ascii=False, default=str)
        # if the session is already heavy, cut this payload proportionally
        if used + len(payload) > budget_chars:
            keep = max(800, budget_chars - used)
            payload = payload[:keep]
        used += len(payload)
        tool_msgs.append(
            {
                "role": "tool",
                "tool_call_id": c["id"],
                "content": payload,
            }
        )
    return {
        "messages": tool_msgs,
        "_pending_calls": [],
        "retrieved_evidence_ids": evidence_ids,
        "_tool_cache": cache,
        "_tool_chars": used,
    }


async def _finalize_node(state: _SessionState) -> dict:
    """Tool budget spent: ONE last call WITHOUT tools demanding the terminal
    JSON, salvaging whatever the session learned."""
    parent = state["parent_state"]
    new_states, found_answer = [], None
    evidence_ids = list(state.get("retrieved_evidence_ids") or [])

    budget_prompt = (
        "TOOL BUDGET EXHAUSTED. Based ONLY on the passages retrieved above, "
        "output now — no prose outside the block:\n"
        '<state>{"new_states": [{"state": [{"id": <slot_id>, "candidate": "<value>", '
        '"candidate_strength": <0..1>, "discovered_clues": ["..."]}]}]}</state>\n'
        'If NOTHING was learned use: <state>{"new_states": []}</state>'
    )
    # Defensive: strip any assistant.tool_calls that never got a tool response
    # (e.g. tool execution failed), else the provider rejects the history.
    finalize_msgs = _strip_unpaired_tool_calls(list(state["messages"])) + [HumanMessage(content=budget_prompt)]
    try:
        async with asyncio.timeout(max(15.0, min(45.0, state.get("deadline_left") or 45.0))):
            fresp = await _acompletion(
                state["mdl"],
                finalize_msgs,
                tools_list=None,
                temperature=0.3,
                timeout_s=45.0,
            )
        fcontent = fresp.choices[0].message.content or ""
        new_states, found_answer = _parse_terminal(fcontent, parent)
        if found_answer:
            _LOG.info("[action_session] answer salvaged from exhausted session")
        elif new_states:
            _LOG.info("[action_session] %d branch(es) salvaged from exhausted session", len(new_states))
    except Exception:
        _LOG.exception("[action_session] salvage call failed")

    # Loose-clue harvest (deterministic, zero-LLM): even when every JSON
    # protocol attempt failed, the last narration often carries facts worth
    # keeping as breadcrumbs.
    if not new_states and not found_answer:
        loose_clues = []
        for m_ in reversed(state["messages"]):
            if getattr(m_, "type", "") == "ai" and str(getattr(m_, "content", "") or "").strip():
                txt = str(m_.content).strip()
                if len(txt) >= 24:
                    loose_clues = [f"narrative: {txt[:220]}"]
                break
        if loose_clues:
            unresolved = parent.unresolved()
            target_id = unresolved[0].id if unresolved else (parent.state[0].id if parent.state else None)
            if target_id is not None:
                patched = apply_patch(
                    parent,
                    [{"id": target_id, "discovered_clues": loose_clues}],
                )
                if patched is not None:
                    new_states = [patched]
                    _LOG.warning("[action_session] loose-clue patch (narrative)")

    return {"new_states": new_states, "found_answer": found_answer, "_done": True, "retrieved_evidence_ids": evidence_ids}


def _strip_unpaired_tool_calls(messages: list) -> list:
    """Remove assistant.tool_calls that never received a matching tool response
    so the conversation handed to the provider stays well-formed (MiniMax 400
    "tool call result does not follow tool call" otherwise). Keeps content."""
    responded = set()
    for m_ in messages:
        if getattr(m_, "type", "") == "tool":
            responded.add(getattr(m_, "tool_call_id", None))
    cleaned = []
    for m_ in messages:
        if getattr(m_, "type", "") == "ai" and getattr(m_, "tool_calls", None):
            paired = all(tc.get("id") in responded for tc in (m_.tool_calls or []))
            if not paired:
                # keep the assistant text, drop the unpaired tool_calls
                try:
                    cleaned.append(m_.__class__(content=m_.content))
                except Exception:  # noqa: BLE001 - best-effort strip
                    cleaned.append(m_)
                continue
        cleaned.append(m_)
    return cleaned


def _route(state: _SessionState) -> str:
    if state.get("_done"):
        return END
    # Run pending tool_calls FIRST, even at the turn budget: leaving an
    # assistant.tool_calls message without its tool response makes the provider
    # reject the next call ("tool call result does not follow tool call"). The
    # tool node clears _pending_calls, then the route re-checks the budget.
    if state.get("_pending_calls"):
        return "tool"
    if state.get("attempts", 0) >= _ACTION_MAX_TURNS:
        return "finalize"
    return "run_action"  # nudge retry


def _build_session_graph():
    g = StateGraph(_SessionState)
    g.add_node("run_action", _run_action_node)
    g.add_node("tool", _tool_node)
    g.add_node("finalize", _finalize_node)
    g.add_edge(START, "run_action")
    g.add_conditional_edges(
        "run_action",
        _route,
        {"tool": "tool", "finalize": "finalize", END: END, "run_action": "run_action"},
    )
    g.add_edge("tool", "run_action")
    g.add_edge("finalize", END)
    return g.compile()


_SESSION_GRAPH = _build_session_graph()


async def run_action_session(
    tools,
    direction: str,
    parent_state: State,
    deadline_left: float | None = None,
    base_summary: str = "",
) -> Result:
    """Bounded graph-edge session pursuing ONE direction."""
    from rag.prompts.template import load_prompt

    system = load_prompt("action_run")
    seed_user = f"Direction: {direction}\n\nState:\n{parent_state.render_slots()}"
    if base_summary:
        seed_user += f"\n\nPrior round summary:\n{base_summary}"

    from rag.advanced_rag.harness.tools.search import _base_chat_mdl

    mdl = _base_chat_mdl(tools)
    if mdl is None:
        _LOG.warning("[action_session] no usable model resolved for action session")
        return Result(messages=[], new_states=[])

    initial: _SessionState = {
        "messages": [SystemMessage(content=system), HumanMessage(content=seed_user)],
        "parent_state": parent_state,
        "tools": tools,
        "mdl": mdl,
        "_pending_calls": [],
        "_done": False,
        "new_states": [],
        "found_answer": None,
        "retrieved_evidence_ids": [],
        "attempts": 0,
        "deadline_left": deadline_left or _ACTION_TIMEOUT_S,
        "_ctx_budget": _MAX_TOOL_RESPONSE_CHARS * 4,
        "_tool_chars": 0,
        "_tool_cache": {},
    }
    try:
        final = await _SESSION_GRAPH.ainvoke(initial)
    except Exception:
        _LOG.exception("[action_session] session failed")
        return Result(messages=[], new_states=[])
    return Result(
        messages=final.get("messages", []),
        new_states=final.get("new_states", []),
        found_answer=final.get("found_answer"),
        retrieved_evidence_ids=final.get("retrieved_evidence_ids", []),
    )


# ── Slot-table builder (ex-tree/runner.py initialize_state) ──────────────
async def _init_chat(tools, system: str, user: str, tmo: float) -> str:
    """ONE bounded LLM turn for the slot-table decomposition."""
    from rag.advanced_rag.harness.tools.search import _base_chat_mdl

    mdl = _base_chat_mdl(tools)
    if mdl is None:
        return ""
    try:
        async with asyncio.timeout(tmo):
            ans, _u = await mdl.async_chat(system, [{"role": "user", "content": user}], {"temperature": 0.3})
            return str(ans or "")
    except TimeoutError:
        _LOG.warning("[action_session:init] timed out (%ds)", tmo)
    except Exception:
        _LOG.exception("[action_session:init] failed")
    return ""


async def initialize_state(tools, question, fanout_hint, deadline_left=None):
    from rag.prompts.template import load_prompt

    system = load_prompt("action_initialize_state")
    user = f"Question: {question}"
    if fanout_hint:
        user += "\n\nCandidate aspects already identified:\n" + "\n".join(f"- {h}" for h in fanout_hint)
    tmo = min(_INIT_TIMEOUT_S, deadline_left or _INIT_TIMEOUT_S)
    raw = await _init_chat(tools, system, user, tmo)
    data = extract_json(raw) or {}
    if not data:
        # one quick retry — transient provider stalls were observed (45s with
        # zero bytes); a second attempt succeeded in production logs.
        raw = await _init_chat(tools, system, user, tmo)
        data = extract_json(raw) or {}
    slots = []
    for i, s in enumerate(data.get("slots") or []):
        if isinstance(s, dict):
            slots.append(
                Variable(
                    id=int(s.get("id", i)),
                    type=str(s.get("type") or "entity"),
                    question_clues=[str(c) for c in (s.get("clues") or [])][:4],
                )
            )
    first_queries = [str(q).strip() for q in (data.get("first_queries") or [])][:3]
    ans_var = data.get("answer_variable")
    if not slots:
        # Decomposition failed (timeout/parse): build the table from planner
        # fanouts so the first round still targets DISTINCT aspects instead of
        # one oversized query (observed: single-slot trees answered directly
        # and died, or grepped the raw question and missed).
        hint_slots = [Variable(id=i, type="aspect", question_clues=[str(h)[:120]]) for i, h in enumerate((fanout_hint or [])[:4])]
        if hint_slots:
            slots = hint_slots
            ans_var = 0
            if not first_queries:
                first_queries = [str(h) for h in (fanout_hint or [])[:3]]
        else:
            slots = [Variable(id=0, type="answer", question_clues=[question])]
            ans_var = 0
            if not first_queries:
                first_queries = [question]
    elif not first_queries:
        first_queries = [question]
    ans_slot = ans_var if isinstance(ans_var, int) and 0 <= ans_var < len(slots) else 0
    root = State(state=slots, depth=0, answer_variable=ans_slot)
    _LOG.info("[action_session:init] %s", root.brief())
    return root, first_queries
