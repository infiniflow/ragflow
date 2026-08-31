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

"""Slot-table research primitives + the graph-edge action session."""

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

from rag.advanced_rag.harness.config import resolve_mode

_LOG = logging.getLogger(__name__)

_INIT_TIMEOUT_S = 45.0
_ACTION_TIMEOUT_S = 75.0
_SNIPPETS_PER_QUERY = 4
_MAX_TOOL_RESPONSE_CHARS = 12000
# Near-duplicate search-query detection: the model re-issues the SAME intent with
# many paraphrase queries (observed: 20+ "Culdcept original creator/designer/
# OmiyaSoft/founder" variants in ONE session), each executed against ES even
# though the evidence is unchanged. We Jaccard-dedupe retrieval queries so a
# paraphrase (>= _NEAR_DUP_JACCARD token overlap with an already-run query) is
# short-circuited with a nudge instead of burning a turn on a redundant search.
_NEAR_DUP_JACCARD = 0.8
_RETRIEVAL_TOOLS = ("search_chunks", "grep_chunks", "grep_search")


def _search_tokens(q: str) -> set[str]:
    """Lowercased alphanumeric tokens of a query for near-duplicate detection."""
    return set(re.findall(r"[a-z0-9]{2,}", (q or "").lower()))


def _is_near_dup(q: str, seen: list[str]) -> bool:
    """True when q shares >= _NEAR_DUP_JACCARD of its tokens with any seen query."""
    if not q or not seen:
        return False
    toks = _search_tokens(q)
    if len(toks) < 2:
        return False
    for s in seen:
        other = _search_tokens(s)
        inter = len(toks & other)
        union = len(toks | other)
        if union and inter / union >= _NEAR_DUP_JACCARD:
            return True
    return False


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
            "SEMANTIC retrieval (hybrid vector+BM25) with COMPILED-STRUCTURE "
            "EXPANSION. Use as the PRIMARY recall tool when exact-term "
            "``retrieve`` returns nothing useful, or when the dataset is large and "
            "you are unsure which document holds the answer — the answer passage "
            "may share NO surface words with the query. "
            "Compiled expansion: automatically appends related chunks from the "
            "dataset's compiled structure (page index, tree/heading hierarchy, "
            "knowledge graph, wiki pages when present) so a semantic hit carries "
            "its structural neighbours (parent/child headings, sibling pages). "
            "If the dataset has NO compiled structure (incl. no wiki), expansion "
            "is a no-op — no error, just semantic hits. "
            "Returns snippet chunks ranked by relevance. 1-2 queries per call."
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
_WEB_SEARCH_TOOL_SPEC = {
    "type": "function",
    "function": {
        "name": "web_search",
        "description": (
            "Search the open WEB. Use ONLY when the needed fact is world "
            "knowledge / recent event / not covered by the fixed corpus — e.g. "
            "a current event, a person's alive-now status, or a statistic newer "
            "than the corpus. If the fact plausibly lives in the documents, "
            "prefer corpus tools (retrieve/search_chunks) first. 1-2 queries per call."
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

_NAVIGATE_TREE_TOOL_SPEC = {
    "type": "function",
    "function": {
        "name": "navigate_tree",
        "description": (
            "LOCATE the RIGHT DOCUMENT among MANY before deep-reading. Use it "
            "BEFORE search_chunks when the dataset is large and you have no "
            "doc_id yet — it routes by TOPIC/CLUSTERING similarity over the "
            "compiled document-navigation tree (not exact surface words), so it "
            "finds the document even when your query words differ from its text. "
            "Returns candidate doc_ids + a first-chunk summary of each. "
            "This is the FIRST hop of a navigation chain: "
            "navigate_tree(query) -> doc_id -> navigate_structure(doc_id, ...) "
            "-> list_chunks(doc_id, chunk_ids). "
            "Use when: the question names a topic/entity/alias but you do not "
            "know which document discusses it; search_chunks returned scattered "
            "hits across many docs and you must pick the source. "
            "Do NOT use if you already hold a doc_id (go straight to "
            "navigate_structure) or if the answer is likely a single exact "
            "passage (prefer retrieve/search_chunks). "
            "If the dataset has no compiled document navigation tree, it returns "
            "empty — fall back to search_chunks."
        ),
        "parameters": {
            "type": "object",
            "properties": {
                "query": {
                    "type": "string",
                    "description": "topic / entity / alias whose document(s) to locate",
                }
            },
            "required": ["query"],
        },
    },
}

_NAVIGATE_STRUCTURE_TOOL_SPEC = {
    "type": "function",
    "function": {
        "name": "navigate_structure",
        "description": (
            "PINPOINT A PASSAGE inside ONE document using its compiled structure "
            "(heading/catalog tree, concept mindmap, or entity graph) — the "
            "in-document counterpart of navigate_tree. "
            "Use AFTER you know the doc_id (from navigate_tree / search_chunks / "
            "retrieve) and need to find where the answer lives WITHOUT reading "
            "every chunk. Returns the structure outline annotated with matching "
            "chunk_ids (reading-order aware). Then call list_chunks(doc_id, "
            "chunk_ids) to read exactly those. "
            "kind: 'catalog' (default) for page-index/heading/timeline trees, "
            "'mindmap' for concept maps, 'graph' for entity-relation graphs. "
            "If the document has NO compiled structure, an empty <doc/> is "
            "returned — fall back to list_chunks to read the full document."
        ),
        "parameters": {
            "type": "object",
            "properties": {
                "doc_id": {"type": "string", "description": "document id seen from a prior tool result"},
                "query": {"type": "string", "description": "what to locate within the document"},
                "kind": {"type": "string", "enum": ["catalog", "mindmap", "graph"], "description": "compiled structure kind, default catalog"},
            },
            "required": ["doc_id"],
        },
    },
}

_CALCULATE_TOOL_SPEC = {
    "type": "function",
    "function": {
        "name": "calculate",
        "description": (
            "COMPUTE a numeric answer by generating and safely running code. "
            "MANDATORY whenever the question asks you to DERIVE a number by "
            "combining facts you found (sum/difference/percentage/ratio/sort/"
            "compare/difference in length/age, price, area, growth, etc.) — do "
            "NOT do arithmetic mentally. Language-neutral: the question and "
            "facts may be in ANY language (English, Chinese, ...); pass the "
            "numbers verbatim as written in the evidence regardless of language. "
            "Steps: (1) collect every needed number first (retrieve / "
            "search_chunks / navigate_* / list_chunks); (2) call calculate with "
            "the question + ALL those numbers; (3) report the computed result "
            "verbatim. If a needed number is missing, search for it first — do "
            "not estimate. If the answer IS one of the stated numbers (no "
            "combination needed), answer directly without this tool."
        ),
        "parameters": {
            "type": "object",
            "properties": {
                "question": {"type": "string", "description": "the user's question, verbatim"},
                "facts": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "numbers/facts found in the evidence, verbatim",
                },
            },
            "required": ["question", "facts"],
        },
    },
}

_GRAPH_EXPLORE_TOOL_SPEC = {
    "type": "function",
    "function": {
        "name": "graph_explore",
        "description": (
            "EXPLORE the compiled KNOWLEDGE GRAPH (entities + relations) for a "
            "RELATIONAL/multi-hop answer. Different from navigate_*: instead of "
            "locating a document or passage, it seeds entities for the query, "
            "hops along their RELATIONS, and returns either a direct answer or "
            "the source passages behind the relevant entities/relations. "
            "Use when the answer requires connecting several entities through "
            "their relations (e.g. who-was-related-to-whom, cause-effect chains, "
            "membership/ownership) and you already have a starting entity from a "
            "search result, a navigation outline, or a list_chunks reading. "
            "If the dataset has NO compiled knowledge graph, it returns empty — "
            "fall back to search_chunks / navigate_structure."
        ),
        "parameters": {
            "type": "object",
            "properties": {
                "query": {"type": "string", "description": "the relational question / starting entity"},
                "doc_scope": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "optional doc_ids to restrict the graph to (from prior navigation/list_chunks)",
                },
            },
            "required": ["query"],
        },
    },
}

_TOOL_MAP = {
    "retrieve": _RETRIEVE_TOOL_SPEC,
    "search_chunks": _SEARCH_CHUNKS_TOOL_SPEC,
    "list_chunks": _LIST_CHUNKS_TOOL_SPEC,
    "navigate_tree": _NAVIGATE_TREE_TOOL_SPEC,
    "navigate_structure": _NAVIGATE_STRUCTURE_TOOL_SPEC,
    "calculate": _CALCULATE_TOOL_SPEC,
    "graph_explore": _GRAPH_EXPLORE_TOOL_SPEC,
    "web_search": _WEB_SEARCH_TOOL_SPEC,
}


def _active_tool_specs(tools) -> list:
    """Tool schemas exposed to the model for THIS mode.

    Visibility rules:
    * the per-mode tool set is declared in ``config.THINKING_MODES`` — ultra is
      the only mode that sees the relational ``graph_explore``.
    * ``web_search`` is hidden when NO web provider is configured — the model
      otherwise retries it up to 4× per session burning turns (40 empty calls
      observed in a 20-question run). Hiding beats any "do not use again" note.
    * compile-only tools (navigate_* / graph_explore) are kept visible even on
      datasets without compiled structure; their executors return an explicit
      empty-with-hint so the model falls back to plain search (see below).
    * RUNTIME: a compile-only tool is REMOVED from the surface after its first
      call proves the dataset has no such compiled structure (the executor marks
      it in ``tools._disabled_tools``). Without this the model keeps re-invoking
      a tool that always returns empty — e.g. 24 graph_explore calls in one
      benchmark run, every one "No compiled knowledge graph in scope".

    The returned list is what the model can call this session.
    """
    names = set(resolve_mode(tools).tools) & set(_TOOL_MAP.keys())
    # Hide web_search without a provider — a hard visibility gate beats hoping
    # the model obeys a "do not use" note across sessions.
    if getattr(tools, "web_search", None) is None:
        names.discard("web_search")
    # Runtime: drop tools proven unavailable this session (no compiled structure).
    disabled = getattr(tools, "_disabled_tools", None) or set()
    if disabled:
        names -= set(disabled)
    return [spec for name, spec in _TOOL_MAP.items() if name in names]


def _disable_tool(tools, name: str) -> None:
    """Mark a compile-only tool as unavailable for the REST of this session.

    Records the (best-effort, tools-object-scoped) fact that the dataset has no
    such compiled structure, so ``_active_tool_specs`` stops advertising it and
    ``execute_tool`` short-circuits it. Subsequent calls return a note instead of
    re-running a retrieval that would only come back empty.
    """
    if name not in _TOOL_MAP:
        return
    try:
        disabled = getattr(tools, "_disabled_tools", None)
        if disabled is None:
            disabled = set()
            try:
                tools._disabled_tools = disabled
            except Exception:  # tools may be frozen/slots in tests
                return
        disabled.add(name)
    except Exception:
        _LOG.warning("[action_session] could not mark tool %r disabled", name, exc_info=True)


def _is_empty_structure(text, xml_tag: str) -> bool:
    """True when a compiled-structure executor's payload has no usable content.

    A payload is "empty" not only when blank — a well-formed XML with
    ``count="0"`` or an ``error=`` marker also means "no structure compiled for
    this kind". Treating those as empty lets the executor disable the tool and
    steer the model back to plain retrieval instead of feeding it a count="0"
    outline it will keep probing.
    """
    text = (text or "").strip()
    if not text or text in ("[]", "{}"):
        return True
    if xml_tag and f"<{xml_tag}" in text and 'count="0"' in text:
        return True
    if "error=" in text:
        return True
    return False


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


async def _run_search(tools, search_fn, queries: list, top_n: int, max_q: int, **kw) -> tuple:
    """Run a corpus search fn per query and admit hits to output + evidence pool.

    Shared driver for every query-based tool (retrieve / search_chunks): the
    only differences are the search backend (grep vs hybrid), ``top_n``, the
    per-call query cap — all passed in. ``kw`` forwards backend kwargs such as
    ``use_compiled`` (search_chunks' compiled-structure expansion).
    """
    from rag.advanced_rag.harness.tools.search import _chunk_id

    kb_ids = _kb_ids(tools)
    out, ids = [], []
    seen = set()
    kbinfos = _seed_evidence(tools)
    kb_seen = {_chunk_id(c) for c in kbinfos["chunks"] if isinstance(c, dict)}
    for fq in queries[:max_q]:
        try:
            res = await search_fn(tools, fq, kb_ids=kb_ids, top_n=top_n, **kw)
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


async def _exec_search_chunks(tools, queries: list, use_compiled: bool = False) -> tuple:
    """Semantic retrieval (hybrid vector+BM25, narrow bypass) — the react-style
    channel that finds passages sharing NO surface words with the query.

    ``use_compiled=True`` (high mode) turns on hybrid_search's COMPILED
    expansion: page-index / tree / knowledge-graph / wiki pages (when the
    dataset has them) are appended to a semantic hit so its structural
    neighbours (parent/child headings, sibling wiki pages) come along.
    Datasets with NO compiled structure are unaffected — expansion is a no-op.
    """
    from rag.advanced_rag.harness.tools.search import hybrid_search

    return await _run_search(tools, hybrid_search, queries, top_n=20, max_q=2, use_compiled=use_compiled)


async def _exec_web_search(tools, queries: list) -> tuple:
    """Open-web retrieval via the configured provider (Tavily/QuerIt/Serply/
    YouCom). Results share the RAGFlow chunk shape so they merge into the same
    evidence pool as corpus hits."""
    from rag.advanced_rag.harness.tools.search import _chunk_id

    provider = getattr(tools, "web_search", None)
    if provider is None:
        # No web provider configured. DO NOT return an empty payload (the model
        # would just retry and burn turns). Return an explicit, actionable note
        # so the model moves on to the corpus tools — a web failure must never
        # block the Q&A.
        _LOG.warning("[action_session] web_search unavailable (no provider configured)")
        return [{"kind": "web_search", "note": "Web search is NOT configured for this session. Do not use this tool again; use the corpus tools (retrieve / search_chunks / navigate_*) instead."}], []

    out, ids = [], []
    seen = set()
    kbinfos = _seed_evidence(tools)
    kb_seen = {_chunk_id(c) for c in kbinfos["chunks"] if isinstance(c, dict)}
    for q in queries[:2]:
        try:
            web_res = provider.retrieve_chunks(q)
            if asyncio.iscoroutine(web_res) or hasattr(web_res, "__await__"):
                web_res = await web_res
        except Exception:
            _LOG.warning("[action_session] web_search failed for %r", q, exc_info=True)
            continue
        for c in ((web_res or {}).get("chunks") or [])[:8]:
            cid = _chunk_id(c)
            if not cid or cid in seen:
                continue
            _admit_evidence(kbinfos, kb_seen, c, out, ids, seen, include_doc_id=False)
    return out, ids


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


def _inject_nav_tools_ref(tools) -> None:
    """navigation.py's _navigate_*_impl / graph_explore read the request-scoped
    tools instance from THEIR OWN module-level ``_tools_ref`` (for tenant /
    embed_mdl resolution). The old bind_dynamic_tools used to populate it; it
    is gone, so populate it here before any navigation/graph call — otherwise
    those tools crash on a None tools instance ('NoneType' has no attribute
    '_resolve_doc_tenant') or warn 'no embed_mdl available' and return empty.
    """
    try:
        import rag.advanced_rag.harness.tools.navigation as _nav

        _nav._tools_ref["tools"] = tools
    except Exception:
        _LOG.warning("[action_session] could not inject navigation tools ref", exc_info=True)


async def _exec_navigate_tree(tools, args: dict) -> tuple:
    """Document-level embedding routing (navigate_tree). Returns doc candidates
    as a text outline; evidence ids = routed doc_ids for later deep-read."""
    from rag.advanced_rag.harness.tools.navigation import _navigate_tree_impl

    _inject_nav_tools_ref(tools)
    query = str(args.get("query") or "")
    text = await _navigate_tree_impl(query, keywords=str(args.get("keywords") or ""))
    # Empty / count="0" / error= payloads mean no compiled navigation tree —
    # this dataset has no such structure. Disable the tool for the session and
    # tell the model to fall back to plain retrieval (a count="0" XML is NOT a
    # useful payload: without this, the model keeps re-invoking it).
    text = (text or "").strip()
    if _is_empty_structure(text, "tree_navigation"):
        _disable_tool(tools, "navigate_tree")
        _LOG.info("[action_session] navigate_tree disabled: dataset has no compiled navigation tree (text=%r)", text[:120])
        return [{"kind": "navigate_tree", "note": "This dataset has NO compiled document navigation tree. navigate_tree is unavailable; use search_chunks / retrieve instead."}], []
    return [{"kind": "navigate_tree", "content": text[:8000]}], []


async def _exec_navigate_structure(tools, args: dict) -> tuple:
    """In-document compiled-structure navigation (navigate_structure). Returns
    outline annotated with chunk_ids; those become evidence for list_chunks."""
    from rag.advanced_rag.harness.tools.navigation import _navigate_structure_impl

    _inject_nav_tools_ref(tools)
    doc_id = str(args.get("doc_id") or "")
    query = str(args.get("query") or "")
    kind = str(args.get("kind") or "catalog")
    text = await _navigate_structure_impl(query, doc_id=doc_id, kind=kind)
    # Empty / count="0" / error= payloads mean no compiled structure of this kind
    # in the dataset — the tool is a dead end. Disable it for the session so the
    # model stops re-invoking it (a count="0" XML is NOT a useful payload).
    text = (text or "").strip()
    if _is_empty_structure(text, "structure_navigation"):
        _disable_tool(tools, "navigate_structure")
        _LOG.info("[action_session] navigate_structure disabled: no compiled structure kind=%r (text=%r)", kind, text[:120])
        return [
            {
                "kind": "navigate_structure",
                "doc_id": doc_id,
                "note": "This document/dataset has NO compiled structure of the requested kind. navigate_structure is unavailable here; use search_chunks / retrieve / list_chunks instead.",
            }
        ], [doc_id] if doc_id else []
    return [{"kind": "navigate_structure", "doc_id": doc_id, "content": text[:8000]}], [doc_id] if doc_id else []


async def _exec_calculate(tools, args: dict) -> tuple:
    """Arithmetic terminal: compute_from_facts decides whether the question
    needs a computed number and safely evaluates an expression over facts."""
    from rag.advanced_rag.harness.arithmetic import compute_from_facts

    question = str(args.get("question") or "")
    facts = [str(f) for f in (args.get("facts") or []) if str(f).strip()]
    # compute_from_facts needs BOTH ``max_length`` (message-fit budget) and
    # ``async_chat``. The OUTER chat_mdl (CountingChatModel/LLMBundle) carries
    # max_length; the innermost LiteLLMBase that _base_chat_mdl returns does
    # NOT. Pass the outer bundle.
    mdl = getattr(tools, "chat_mdl", None)
    if mdl is None:
        return [{"kind": "calculate", "error": "no model"}], []
    try:
        res = await compute_from_facts(mdl, question, facts)
    except Exception:
        _LOG.warning("[action_session] calculate failed", exc_info=True)
        res = None
    if not res:
        # nothing derivable -> tell the model so it doesn't loop on it
        return [{"kind": "calculate", "expression": None, "note": "no numeric answer derivable from given facts; answer directly or retrieve more numbers."}], []
    return [{"kind": "calculate", "expression": res.get("expression"), "result": res.get("value")}], []


async def _exec_graph_explore(tools, args: dict) -> tuple:
    """Relational knowledge-graph exploration (ultra-only). Returns either a
    direct answer or source passages behind relevant entities/relations, so the
    model can continue with list_chunks/navigate on the returned chunk/doc ids."""
    from rag.advanced_rag.harness.tools.navigation import graph_explore

    _inject_nav_tools_ref(tools)
    query = str(args.get("query") or "")
    doc_scope = [str(d) for d in (args.get("doc_scope") or []) if str(d).strip()]
    try:
        res = await graph_explore(tools, query, doc_scope=doc_scope or None)
    except Exception:
        _LOG.warning("[action_session] graph_explore failed", exc_info=True)
        res = {}
    answer = str(res.get("answer") or "").strip()
    chunks = res.get("chunks") or []
    if answer:
        return [{"kind": "graph_explore", "answer": answer}], []
    if not chunks:
        # No knowledge graph compiled for this dataset/scope: relational
        # exploration is a dead end. Disable the tool for the session (it will
        # keep coming back empty otherwise — 24 empty calls in one run) and
        # guide the model back to retrieval.
        _disable_tool(tools, "graph_explore")
        _LOG.info("[action_session] graph_explore disabled: no compiled knowledge graph in scope")
        return [
            {
                "kind": "graph_explore",
                "note": "This dataset has NO compiled knowledge graph (or none in the given scope). graph_explore is unavailable; use search_chunks / navigate_structure / retrieve instead.",
            }
        ], []
    snippet = [{"id": c.get("id"), "content": (str(c.get("content") or "")[:1500])} for c in chunks[:6]]
    ids = [str(c.get("id")) for c in chunks[:6] if c.get("id")]
    return [{"kind": "graph_explore", "chunks": snippet}], ids


async def execute_tool(tools, name: str, args: dict) -> tuple:
    """DeepSearch ToolNode: dispatch ONE native tool call by name.

    Returns ``(results, evidence_ids)`` where results is a JSON-serializable
    payload and evidence_ids are any doc/chunk ids worth tracking.
    """
    # Short-circuit a tool already proven unavailable this session (no compiled
    # structure of its kind). We still return a note, not an error, so the model
    # learns to switch to the corpus tools rather than loop.
    if name in (_TOOL_MAP and (getattr(tools, "_disabled_tools", None) or set())):
        _LOG.info("[action_session] tool %r disabled (no compiled structure); returning note", name)
        return [{"kind": name, "note": f"{name} is unavailable in this dataset (no compiled structure of its kind). Use search_chunks / retrieve / list_chunks instead."}], []
    if name == "retrieve":
        return await _exec_retrieve(tools, _arg_query_list(args, 3))
    if name == "search_chunks":
        # Compiled-structure expansion (page-index/tree/wiki/KG) inside
        # hybrid_search is enabled in ALL modes. Datasets with NO compiled
        # structure are unaffected — expansion is a no-op there.
        return await _exec_search_chunks(tools, _arg_query_list(args, 2), use_compiled=True)
    if name == "list_chunks":
        return await _exec_list_chunks(tools, str(args.get("doc_id") or ""))
    if name == "navigate_tree":
        return await _exec_navigate_tree(tools, args)
    if name == "navigate_structure":
        return await _exec_navigate_structure(tools, args)
    if name == "calculate":
        return await _exec_calculate(tools, args)
    if name == "graph_explore":
        return await _exec_graph_explore(tools, args)
    if name == "web_search":
        return await _exec_web_search(tools, _arg_query_list(args, 2))
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


async def _llm_once_with_tools(tools, mdl, messages: list):
    """ONE native-tool completion with THIS mode's tool surface.

    The exposed tool set is mode-dependent: ultra adds the relational
    ``graph_explore``; high/medium keep the 7-tool document/structure surface.
    """
    return await _acompletion(mdl, messages, tools_list=_active_tool_specs(tools), temperature=0.3)


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
            # Do NOT drop it. OpenAI's protocol requires every assistant
            # tool_call to be answered by a matching ``tool`` message — dropping
            # one leaves a dangling tool_call and the next request is rejected
            # ("tool call result does not follow tool call"). Keep it as a
            # "unknown" call so _tool_node replies with a correction instead.
            #
            # Models most often emit the XML protocol tags (state / answer) as
            # tool names; the hint below steers them back to plain-text output.
            _LOG.warning("[action_session] tool_call to unknown tool %r; replying with a hint", name)
            calls.append({"id": tc.id or f"call_{i}", "name": name, "args": args, "unknown": True})
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
            resp = await _llm_once_with_tools(state["tools"], mdl, state["messages"])
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
    seen_queries = list(state.get("_search_queries") or [])
    skipped = 0
    for c in pending:
        # Near-duplicate retrieval suppression: if the model re-issues the same
        # intent as an earlier search (paraphrase), do NOT re-run ES — return a
        # nudge so it patches / reframes instead of burning turns (Q30 burned
        # 20+ "Culdcept creator" paraphrases in one session).
        q = str((c.get("args") or {}).get("query") or "").strip()
        if c["name"] in _RETRIEVAL_TOOLS and q and _is_near_dup(q, seen_queries):
            skipped += 1
            _LOG.info("[action_session] skipping near-duplicate retrieval %r (already searched)", q[:80])
            tool_msgs.append(
                {
                    "role": "tool",
                    "tool_call_id": c["id"],
                    "content": json.dumps(
                        {
                            "passages": [
                                {
                                    "kind": c["name"],
                                    "note": "This query is a near-duplicate of an earlier retrieval and was skipped to avoid redundant searching. Patch the slot with what you have, or issue a genuinely NEW retrieval angle.",
                                }
                            ]
                        },
                        ensure_ascii=False,
                    ),
                }
            )
            continue
        # Unknown tool name — never execute it; answer with a correction so the
        # model can recover. Models usually emit the XML protocol tags (state /
        # answer) as tool names; they belong in the reply body as plain text.
        if c.get("unknown"):
            hint = (
                f"'{c['name']}' is not a tool. State patches and final answers are "
                "plain TEXT in your reply body, wrapped in <state>...</state> or "
                "<answer>...</answer> XML tags — do not emit them as tool calls. "
                f"Available tools: {', '.join(sorted(_TOOL_MAP))}."
            )
            tool_msgs.append(
                {
                    "role": "tool",
                    "tool_call_id": c["id"],
                    "content": json.dumps({"passages": [{"kind": "error", "note": hint}]}, ensure_ascii=False),
                }
            )
            continue
        # same-session tool cache: avoid re-grep/list_chunks on the same target
        cache_key = (c["name"], json.dumps(c["args"], sort_keys=True, ensure_ascii=False))
        if cache_key in cache:
            chunks, ids = cache[cache_key]
        else:
            chunks, ids = await execute_tool(tools, c["name"], c["args"])
            cache[cache_key] = (chunks, ids)
        if q:
            seen_queries.append(q)
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
        "_search_queries": seen_queries,
        "_skipped_dup": int(state.get("_skipped_dup", 0)) + skipped,
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


def _action_max_turns(state: _SessionState) -> int:
    """Per-mode action-session turn budget: ultra goes deeper inside each
    session (more tool calls before the finalize fallback) — the latency cost
    of the relational depth that distinguishes it from high."""
    return resolve_mode(state.get("tools")).action_max_turns


def _route(state: _SessionState) -> str:
    if state.get("_done"):
        return END
    # Run pending tool_calls FIRST, even at the turn budget: leaving an
    # assistant.tool_calls message without its tool response makes the provider
    # reject the next call ("tool call result does not follow tool call"). The
    # tool node clears _pending_calls, then the route re-checks the budget.
    if state.get("_pending_calls"):
        return "tool"
    if state.get("attempts", 0) >= _action_max_turns(state):
        return "finalize"
    # No-progress convergence: if the model keeps re-issuing near-duplicate
    # retrieval queries (skipped >=2), further turns are unlikely to surface new
    # evidence — converge to finalize instead of burning the remaining budget on
    # paraphrases (Q30/Q759 timeout root cause).
    if int(state.get("_skipped_dup", 0)) >= 2:
        _LOG.info("[action_session] %d near-duplicate retrieval(s) skipped; converging session early", int(state.get("_skipped_dup", 0)))
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
        "_search_queries": [],
        "_skipped_dup": 0,
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
        _LOG.warning("[Action Session:init] timed out (%ds)", tmo)
    except Exception:
        _LOG.exception("[Action Session:init] failed")
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
    if not slots:
        # Decomposition failed (timeout/parse): build the table from planner
        # fanouts so the first round still targets DISTINCT aspects instead of
        # one oversized query (observed: single-slot trees answered directly
        # and died, or grepped the raw question and missed).
        hint_slots = [Variable(id=i, type="aspect", question_clues=[str(h)[:120]]) for i, h in enumerate((fanout_hint or [])[:4])]
        if hint_slots:
            slots = hint_slots
            if not first_queries:
                first_queries = [str(h) for h in (fanout_hint or [])[:3]]
        else:
            slots = [Variable(id=0, type="answer", question_clues=[question])]
            if not first_queries:
                first_queries = [question]
    elif not first_queries:
        first_queries = [question]
    root = State(state=slots, depth=0)
    _LOG.info("[Action Session:init] %s", root.brief())
    return root, first_queries
