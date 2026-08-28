"""Search tools: hybrid, vector, BM25, web, structured."""

import logging
import re
from typing import Any
from rag.llm.tool_decorator import tool
import hashlib
from functools import lru_cache
from common import settings
from .navigation import _expand_related_via_structure, _kg_scopes

_LOG = logging.getLogger(__name__)


def _compact_keywords(kw: str, max_terms: int = 15) -> str:
    """Deduplicate and cap a formalize / extract_keywords keyword string.

    The extraction prompt asks the model for 3-10 terms *plus* 2-3 synonyms each,
    which models answer with a 40-60 word redundant synonym run (e.g. "average
    distance left field line MLB retractable roof stadiums 2024 ... retractable
    dome covered stadium mean distance outfield" — ~350 chars). Appending that
    whole run onto the query diluted the vector leg and dragged BM25 onto
    unrelated docs. This keeps the recall terms but drops the redundancy:
    dedupe (preserving order) and cap at ``max_terms`` so it stays a compact
    hint instead of a pollution source. Accepts both space- and comma-separated
    input (single-turn extract_keywords emits spaces; multi-turn formalize
    emits commas).
    """
    if not kw:
        return ""
    tokens = re.split(r"[,\s]+", (kw or "").strip())
    seen: list[str] = []
    for t in tokens:
        t = t.strip()
        if not t:
            continue
        if t not in seen:
            seen.append(t)
        if len(seen) >= max_terms:
            break
    return " ".join(seen)


# Sentence terminators: Chinese 。！？；, English ! ? ;, newline, and a
# digit-guarded English period (so "3.14" / "v1.2" don't split).
_SENT_END = re.compile(r"[。！？；!?;]+|(?<!\d)\.(?!\d)")

# Block-level HTML elements and markdown tables are kept ATOMIC — never split by
# sentence terminators — so a whole table / list / block counts as ONE "sentence"
# for keyword matching and narrowing (a keyword inside one keeps the whole block).
_HTML_TAG = re.compile(r"<(/?)([a-zA-Z][a-zA-Z0-9]*)\b([^>]*)>")

# Only BLOCK-level containers are protected. Inline tags (<b>, <i>, <a>, <span>,
# <em>, <strong>, <code>, ...) are deliberately excluded so ordinary prose that
# contains inline formatting still splits into sentences normally.
_HTML_BLOCK_TAGS = {
    "table",
    "thead",
    "tbody",
    "tfoot",
    "tr",
    "td",
    "th",
    "caption",
    "colgroup",
    "ul",
    "ol",
    "li",
    "dl",
    "dt",
    "dd",
    "div",
    "p",
    "pre",
    "blockquote",
    "section",
    "article",
    "aside",
    "nav",
    "main",
    "figure",
    "figcaption",
    "header",
    "footer",
    "address",
    "details",
    "summary",
    "form",
    "fieldset",
    "h1",
    "h2",
    "h3",
    "h4",
    "h5",
    "h6",
}

# Markdown table: a header row with a pipe, a separator row of dashes/colons/
# pipes, then zero+ body rows with a pipe.
_MD_TABLE = re.compile(
    r"^[ \t]*\|?[^\n]*\|[^\n]*\r?\n"
    r"[ \t]*\|?[ \t]*:?-{1,}:?[ \t]*(?:\|[ \t]*:?-{1,}:?[ \t]*)+\|?[ \t]*\r?\n"
    r"(?:[ \t]*\|?[^\n]*\|[^\n]*\r?\n?)*",
    re.MULTILINE,
)


def _html_block_spans(text: str) -> list[tuple[int, int]]:
    """Outermost balanced block-level HTML element spans (nesting-aware).

    Uses a tag stack (not a regex) so nested elements (e.g. a ``<table>`` with
    ``<td>``s, or nested ``<div>``s) yield ONE span for the outermost element and
    are never truncated at the first close tag the way a non-greedy regex would.
    Unclosed / stray tags are ignored (that region just falls back to plain
    sentence splitting).
    """
    spans: list[tuple[int, int]] = []
    stack: list[tuple[str, int]] = []
    for m in _HTML_TAG.finditer(text):
        name = m.group(2).lower()
        if name not in _HTML_BLOCK_TAGS:
            continue
        if m.group(1):  # closing tag </name>
            for i in range(len(stack) - 1, -1, -1):
                if stack[i][0] == name:
                    start = stack[i][1]
                    del stack[i:]
                    if not stack:  # closed an outermost block
                        spans.append((start, m.end()))
                    break
            # a stray </name> with no matching open is ignored
        elif not m.group(3).rstrip().endswith("/"):  # opening (skip self-closing)
            stack.append((name, m.start()))
    return spans


def _protected_spans(text: str) -> list[tuple[int, int]]:
    """Non-overlapping ``(start, end)`` spans kept atomic, in order.

    Covers block-level HTML elements and markdown tables; overlapping spans are
    merged (unioned) so a match that straddles two is never split.
    """
    spans = _html_block_spans(text)
    spans += [(m.start(), m.end()) for m in _MD_TABLE.finditer(text)]
    spans.sort()
    merged: list[tuple[int, int]] = []
    last_end = -1
    for s, e in spans:
        if s < last_end:  # overlaps an already-kept span -> union it in
            if e > last_end:
                merged[-1] = (merged[-1][0], e)
                last_end = e
            continue
        merged.append((s, e))
        last_end = e
    return merged


def _split_plain(text: str) -> list[str]:
    """Terminator-based sentence split, keeping each terminator attached."""
    sents: list[str] = []
    start = 0
    for m in _SENT_END.finditer(text):
        end = m.end()
        seg = text[start:end]
        if seg.strip():
            sents.append(seg)
        start = end
    if start < len(text):
        tail = text[start:]
        if tail.strip():
            sents.append(tail)
    return sents


def _split_sentences(text: str) -> list[str]:
    """Split ``text`` into sentences, keeping each terminator attached.

    Block-level HTML elements (``<table>``, ``<div>``, ``<p>``, ``<ul>``, ... —
    see :data:`_HTML_BLOCK_TAGS`) and markdown tables are treated as a single
    atomic sentence and are never split internally, so a keyword falling inside
    one keeps the whole block together.
    """
    if not text:
        return []
    spans = _protected_spans(text)
    if not spans:
        return _split_plain(text)

    sents: list[str] = []
    pos = 0
    for s, e in spans:
        if s > pos:
            sents.extend(_split_plain(text[pos:s]))
        block = text[s:e]
        if block.strip():
            sents.append(block)
        pos = e
    if pos < len(text):
        sents.extend(_split_plain(text[pos:]))
    return sents


# ---------------------------------------------------------------------------
# Stem-tolerant keyword matching (ported from agentic_search4 v8)
#
# Substring matching misses inflected forms: "nominations" misses "nominated",
# "company" misses "companies". Keywords are derived from the question, which
# states things in the inflected form ("which band HEADLINED", "was NOMINATED
# three times"), so the failing direction is the common one. Both sides are
# therefore reduced to a stem before comparison.
# ---------------------------------------------------------------------------
try:  # available at runtime — nltk already backs rag/nlp/synonym.py
    from nltk.stem import PorterStemmer as _PorterStemmer

    _porter_stem = _PorterStemmer().stem
except Exception:  # pragma: no cover - exercised only where nltk is absent
    _porter_stem = None

# Longest first: "nominations" must lose "ations", not just the trailing "s".
_STEM_SUFFIXES = (
    ("ations", ""),
    ("ation", ""),
    ("ated", ""),
    ("ates", ""),
    ("ate", ""),
    ("ings", ""),
    ("ing", ""),
    ("ies", "i"),
    ("ied", "i"),
    ("ed", ""),
    ("es", ""),
    ("s", ""),
)

_WORD_RE = re.compile(r"[a-z0-9]+")


def _fallback_stem(word: str) -> str:
    """Suffix stripper used when nltk is unavailable. Approximates Porter."""
    w = word
    for suffix, replacement in _STEM_SUFFIXES:
        if w.endswith(suffix) and len(w) - len(suffix) >= 3:
            w = w[: len(w) - len(suffix)] + replacement
            break
    if len(w) > 3 and w.endswith("y"):
        w = w[:-1] + "i"
    if len(w) > 3 and w.endswith("e"):
        w = w[:-1]
    if len(w) > 3 and w[-1] == w[-2] and w[-1] not in "aeiou":
        w = w[:-1]  # running -> runn -> run
    return w


@lru_cache(maxsize=8192)
def _stem(word: str) -> str:
    return _porter_stem(word) if _porter_stem else _fallback_stem(word)


def _stemmable(token: str) -> bool:
    """Only plain ASCII words are stemmed.

    Identifiers ("1344259", "2020-21"), notation ("PPG") and CJK text must match
    verbatim — stemming would corrupt them, and it has no meaning for Chinese.
    """
    return len(token) >= 4 and token.isascii() and token.isalpha()


def _keyword_forms(kwds: list[str]) -> tuple[list[str], list[tuple[str, ...]]]:
    """Split keywords into verbatim substrings and stem sequences.

    A keyword whose tokens are ALL stemmable becomes a stem sequence (matched
    anywhere as a contiguous run of stems); anything containing an identifier,
    notation or CJK falls back to a verbatim substring match.
    """
    verbatim: list[str] = []
    stemmed: list[tuple[str, ...]] = []
    for kw in kwds or []:
        k = (kw or "").strip().lower()
        if not k:
            continue
        tokens = _WORD_RE.findall(k)
        if tokens and all(_stemmable(t) for t in tokens):
            stemmed.append(tuple(_stem(t) for t in tokens))
        else:
            verbatim.append(k)
    return verbatim, stemmed


def _sentence_stems(sentence: str) -> list[str]:
    return [_stem(t) if _stemmable(t) else t for t in _WORD_RE.findall(sentence.lower())]


def _sentence_matches(low: str, stems: list[str], verbatim: list[str], stemmed: list[tuple[str, ...]]) -> bool:
    """True when a sentence contains a verbatim keyword or a contiguous stem run."""
    if any(v in low for v in verbatim):
        return True
    for seq in stemmed:
        width = len(seq)
        for start in range(len(stems) - width + 1):
            if tuple(stems[start : start + width]) == seq:
                return True
    return False


_FACT_RE = re.compile(
    r"(\d[\d,\.]*(?:st|nd|rd|th)?%?)"
    r"|(19|20)\d{2}"  # years
    r"|\b(percent|percentage|million|billion|thousand|km|km2|sq\s*km|m\s*above|m)"
    r"\b",
    re.IGNORECASE,
)
_PROPER_NOUN_RE = re.compile(r"(?<![.!?]\.)\b[A-Z][a-z]{2,}\b")


def _is_fact_dense_sentence(sent: str) -> bool:
    """Heuristically flag a sentence that carries a fact the answer may hinge on
    but which does not necessarily contain the query keywords — a number, a year,
    a percentage, or a proper noun / named entity. Such sentences are kept during
    narrowing even when they sit far from any keyword hit, so a numeric or
    entity answer is never dropped just because it lacks the keyword phrasing.
    """
    low = sent.lower()
    if _FACT_RE.search(sent) or _FACT_RE.search(low):
        return True
    if _PROPER_NOUN_RE.search(sent):
        return True
    return False


def _narrow_content(content: str, kwds: list[str]) -> str | None:
    """Return ``content`` narrowed to keyword sentences +/- 2 neighbours.

    Matching is stem-tolerant: a keyword matches any word sharing its stem, so
    "nominations" finds "nominated". Sentences that are fact-dense (numbers /
    years / percentages / proper nouns) are kept regardless of keyword distance,
    so numeric or named-entity answers survive narrowing. Returns ``None`` when
    no keyword occurs anywhere in ``content``.
    """
    # Structured tables must be returned whole. A keyword hit anywhere in a big
    # table (e.g. the capitals-by-latitude table) otherwise narrows to the hit
    # sentence +/- neighbours and DROPS the far end of the table — precisely the
    # "table truncated at -4.58°N, Maseru (-29.3°) missing" bug on FRAMES Q408.
    # A table row is one data point, not a sentence, so keyword-window narrowing is
    # wrong here; keep the full table (it is already rank-sorted by the retriever).
    if "<table" in content.lower() or "<tr" in content.lower() or "<td" in content.lower():
        return "..." + _highlight_keywords(content, kwds) + "..."

    sents = _split_sentences(content)
    if not sents:
        return None
    # Stem-tolerant matching: a keyword matches any word sharing its stem, so
    # "nominations" finds "nominated" and "company" finds "companies".
    verbatim, stemmed = _keyword_forms(kwds)
    if not verbatim and not stemmed:
        return None
    keep: set[int] = set()
    matched = False
    for i, s in enumerate(sents):
        low = s.lower()
        if _sentence_matches(low, _sentence_stems(s), verbatim, stemmed):
            matched = True
            for j in range(max(0, i - 2), min(len(sents), i + 3)):
                keep.add(j)
        elif _is_fact_dense_sentence(s):
            # Keep fact-dense sentences even without a keyword hit so the answer
            # value (a bare figure, a date, a proper noun) is never lost.
            for j in range(max(0, i - 1), min(len(sents), i + 2)):
                keep.add(j)
    if not matched:
        return None
    narrowed = "".join(sents[i] for i in sorted(keep)).strip()
    return "..." + _highlight_keywords(narrowed, kwds) + "..."


def _highlight_keywords(text: str, kwds: list[str]) -> str:
    """Star the verbatim keyword phrases AND any word sharing a keyword's stem.

    Full keyword phrases are matched first and starred as ONE contiguous span, so
    a multi-word entity like "Atlanta Braves" becomes ``*Atlanta Braves*`` — never
    ``*Atlanta* *Braves*`` — because the downstream cross-check matches entities
    with a bounded contiguous regex that a per-word star would break.
    """
    phrases = sorted({(kw or "").strip().lower() for kw in kwds or [] if (kw or "").strip()}, key=len, reverse=True)
    terms: list[str] = list(phrases)
    # Add stem-matched words NOT already inside a phrase, so "nominated" still
    # gets starred for keyword "nominations" while "Atlanta Braves" stays whole.
    verbatim, stemmed = _keyword_forms(kwds)
    stem_set = {s for seq in stemmed for s in seq}
    if stem_set:
        for word in re.findall(r"[A-Za-z]+", text):
            low = word.lower()
            if _stemmable(low) and _stem(low) in stem_set and not any(low in p for p in phrases):
                terms.append(low)
    if not terms:
        return text
    # Longest first so a phrase wins over a word it contains; one pass, so an
    # already-starred span is never starred again.
    pattern = re.compile("|".join(re.escape(t) for t in sorted(terms, key=len, reverse=True)), re.IGNORECASE)
    return pattern.sub(lambda m: f"*{m.group(0)}*", text)


def _narrow_by_keywords(chunks: list[dict], keywords: str) -> list[dict]:
    """Narrow each chunk to its keyword-bearing sentences (+/- 1 neighbour) and
    drop keyword-less chunks.

    Keywords are the comma-separated terms (with close synonyms) produced by
    ``formalize``; matching is stem-tolerant (a keyword matches any word sharing
    its stem).
    """
    kwds = [k.strip().lower() for k in (keywords or "").split(",") if k.strip()]
    if not kwds or not chunks:
        return chunks
    if len(kwds) < 3:
        kwds = [k.strip().lower() for k in (keywords or "").split(" ") if k.strip()]
        _kwds = []
        for i in range(len(kwds) - 1):
            _kwds.append(kwds[i] + " " + kwds[i + 1])
        kwds = _kwds

    scored = [(ck, _narrow_content(ck.get("content_with_weight") or ck.get("content") or "", kwds)) for ck in chunks]
    out: list[dict] = []
    dedup: set[str] = set()
    for ck, nc in scored:
        if nc is not None:
            nc_hash = hashlib.md5(nc.encode("utf-8")).hexdigest()
            if nc_hash in dedup:
                continue
            dedup.add(nc_hash)
            ck["content_with_weight"] = nc
            if "content" in ck:
                ck["content"] = nc
            ck.pop("highlight", None)
            out.append(ck)
    return out


def _narrow_or_keep(chunks: list[dict], keywords: str, label: str) -> list[dict]:
    """Narrow chunks to keyword sentences, but keep the originals when
    narrowing would drop everything.

    No keyword overlap does not mean irrelevant — the retriever already ranked
    these chunks, and a sub-question's wording need not contain the parent
    question's keywords. Dropping them all produced empty results, unverified
    claims and pointless retry cycles.
    """
    if not keywords or not chunks:
        return chunks
    length = len(chunks)
    narrowed = _narrow_by_keywords(chunks, keywords)
    if narrowed:
        _LOG.info(f"[{label}] Kept {len(narrowed)} of {length} passage(s) that actually mention the keywords.")
        return narrowed
    _LOG.info(f"[{label}] Keyword narrowing matched nothing — keeping all {length} retrieved passage(s).")
    return chunks


def _search_cache_key(effective_query: str, target_ids, top_n: int, doc_scope) -> tuple:
    """Key a retrieval by what actually determines its result.

    Includes the scope/limits so semantically different searches are never
    collapsed together — only a genuinely identical query is served from cache.
    """
    return (
        " ".join((effective_query or "").split()).lower(),
        tuple(sorted(target_ids or ())),
        int(top_n),
        tuple(sorted(doc_scope or ())),
    )


def _normalize(kbinfos: dict, tenant_ids: list[str] | str | None) -> dict:
    if not kbinfos:
        return {"chunks": [], "doc_aggs": []}
    if not tenant_ids:
        _LOG.warning("search: skip child retrieval because tenant_ids is empty")
        return kbinfos
    if isinstance(tenant_ids, str):
        tenant_ids = [tenant_ids]
    kbinfos["chunks"] = settings.retriever.retrieval_by_children(
        kbinfos.get("chunks", []),
        tenant_ids,
    )
    return kbinfos


async def hybrid_search(
    tools, query: str, kb_ids: list[str] | None = None, top_n: int = 12, doc_scope: list[str] | None = None, keywords: str = "", retrieval_query: str = "", use_compiled: bool = False
) -> dict:
    target_ids = kb_ids or list(dict.fromkeys(tools.kb_ids + [kb.id for kb in tools.sql_kbs]))
    if not target_ids:
        return {"chunks": [], "doc_aggs": []}
    if hasattr(tools, "scoped_doc_ids"):
        doc_scope = tools.scoped_doc_ids(doc_scope)
    _LOG.info(f'[Hybrid search] Searching the knowledge base for "{query}" (keywords: {keywords})')

    # Query expansion: append the entity-weighted ``retrieval_query`` (entity
    # terms repeated so BM25 weights them up inside the same query) — the plain
    # keyword union or the compact formalize keywords fall back when none is
    # supplied. ``keywords`` is always used only to narrow retrieved chunks.
    if retrieval_query:
        effective_query = f"{query} {retrieval_query}".strip()[:400]
    else:
        effective_query = f"{query} {keywords}".strip() if keywords else query

    # Per-request dedup: an identical query+scope is retrieved at most once, so
    # e.g. pre_search and a claim search asking the same question don't repeat
    # the ES round-trip, child fetch and narrowing.
    cache = getattr(tools, "search_cache", None)
    cache_key = _search_cache_key(effective_query, target_ids, top_n, doc_scope)
    if cache is not None and cache_key in cache:
        cached = cache[cache_key]
        _LOG.info(f"[Hybrid search] Already searched this — reusing the {len(cached.get('chunks', []))} passage(s) found earlier.")
        return cached

    embd_mdl = tools.embed_mdl
    vector_weight = 0.3 if embd_mdl else 0

    kbinfos = await settings.retriever.retrieval(
        effective_query,
        embd_mdl,
        tools.tenant_ids,
        target_ids,
        1,
        top_n,
        0.2,
        vector_similarity_weight=vector_weight,
        aggs=True,
        highlight=False,
        doc_ids=doc_scope,
        must_not={"exists": "compile_kwd"},  # plain retrieval = document chunks only; compiled products have their own tools
    )
    kbinfos = _normalize(kbinfos, tools.tenant_ids)
    # Preserve the RAW retrieved chunks in the central memory store BEFORE any
    # narrowing. search is cheap and the raw corpus may hold a fact the LLM's
    # report/grounded extraction later compresses away — a gap-driven grep over
    # memory recovers it without re-querying the knowledge base.
    try:
        from rag.advanced_rag.harness.memory import add as _memory_add

        _memory_add(tools, kbinfos.get("chunks", []) or [])
    except Exception:
        pass  # memory is best-effort; never fail the search over it.
    # Narrow-or-keep: if the query keywords match any chunk, keep only the
    # matching passages (shrinking the evidence handed to the LLM so a single
    # full-context call stays small and fast); if nothing matches, keep ALL
    # chunks intact so no evidence is silently dropped. This bounds per-claim
    # analysis size without losing the numeric/entity rows when keywords hit.
    kbinfos["chunks"] = _narrow_or_keep(kbinfos.get("chunks", []), keywords, "hybrid_search")
    if use_compiled and kbinfos.get("chunks"):
        _LOG.info("[Hybrid search] Compiled expansion enabled — enriching with page_index/tree/KG navigation.")
        await _expand_with_compiled(tools, query, keywords, kbinfos, doc_scope)
    if cache is not None:
        cache[cache_key] = kbinfos
    return kbinfos


async def vector_search(tools, query: str, kb_ids: list[str] | None = None, top_n: int = 12, keywords: str = "", retrieval_query: str = "", doc_scope: list[str] | None = None) -> dict:
    if not tools.embed_mdl:
        _LOG.warning("vector_search: no embed_mdl available")
        return {"chunks": [], "doc_aggs": []}

    _LOG.info(f'[Vector search] Searching by meaning for "{query}" (keywords: {keywords})')
    effective_query = f"{query} {retrieval_query}".strip()[:400] if retrieval_query else f"{query} {keywords}".strip() if keywords else query
    target_ids = kb_ids or tools.kb_ids
    if hasattr(tools, "scoped_doc_ids"):
        doc_scope = tools.scoped_doc_ids(doc_scope)
    kbinfos = await settings.retriever.retrieval(
        effective_query,
        tools.embed_mdl,
        tools.tenant_ids,
        target_ids,
        1,
        top_n,
        0.2,
        vector_similarity_weight=1.0,
        aggs=False,
        highlight=False,
        doc_ids=doc_scope,
        must_not={"exists": "compile_kwd"},
    )
    kbinfos = _normalize(kbinfos, tools.tenant_ids)
    try:
        from rag.advanced_rag.harness.memory import add as _memory_add

        _memory_add(tools, kbinfos.get("chunks", []) or [])
    except Exception:
        pass
    kbinfos["chunks"] = _narrow_or_keep(kbinfos.get("chunks", []), keywords, "Vector search")
    return kbinfos


async def bm25_search(tools, query: str, kb_ids: list[str] | None = None, top_n: int = 12, keywords: str = "", retrieval_query: str = "", doc_scope: list[str] | None = None) -> dict:
    _LOG.info(f'[BM25 search] Searching by keyword for "{query}" (keywords: {keywords})')
    target_ids = kb_ids or tools.kb_ids
    effective_query = f"{query} {retrieval_query}".strip()[:400] if retrieval_query else f"{query} {keywords}".strip() if keywords else query
    if hasattr(tools, "scoped_doc_ids"):
        doc_scope = tools.scoped_doc_ids(doc_scope)
    kbinfos = await settings.retriever.retrieval(
        effective_query,
        None,
        tools.tenant_ids,
        target_ids,
        1,
        top_n,
        0.0,
        vector_similarity_weight=0,
        aggs=False,
        highlight=False,
        doc_ids=doc_scope,
        must_not={"exists": "compile_kwd"},
    )
    kbinfos = _normalize(kbinfos, tools.tenant_ids)
    try:
        from rag.advanced_rag.harness.memory import add as _memory_add

        _memory_add(tools, kbinfos.get("chunks", []) or [])
    except Exception:
        pass
    kbinfos["chunks"] = _narrow_or_keep(kbinfos.get("chunks", []), keywords, "BM25 search")
    return kbinfos


# ─── Compiled product expansion (zero-LLM, used by hybrid_search with use_compiled=True) ───


async def _expand_with_compiled(tools, query: str, keywords: str, kbinfos: dict, doc_scope: list[str] | None = None) -> None:
    """Zero-LLM compiled-product expansion: page_index → tree → KG.

    For each bound KB, searches compiled entity rows matching the query,
    hops 1-hop via relations to find neighbour entities, then appends
    their source passages to ``kbinfos["chunks"]``.
    """
    before = len(kbinfos.get("chunks", []))
    seen_ids = {c.get("chunk_id") or c.get("id") for c in kbinfos.get("chunks", [])}

    scopes = await _kg_scopes(tools, doc_scope)
    if not scopes:
        return

    for kb_id, tenant_id, doc_ids in scopes:
        # 1-hop entity-graph expansion per template kind.
        # Each template writes entity/relation rows tagged with
        # ``compilation_template_kind_kwd`` — search them independently.
        for label, template_kind in (
            ("knowledge_graph", "knowledge_graph"),
            ("mind_map", "mind_map"),
            ("timeline", "timeline"),
            ("page_index", "page_index"),
        ):
            chunks = await _expand_compiled_strategy(
                tools,
                kb_id,
                tenant_id,
                doc_ids,
                query,
                seen_ids,
                template_kind=template_kind,
                max_chunks=5,
            )
            if chunks:
                kbinfos.setdefault("chunks", []).extend(chunks)
                _LOG.debug("[Compiled expand] %s: +%d chunks", label, len(chunks))

        # Tree structure graph (uses ``compile_kwd``, not template kind).
        chunks = await _expand_compiled_strategy(
            tools,
            kb_id,
            tenant_id,
            doc_ids,
            query,
            seen_ids,
            compile_kwd="tree",
            max_chunks=5,
        )
        if chunks:
            kbinfos.setdefault("chunks", []).extend(chunks)
            _LOG.debug("[Compiled expand] tree: +%d chunks", len(chunks))

        # Synthesis pages — standalone rendered articles from wiki / session
        # graph / session essence templates.  Searched directly (no entity-graph nav).
        for label, ckwd in (
            ("wiki_page", "wiki_page"),
            ("artifact_page", "artifact_page"),
            ("essence", "essence"),
        ):
            chunks = await _expand_wiki_page_strategy(
                tools,
                kb_id,
                tenant_id,
                doc_ids,
                query,
                seen_ids,
                compile_kwd=ckwd,
                max_chunks=5,
            )
            if chunks:
                kbinfos.setdefault("chunks", []).extend(chunks)
                _LOG.debug("[Compiled expand] %s: +%d chunks", label, len(chunks))

    # Re-sort so compiled-expansion chunks blend by similarity with regular ones.
    chunks = kbinfos.get("chunks", [])
    if chunks:
        chunks.sort(key=lambda c: c.get("similarity", 0.0), reverse=True)

    after = len(chunks)
    _LOG.info("[Hybrid search] Compiled expansion added %d chunks.", after - before)


async def _search_compiled_rows(
    tools,
    kb_id: str,
    tenant_id: str,
    doc_ids: list[str] | None,
    kind: str,
    *,
    text: str = "",
    top_n: int = 8,
    extra: dict | None = None,
    compile_kwd: str | None = None,
    template_kind: str | None = None,
) -> dict:
    """Search compiled KG rows in one KB, returning raw field maps.

    *compile_kwd* filters by the ``compile_kwd`` field (e.g. "tree" for tree
    structure nodes).  *template_kind* filters by ``compilation_template_kind_kwd``
    (e.g. "knowledge_graph", "mind_map").  Leave both ``None`` to scan all rows.
    """
    from common import settings
    from common.doc_store.doc_store_base import MatchTextExpr, OrderByExpr
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search

    condition: dict = {"knowledge_graph_kwd": [kind]}
    if compile_kwd:
        condition["compile_kwd"] = compile_kwd
    if template_kind:
        condition["compilation_template_kind_kwd"] = template_kind
    if doc_ids:
        condition["doc_id"] = list(doc_ids)
    if extra:
        condition.update(extra)

    fields = [
        "content_with_weight",
        "source_chunk_ids",
        "doc_id",
        "docnm_kwd",
        "from_entity_kwd",
        "to_entity_kwd",
        "name_kwd",
    ]
    exprs = []
    if text:
        embd_mdl = getattr(tools, "embed_mdl", None)
        if embd_mdl:
            try:
                exprs.append(await settings.retriever.get_vector(text, embd_mdl, top_n, 0.1))
            except Exception:
                _LOG.exception("[Compiled expand] vector build failed; using keyword match")
        if not exprs:
            exprs.append(
                MatchTextExpr(
                    ["content_ltks", "content_sm_ltks"],
                    text,
                    top_n,
                )
            )

    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields,
            [],
            condition,
            exprs,
            OrderByExpr(),
            0,
            top_n,
            search.index_name(tenant_id),
            [kb_id],
        )
        return settings.docStoreConn.get_fields(res, fields) or {}
    except Exception:
        _LOG.exception("[Compiled expand] search failed (kind=%s compile_kwd=%s)", kind, compile_kwd)
        return {}


async def _load_chunks_for_doc(tools, doc_id: str, chunk_ids: list[str]) -> list[dict]:
    """Load chunks by their IDs from the doc store."""
    if not chunk_ids:
        return []
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search

    resolved = tools._resolve_doc_tenant(doc_id)
    if not resolved:
        return []
    kb_id, tenant_id = resolved

    fields = ["content_with_weight", "docnm_kwd", "doc_id", "id"]
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields,
            [],
            {"id": list(chunk_ids)},
            [],
            OrderByExpr(),
            0,
            len(chunk_ids),
            search.index_name(tenant_id),
            [kb_id],
        )
        rows = settings.docStoreConn.get_fields(res, fields)
        if not rows:
            return []
        return [{**v, "chunk_id": k} for k, v in rows.items()]
    except Exception:
        _LOG.exception("[Compiled expand] failed to load chunks for doc_id=%s", doc_id)
        return []


async def _expand_compiled_strategy(
    tools,
    kb_id: str,
    tenant_id: str,
    doc_ids: list[str] | None,
    query: str,
    seen_ids: set[str],
    *,
    compile_kwd: str | None = None,
    template_kind: str | None = None,
    max_chunks: int = 5,
) -> list[dict]:
    """Generic 1-hop compiled expansion: entity search → relation nav → chunk load.

    1. Embedding-match seed entities (filtered by *compile_kwd* or *template_kind*).
    2. Fetch relations adjacent to seed entities (forward + backward).
    3. Collect neighbour entity names (1-hop away).
    4. Look up neighbour entities to get ``source_chunk_ids``.
    5. Load actual chunks, deduplicate, respect *max_chunks*.

    *compile_kwd* is used for structure graphs (e.g. "tree").
    *template_kind* is used for entity extraction rows (e.g. "knowledge_graph").
    """
    import json

    # -- 1. Seed entities --
    seed_rows = await _search_compiled_rows(
        tools,
        kb_id,
        tenant_id,
        doc_ids,
        "entity",
        text=query,
        top_n=5,
        compile_kwd=compile_kwd,
        template_kind=template_kind,
    )
    if not seed_rows:
        return []

    seed_names: set[str] = set()
    for r in seed_rows.values():
        try:
            payload = json.loads(r.get("content_with_weight") or "{}")
        except Exception:
            continue
        name = (payload.get("name") or payload.get("title") or "").strip()
        if name:
            seed_names.add(name)
    if not seed_names:
        return []

    # -- 2. Adjacent relations (outgoing + incoming) --
    # Provide both original and lowercased names — dataset_structure_merger
    # lowercases merged-row endpoints while per-doc rows keep original case.
    seed_list = sorted({n.lower() for n in seed_names} | seed_names)
    fwd = await _search_compiled_rows(
        tools,
        kb_id,
        tenant_id,
        doc_ids,
        "relation",
        top_n=50,
        compile_kwd=compile_kwd,
        template_kind=template_kind,
        extra={"from_entity_kwd": seed_list},
    )
    bwd = await _search_compiled_rows(
        tools,
        kb_id,
        tenant_id,
        doc_ids,
        "relation",
        top_n=50,
        compile_kwd=compile_kwd,
        template_kind=template_kind,
        extra={"to_entity_kwd": seed_list},
    )
    all_rels = {**fwd, **bwd}

    # -- 3. Neighbour names (1-hop, exclude seeds) --
    seed_lower = {n.lower() for n in seed_names}
    neighbour_names: set[str] = set()
    for r in all_rels.values():
        frm = (r.get("from_entity_kwd") or "").strip()
        frm_lower = frm.lower()
        to = (r.get("to_entity_kwd") or "").strip()
        to_lower = to.lower()
        if frm_lower in seed_lower and to and to_lower not in seed_lower:
            neighbour_names.add(to)
        if to_lower in seed_lower and frm and frm_lower not in seed_lower:
            neighbour_names.add(frm)
    if not neighbour_names:
        return []

    # -- 4. Neighbour entity source_chunk_ids --
    # Provide both original and lowercased — same as seed_list above.
    neigh_list = sorted({n.lower() for n in neighbour_names} | neighbour_names)
    if len(neigh_list) > 100:
        neigh_list = neigh_list[:100]  # reasonable cap for name_kwd search
    neigh_rows = await _search_compiled_rows(
        tools,
        kb_id,
        tenant_id,
        doc_ids,
        "entity",
        top_n=len(neigh_list),
        compile_kwd=compile_kwd,
        template_kind=template_kind,
        extra={"name_kwd": neigh_list},
    )

    # Group chunk IDs by doc
    by_doc: dict[str, set[str]] = {}
    for r in neigh_rows.values():
        doc_id = r.get("doc_id") or ""
        for cid in r.get("source_chunk_ids") or []:
            if cid and cid not in seen_ids:
                by_doc.setdefault(doc_id, set()).add(cid)

    # -- 5. Load and return --
    new_chunks: list[dict] = []
    for doc_id, cids in by_doc.items():
        if len(new_chunks) >= max_chunks:
            break
        limit = max_chunks - len(new_chunks)
        chunks = await _load_chunks_for_doc(tools, doc_id, list(cids)[:limit])
        for c in chunks:
            cid = c.get("chunk_id") or c.get("id")
            if cid and cid not in seen_ids:
                seen_ids.add(cid)
                new_chunks.append(c)

    return new_chunks


async def _search_synthesis_pages(
    tools,
    kb_id: str,
    tenant_id: str,
    doc_ids: list[str] | None,
    text: str,
    *,
    compile_kwd: str = "wiki_page",
    top_n: int = 8,
) -> dict:
    """Search synthesis-compiled page rows (no knowledge_graph_kwd filter).

    Synthesis pages are standalone articles (wiki_page, artifact_page,
    essence, etc.) with ``content_with_weight``, keyword index, and
    vector.  They do NOT carry the ``knowledge_graph_kwd`` field (unlike
    entity/relation rows from extraction).
    """
    from common import settings
    from common.doc_store.doc_store_base import MatchTextExpr, OrderByExpr
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search

    condition: dict = {"compile_kwd": compile_kwd, "available_int": 1}
    if doc_ids:
        condition["source_doc_ids"] = list(doc_ids)

    fields = [
        "content_with_weight",
        "summary_with_weight",
        "source_chunk_ids",
        "doc_id",
        "title_kwd",
        "topic_kwd",
    ]

    exprs = []
    if text:
        embd_mdl = getattr(tools, "embed_mdl", None)
        if embd_mdl:
            try:
                exprs.append(await settings.retriever.get_vector(text, embd_mdl, top_n, 0.1))
            except Exception:
                _LOG.exception("[Wiki expand] vector build failed; using keyword match")
        if not exprs:
            exprs.append(
                MatchTextExpr(
                    ["content_ltks", "content_sm_ltks"],
                    text,
                    top_n,
                )
            )

    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields,
            [],
            condition,
            exprs,
            OrderByExpr(),
            0,
            top_n,
            search.index_name(tenant_id),
            [kb_id],
        )
        return settings.docStoreConn.get_fields(res, fields) or {}
    except Exception:
        _LOG.exception("[Wiki expand] search failed for kb=%s", kb_id)
        return {}


async def _expand_wiki_page_strategy(
    tools,
    kb_id: str,
    tenant_id: str,
    doc_ids: list[str] | None,
    query: str,
    seen_ids: set[str],
    *,
    compile_kwd: str = "wiki_page",
    max_chunks: int = 5,
) -> list[dict]:
    """Expand synthesis-compiled pages: semantic search → load source chunks.

    Unlike ``_expand_compiled_strategy`` (which does 1-hop entity-graph
    navigation), synthesis pages are standalone rendered articles — we search
    them directly and load the referenced source chunks as context.

    *compile_kwd* selects which synthesis type: "wiki_page" (Wiki template),
    "artifact_page" (Session Graph synthesis), "essence" (Session Essence).
    """
    # -- 1. Search synthesis pages --
    wiki_rows = await _search_synthesis_pages(
        tools,
        kb_id,
        tenant_id,
        doc_ids,
        query,
        compile_kwd=compile_kwd,
        top_n=5,
    )
    if not wiki_rows:
        return []

    # -- 2. Collect source_chunk_ids from matching pages --
    by_doc: dict[str, set[str]] = {}
    for r in wiki_rows.values():
        doc_id = r.get("doc_id") or ""
        for cid in r.get("source_chunk_ids") or []:
            if cid and cid not in seen_ids:
                by_doc.setdefault(doc_id, set()).add(cid)

    # -- 3. Load chunks, assign high similarity for priority ranking --
    new_chunks: list[dict] = []
    for doc_id, cids in by_doc.items():
        if len(new_chunks) >= max_chunks:
            break
        limit = max_chunks - len(new_chunks)
        chunks = await _load_chunks_for_doc(tools, doc_id, list(cids)[:limit])
        for c in chunks:
            cid = c.get("chunk_id") or c.get("id")
            if cid and cid not in seen_ids:
                seen_ids.add(cid)
                c.setdefault("similarity", 0.9)  # wiki pages rank high
                new_chunks.append(c)

    return new_chunks


async def web_search(tools, query: str, keywords: str = "", retrieval_query: str = "") -> dict:
    if not tools.has_web():
        return {"chunks": [], "doc_aggs": []}

    _LOG.info(f'[Web search] Searching the web for "{query}"')
    try:
        from common.misc_utils import thread_pool_exec

        effective_query = f"{query} {retrieval_query}".strip()[:400] if retrieval_query else f"{query} {keywords}".strip() if keywords else query
        web_res = await thread_pool_exec(tools.web_search.retrieve_chunks, effective_query)
        return {"chunks": web_res.get("chunks", []), "doc_aggs": web_res.get("doc_aggs", [])}
    except Exception:
        _LOG.exception("web_search failed")
        return {"chunks": [], "doc_aggs": []}


async def structured_query(tools, query: str, keywords: str = "", kb_ids: list[str] | None = None, doc_scope: list[str] | None = None) -> dict:
    """Answer from the structured (tabular) KBs by translating the query to SQL.

    ``keywords`` is accepted for schema conformance but deliberately unused: the
    query is translated to SQL rather than keyword-matched, and the rows it
    returns are not prose to narrow.
    """
    _LOG.info(f'[Structured search] Querying the structured (table) data for "{query}"')
    sql_kbs = [kb for kb in tools.sql_kbs if kb_ids is None or kb.id in kb_ids]
    if not sql_kbs:
        return {"answer": "", "chunks": [], "doc_aggs": []}
    if hasattr(tools, "scoped_doc_ids"):
        doc_scope = tools.scoped_doc_ids(doc_scope)
    from api.db.services.dialog_service import use_sql

    tenant_id = sql_kbs[0].tenant_id
    sql_kb_ids = [kb.id for kb in sql_kbs]
    try:
        ans = await use_sql(query, tools.field_map, tenant_id, tools.chat_mdl, quota=True, kb_ids=sql_kb_ids, doc_ids=doc_scope)
    except Exception:
        _LOG.exception("structured_query failed")
        return {"answer": "", "chunks": [], "doc_aggs": []}
    if not ans:
        return {"answer": "", "chunks": [], "doc_aggs": []}
    ref = ans.get("reference") or {}
    return {
        "answer": ans.get("answer", "") or "",
        "chunks": ref.get("chunks") or [],
        "doc_aggs": ref.get("doc_aggs") or [],
    }


# ─── Grep-style exact search (BM25 candidate pool + regex locate), and ───
# ─── list_chunks (deep-read a document) — used by the sub-agent so it can   ───
# ─── do exact keyword/pattern locate + full-document deep-read like dynamic. ───

_GREP_TERMS_MAX = 10
_GREP_OUT_CHARS_PER_CHUNK = 700
_GREP_OUT_TOTAL_CHARS = 8000
_LIST_CHUNKS_MAX_CHUNKS = 80


def _grep_terms_from_query(query: str, max_terms: int = _GREP_TERMS_MAX) -> list[str]:
    """Extract compact grep terms from a query: bare alnum words of length>=2,
    deduped (order-preserving) and capped. Numbers/ids are preserved as-is."""
    if not query:
        return []
    terms: list[str] = []
    seen: set[str] = set()
    for m in re.findall(r"[A-Za-z0-9][A-Za-z0-9_.\-]{1,}", query):
        t = m.strip("._-")
        if len(t) < 2:
            continue
        low = t.lower()
        if low in seen:
            continue
        seen.add(low)
        terms.append(t)
        if len(terms) >= max_terms:
            break
    return terms


async def grep_search(
    tools,
    query: str,
    kb_ids: list[str] | None = None,
    top_n: int = 60,
    doc_scope: list[str] | None = None,
) -> dict:
    """Exact keyword/pattern locate: BM25-first candidate pool, then a
    case-insensitive regex locate with a short context window (like dynamic's
    grep_chunks, but returning the ``Pipeline`` ``{"chunks": [...]}`` contract).

    Returns compact snippet chunks (token-cheap) for the sub-agent. When grep
    matches nothing, the BM25 candidates are returned unchanged so evidence is
    never dropped (enumeration / multi-hop answers must not lose candidates).
    """
    from rag.advanced_rag.harness.grep_sed_narrow import narrow_by_terms

    _LOG.info('[Grep search] Keyword-first locate for "%s"', query)
    if not query or not str(query).strip():
        return {"chunks": [], "doc_aggs": []}
    res = await bm25_search(tools, query=str(query).strip(), kb_ids=kb_ids, top_n=top_n, doc_scope=doc_scope)
    chunks = res.get("chunks", []) or []
    terms = _grep_terms_from_query(str(query).strip())
    if not chunks or not terms:
        return res
    try:
        out = narrow_by_terms(
            chunks,
            terms,
            keywords=str(query).strip(),
            context={"before": 1, "after": 0},
            max_out_chars_per_chunk=_GREP_OUT_CHARS_PER_CHUNK,
            max_out_total_chars=_GREP_OUT_TOTAL_CHARS,
        )
        kept = out.get("kept") or []
        if kept:
            _LOG.info(
                "[Grep search] narrowed %d->%d chunk(s), %.1fK chars.",
                len(chunks),
                len(kept),
                sum(len(str(c.get("content_with_weight") or c.get("content") or "")) for c in kept) / 1000.0,
            )
            res["chunks"] = kept
    except Exception:
        _LOG.exception("[Grep search] narrow failed; using raw BM25 candidates.")
    return res


async def list_chunks(tools, doc_id: str) -> dict:
    """Deep-read a document: return its COMPLETE chunk list in reading order.

    The sub-agent calls this after grep_search / hybrid_search hit a document
    when it needs the full text (enumeration / count / arithmetic answers), like
    dynamic's ``list_chunks`` tool. Returns ``{"chunks": [...], "doc_aggs": [...]}``.
    """
    if not doc_id or not str(doc_id).strip():
        return {"chunks": [], "doc_aggs": []}
    if not callable(getattr(tools, "fetch_full_document", None)):
        return {"chunks": [], "doc_aggs": []}
    _LOG.info("[List chunks] Deep-reading document %s", doc_id)
    try:
        full = await tools.fetch_full_document(str(doc_id).strip())
    except Exception:
        _LOG.exception("[List chunks] fetch_full_document failed doc=%s", doc_id)
        return {"chunks": [], "doc_aggs": []}
    chunks = (full.get("chunks") or [])[:_LIST_CHUNKS_MAX_CHUNKS]
    return {"chunks": chunks, "doc_aggs": full.get("doc_aggs") or []}


# ── Rag-agent tool set (migrated from harness/dynamic) ──
_NAV_TREE_MAX_DOCS = 8

# Cap on datasets scanned per tool call (avoids fan-out over too many KBs).
_NAV_TREE_MAX_DATASETS = 10

# Chars of a chunk's text returned by search_chunks in default (snippet) mode.
_SEARCH_SNIPPET_CHARS = 300

# ── XML helpers (uniform tag vocabulary shared by all retrieval tools) ──


def _xml_escape(value: Any) -> str:
    s = "" if value is None else str(value)
    return s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;").replace('"', "&quot;")


def _chunk_text(c: dict) -> str:
    return str(c.get("content_with_weight") or c.get("content") or c.get("text") or "")


def _chunk_attr(c: dict, keys: tuple[str, ...]) -> str:
    for k in keys:
        v = c.get(k)
        if v not in (None, ""):
            return str(v)
    return ""


def _doc_id(c: dict) -> str:
    return _chunk_attr(c, ("doc_id", "docid", "document_id"))


def _dataset_id(c: dict) -> str:
    return _chunk_attr(c, ("dataset_id", "kb_id", "knowledgebase_id"))


def _doc_title(c: dict) -> str:
    return _chunk_attr(c, ("docnm_kwd", "doc_title", "title", "document_name"))


def _chunk_id(c: dict) -> str:
    return _chunk_attr(c, ("chunk_id", "id"))


def _snippet(s: str, n: int) -> str:
    """Truncate a string to ``n`` chars on a char boundary with an ellipsis."""
    s = (s or "").strip()
    if len(s) <= n:
        return s
    return s[:n].rstrip() + "..."


# ── Tool: think ──


@tool
async def _think_impl(
    thought: str,
    next_thought_needed: bool = True,
    thought_number: int | None = None,
    total_thoughts: int | None = None,
    is_revision: bool = False,
    revises_thought: int | None = None,
    branch_from_thought: int | None = None,
    branch_id: str = "",
    needs_more_thoughts: bool = False,
) -> str:
    """A stateless reflective-thinking tool that records a reasoning step.

    Use it to plan, reflect on retrieved content, and reason step by step
    before answering. It does not persist history — keep your plan in the
    conversation context. When your thinking is complete, deliver your answer
    by writing it as your plain reply and stopping (no further tool calls).
    NEVER include the final answer directly in a thought.

    IMPORTANT: The ``thought`` field must be plain natural-language reasoning
    ONLY. Do NOT embed XML tags, ``<invoke ...>``, tool names, JSON, or any
    pseudo tool-call syntax inside it. To call another tool (e.g. search_chunks),
    emit a separate tool call for that tool instead of describing it in text
    inside ``thought``.

    All arguments are optional for robustness: if the model omits the counters,
    the tool records the thought and, by default, signals that thinking continues
    (so the model keeps retrieving rather than stopping early).

    :param thought: Your current thinking step, in natural user-friendly
        language. Focus on WHAT you're trying to find and WHY, not HOW.
    :param next_thought_needed: True if you need more thinking. Defaults to True
        (conservative: keep thinking) when omitted.
    :param thought_number: Current thought number (>= 1). Optional.
    :param total_thoughts: Current estimate of thoughts needed (>= 1). Optional.
    :param is_revision: Whether this thought revises previous thinking.
    :param revises_thought: If a revision, which thought number is reconsidered.
    :param branch_from_thought: If branching, which thought number is the branch point.
    :param branch_id: Identifier for the current branch (if any).
    :param needs_more_thoughts: If reaching the end but realising more thoughts are needed.
    :return: A completion marker; does not affect the conversation content.
    """
    if not thought or not str(thought).strip():
        return "Thought process invalid: thought must be a non-empty string"
    if thought_number is not None and thought_number < 1:
        return "Thought process invalid: thought_number must be >= 1"
    if total_thoughts is not None and total_thoughts < 1:
        return "Thought process invalid: total_thoughts must be >= 1"
    # Conservative by default: without explicit counters we prefer to keep going
    # rather than stop early (avoids partial answers on multi-hop questions).
    if thought_number is None or total_thoughts is None:
        incomplete = bool(next_thought_needed or needs_more_thoughts)
    else:
        incomplete = bool(next_thought_needed or needs_more_thoughts or thought_number < total_thoughts)
    if incomplete:
        return "Thought process recorded - unfinished steps remain, continue exploring and calling tools"
    return "Thought process recorded"


# ── Tool: todo_write ──


@tool
async def _todo_write_impl(task: str = "", steps: list | None = None) -> str:
    """Create and manage a structured task list for retrieval and research tasks.

    Use it to track progress across 3+ retrieval/research steps. It is
    stateless: the model maintains the plan within the conversation context.

    CRITICAL - Focus on Retrieval Tasks Only: track RETRIEVAL and RESEARCH
    tasks (searching datasets, retrieving documents, gathering information).
    Do NOT include summary or synthesis tasks here — those are for the think
    tool. Examples: "Search for X in dataset", "Retrieve information about Y".
    Exclude: "Summarize findings", "Generate final answer".

    :param task: Short task title.
    :param steps: List of {"id","description","status"} where status is one of
        "pending" / "in_progress" / "completed". Limit to ONE in_progress at a
        time.
    :return: The current task list echoed back.
    """
    entries = steps if isinstance(steps, list) else []
    rendered = [str(task).strip()] if (task or "").strip() else []
    for i, s in enumerate(entries):
        if not isinstance(s, dict):
            continue
        sid = str(s.get("id") or i + 1)
        desc = str(s.get("description") or "").strip()
        status = str(s.get("status") or "pending")
        rendered.append(f"  [{status}] #{sid}: {desc}")
    if not rendered:
        return "No tasks recorded."
    return "\n".join(rendered)


# ── Tool: grep_chunks ──


@tool(timeout=120)
async def _grep_chunks_impl(
    query: str,
    chunk_ids: list[str] | None = None,
    dataset_ids: list[str] | None = None,
    doc_scope: list[str] | None = None,
    tools=None,
) -> str:
    """Search dataset chunk content with a single POSIX regular expression,
    case-insensitive (behaves like grep -E -i).

    Pack multiple concepts into ONE regex using | alternation — do not call
    this tool repeatedly for synonyms. Keep the regex BROAD: use bare keywords
    and names combined with |, and do NOT anchor it to a specific subject/verb
    chain (e.g. prefer "何进|董太后|鸩杀|斩|诛" over "何进.*斩").

    When navigate_structure returned an outline with ``[chunks: c100,c101]``
    pointers, pass those chunk_ids here to grep ONLY within those chunks (precise
    locate inside a document's compiled-structure spans) instead of the whole
    dataset. doc_scope must then carry the owning document id(s).

    Returns an XML <grep_results> document. Each matching chunk is a <chunk>
    element with chunk_id, doc_id (owning document id), doc_title, dataset_id,
    page_num, chunk_index and score attributes, and a <match_snippet> element —
    a SHORT window (~80 chars on each side of the first match), NOT the full
    chunk text. The snippet is for fast relevance judgement only. To read a
    located document's complete text, call list_chunks with the returned doc_id.

    :param query: REQUIRED: a single POSIX regex applied to chunk content
        (case-insensitive). Combine multiple concepts with | in ONE regex.
    :param chunk_ids: Optional list of chunk ids to restrict the grep to (from a
        navigate_structure outline's ``[chunks: ...]`` pointers). When given,
        grep runs only inside those chunks of doc_scope's documents.
    :param dataset_ids: Optional dataset ids to restrict the search to (at most
        10). When omitted, the conversation's bound datasets are used.
    :param doc_scope: Optional document ids to restrict the search to (at most 10).
    :return: XML <grep_results> document.
    """
    if not query or not str(query).strip():
        return '<grep_results count="0">\n</grep_results>'
    q = str(query).strip()
    # Validate the regex up front so a malformed pattern is reported, not silently skipped.
    try:
        re.compile("(?i)" + q)
    except re.error as e:
        return f'<grep_results count="0" error="invalid regex: {_xml_escape(str(e))}">\n</grep_results>'

    from common import settings

    if not getattr(settings, "retriever", None):
        return '<grep_results count="0" error="no retriever">\n</grep_results>'

    _t = tools or _tools_slot()
    target_ids = dataset_ids or list(dict.fromkeys(_get_kb_ids(_t)))
    if not target_ids:
        return '<grep_results count="0" error="no bound datasets">\n</grep_results>'

    # Precise path: grep ONLY within the chunk_ids flagged by navigate_structure's
    # outline pointers (inside doc_scope's documents). No full-dataset retrieval.
    if chunk_ids:
        candidates = await _load_specific_chunks(_t, chunk_ids, doc_scope)
        if not candidates:
            return '<grep_results count="0" error="none of the given chunk_ids found">\n</grep_results>'
    else:
        # Candidate pool: keyword-first retrieval (BM25) gives the widest recall
        # for exact identifiers / names / error codes, then narrow_by_terms does
        # the actual regex locate + context-window extraction (zero extra LLM).
        candidates = []
        try:
            res = await bm25_search(_t, q, kb_ids=target_ids, top_n=60, doc_scope=doc_scope)
            candidates = res.get("chunks", []) or []
        except Exception:
            _LOG.exception("[grep_chunks] bm25 candidate retrieval failed; falling back to hybrid")
            try:
                res = await hybrid_search(_t, q, kb_ids=target_ids, top_n=60, doc_scope=doc_scope)
                candidates = res.get("chunks", []) or []
            except Exception:
                _LOG.exception("[grep_chunks] hybrid fallback failed")
                return '<grep_results count="0" error="retrieval failed">\n</grep_results>'

    terms = _query_to_terms(q)
    regex = re.compile("(?i)" + q)
    if chunk_ids:
        # Strict precise grep: only chunks whose text actually matches the regex
        # are reported. (narrow_by_terms may otherwise keep non-matching context
        # chunks, which is wrong when the scope is exactly the flagged chunks.)
        matched = [c for c in candidates if regex.search(_chunk_text(c))]
    else:
        from rag.advanced_rag.harness.grep_sed_narrow import narrow_by_terms

        narrowed = narrow_by_terms(
            candidates,
            terms,
            fallback_terms=None,
            context={"before": 0, "after": 1},
            keywords=q,
            max_out_chars_per_chunk=1200,
            max_out_total_chars=16000,
        )
        matched = narrowed.get("kept", candidates)

    parts = [f'<grep_results count="{len(matched)}" query="{_xml_escape(q)}">']
    for i, c in enumerate(matched):
        cid = _chunk_id(c)
        did = _doc_id(c)
        dsid = _dataset_id(c)
        title = _doc_title(c)
        score = c.get("similarity", 0.0) or c.get("score", 0.0) or 0.0
        parts.append(f'  <chunk rank="{i + 1}" chunk_id="{_xml_escape(cid)}" doc_id="{_xml_escape(did)}" dataset_id="{_xml_escape(dsid)}" doc_title="{_xml_escape(title)}" score="{float(score):.3f}">')
        snippet = _chunk_text(c)
        if len(snippet) > 240:
            snippet = snippet[:240]
        if snippet:
            parts.append(f"    <match_snippet>{_xml_escape(snippet)}</match_snippet>")
        parts.append("  </chunk>")
    parts.append("</grep_results>")
    return "\n".join(parts)


# ── Tool: search_chunks ──


@tool(timeout=120)
async def _search_chunks_impl(
    queries: list[str],
    chunk_ids: list[str] | None = None,
    dataset_ids: list[str] | None = None,
    doc_scope: list[str] | None = None,
    top_n: int = 8,
    full_content: bool = False,
    similarity_threshold: float = 0.2,
    keywords_similarity_weight: float = 0.3,
    tools=None,
) -> str:
    """Semantic/vector search tool for retrieving knowledge by meaning, intent,
    and conceptual relevance.

    This tool uses embeddings to understand the query and find semantically
    similar content across dataset chunks. It searches by MEANING rather than
    exact text. Do NOT use it for exact keyword matching (use grep_chunks for
    that); do NOT pass long raw text or user messages as queries.

    When navigate_structure returned an outline with ``[chunks: c100,c101]``
    pointers, pass those chunk_ids here to search ONLY within those chunks
    (precise locate inside a document's compiled-structure spans). doc_scope must
    then carry the owning document id(s).

    By default returns a COMPACT snippet per hit (token-cheap) so you can judge
    relevance. When you need the full original chunk text, read it precisely with
    list_chunks (pass the doc_id + chunk_id), or set ``full_content=True`` only if
    you must have the full text in one call.

    Returns an XML <search_results> document. Each hit is a <chunk> element with
    attributes rank, chunk_id, doc_id, page_num, chunk_index, dataset_id,
    doc_title and score, and a <content> element carrying either a short snippet
    (default) or the FULL chunk text (full_content=True).

    :param queries: REQUIRED: 1-5 short, well-formed semantic questions or
        conceptual statements.
    :param chunk_ids: Optional list of chunk ids to restrict the search to (from
        a navigate_structure outline's ``[chunks: ...]`` pointers).
    :param dataset_ids: Optional dataset ids to restrict the search.
    :param doc_scope: Optional document ids to restrict the search.
    :param top_n: Per-query result count, default 8 (kept small to avoid flooding
        the history with large chunks).
    :param full_content: When True, return the FULL chunk text; default False
        returns a compact snippet per hit.
    :param similarity_threshold: Similarity threshold, default 0.2.
    :param keywords_similarity_weight: Keyword-vs-vector weight, default 0.3.
    :return: XML <search_results> document.
    """
    from common import settings

    if not getattr(settings, "retriever", None):
        return '<search_results count="0" error="no retriever">\n</search_results>'

    if not isinstance(queries, list):
        queries = [queries]
    queries = [str(q).strip() for q in queries if str(q).strip()]
    if not queries or len(queries) > 5:
        queries = queries[:5]
    if not queries:
        return '<search_results count="0" error="no queries">\n</search_results>'

    tools_slot = tools or _tools_slot()
    target_ids = dataset_ids or list(dict.fromkeys(_get_kb_ids(tools_slot)))
    if not target_ids:
        return '<search_results count="0" error="no bound datasets">\n</search_results>'

    # Precise path: search ONLY within the chunk_ids flagged by navigate_structure
    # outline pointers (inside doc_scope's documents). Rank by keyword overlap
    # (zero LLM) — the scope is small, so full hybrid search is unnecessary.
    if chunk_ids:
        candidates = await _load_specific_chunks(tools_slot, chunk_ids, doc_scope)
        if not candidates:
            return '<search_results count="0" error="none of the given chunk_ids found">\n</search_results>'
        ranked = _rank_chunks_by_terms(candidates, queries)
        merged = ranked[: max(1, int(top_n or 12)) * 2]
    else:
        seen: set[str] = set()
        merged: list[dict] = []
        for q in queries:
            try:
                res = await hybrid_search(
                    tools_slot,
                    q,
                    kb_ids=target_ids,
                    top_n=max(1, int(top_n or 12)),
                    doc_scope=doc_scope,
                )
            except Exception:
                _LOG.exception("[search_chunks] hybrid_search failed for %r", q)
                continue
            for c in res.get("chunks", []) or []:
                k = _chunk_id(c)
                if k and k in seen:
                    continue
                if k:
                    seen.add(k)
                merged.append(c)

    # After the semantic hits, enrich with RELATED chunks discovered via each hit
    # document's COMPILED structure (zero chat-LLM, vector beam drill-down). For the
    # docs that surfaced, follow the tree/catalog hierarchy to OTHER chunks related
    # to the same query and append their snippets, so a search that lands on one
    # spot in a document also surfaces the surrounding sub-sections that answer it.
    # gated to high/ultra (the same modes that bind navigate_tree/navigate_structure)
    # so medium keeps its plain hybrid behavior.
    mode = str(getattr(tools_slot, "thinking_mode", "") or "").strip().lower()
    if merged and mode in _COMPILED_TOOL_MODES:
        hit_docs = [d for d in dict.fromkeys(_doc_id(c) for c in merged) if d]
        if hit_docs:
            for q in queries:
                related = await _expand_related_via_structure(tools_slot, q, hit_docs, seen)
                merged.extend(related)

    # Cap total chunks returned to keep the tool token-cheap (default snippet mode).
    merged = merged[: max(1, int(top_n or 8))]

    parts = [f'<search_results count="{len(merged)}">']
    for i, c in enumerate(merged):
        cid = _chunk_id(c)
        did = _doc_id(c)
        dsid = _dataset_id(c)
        title = _doc_title(c)
        page = c.get("page_num", 0) or c.get("page", 0) or 0
        cidx = c.get("chunk_index", 0) or 0
        score = c.get("similarity", 0.0) or c.get("score", 0.0) or 0.0
        related = ' related="true"' if c.get("related_via_structure") else ""
        parts.append(
            f'  <chunk rank="{i + 1}" chunk_id="{_xml_escape(cid)}" doc_id="{_xml_escape(did)}" '
            f'page_num="{page}" chunk_index="{cidx}" dataset_id="{_xml_escape(dsid)}" '
            f'doc_title="{_xml_escape(title)}" score="{float(score):.3f}"{related}>'
        )
        content = _chunk_text(c)
        if content:
            if full_content:
                parts.append(f"    <content>{_xml_escape(content)}</content>")
            else:
                parts.append(f"    <content>{_xml_escape(_snippet(content, _SEARCH_SNIPPET_CHARS))}</content>")
        parts.append("  </chunk>")
    parts.append("</search_results>")
    return "\n".join(parts)


# ── Tool: list_chunks ──


@tool(timeout=120)
async def _list_chunks_impl(doc_id: str, chunk_ids: list[str] | None = None, limit: int = 20, offset: int = 0, tools=None) -> str:
    """Read the FULL original text of ONE dataset document in reading order
    (Deep Read).

    Use this AFTER grep_chunks / search_chunks / navigate_structure locate a
    document: pass its doc_id to read the complete chunk text — including
    surrounding context that grep snippets omit. When navigate_structure returned
    an outline with ``[chunks: c100,c101]`` pointers, pass those chunk_ids here to
    read ONLY those chunks (fast, precise deep-read) instead of the whole document.

    Returns an XML <chunks> document with doc_id, offset, limit and fetched
    attributes. Each chunk is a <chunk> element carrying chunk_id, doc_id,
    page_num, chunk_index, dataset_id, doc_title and a <content> element with
    the FULL original chunk text, ordered by reading order. A
    <pagination next_offset=.../> element signals that more pages remain.

    :param doc_id: REQUIRED: the document id to read (from grep_chunks /
        search_chunks / navigate_structure output).
    :param chunk_ids: Optional list of specific chunk ids to read (from a
        navigate_structure outline's ``[chunks: ...]`` pointers). When given, only
        those chunks are returned and limit/offset are ignored.
    :param limit: Page size, default 20, max 100.
    :param offset: Page offset, default 0.
    :return: XML <chunks> document.
    """
    from common import settings

    if not getattr(settings, "retriever", None):
        return f'<chunks doc_id="{_xml_escape(doc_id)}" error="no retriever">\n</chunks>'
    doc_id = str(doc_id or "").strip()
    if not doc_id:
        return '<chunks doc_id="" error="doc_id is required">\n</chunks>'
    limit = max(1, min(int(limit or 20), 100))
    offset = max(0, int(offset or 0))

    tools_slot = tools or _tools_slot()
    try:
        full = await tools_slot.fetch_full_document(doc_id)
    except Exception:
        _LOG.exception("[list_chunks] fetch_full_document failed for doc_id=%s", doc_id)
        return f'<chunks doc_id="{_xml_escape(doc_id)}" offset="{offset}" limit="{limit}" fetched="0">\n</chunks>'

    chunks = full.get("chunks", []) or []

    # Precise deep-read: only the requested chunk ids (from navigate_structure
    # outline pointers). Ignores limit/offset.
    if chunk_ids:
        wanted = [str(c).strip() for c in chunk_ids if str(c).strip()]
        by_id = {(_chunk_id(c) or ""): c for c in chunks}
        page = [by_id[cid] for cid in wanted if cid in by_id]
        if not page:
            return f'<chunks doc_id="{_xml_escape(doc_id)}" chunk_ids="{_xml_escape(",".join(wanted))}" fetched="0">\n</chunks>'
        parts = [f'<chunks doc_id="{_xml_escape(doc_id)}" chunk_ids="{_xml_escape(",".join(wanted))}" fetched="{len(page)}">']
        for c in page:
            parts.append(_render_chunk_block(c, doc_id))
        parts.append("</chunks>")
        return "\n".join(parts)

    page = chunks[offset : offset + limit]

    parts = [f'<chunks doc_id="{_xml_escape(doc_id)}" offset="{offset}" limit="{limit}" fetched="{len(page)}">']
    for c in page:
        parts.append(_render_chunk_block(c, doc_id))
    if len(page) == limit and (offset + limit) < len(chunks):
        parts.append(f'  <pagination next_offset="{offset + limit}" remaining="true" />')
    parts.append("</chunks>")
    return "\n".join(parts)


# ── Tool: calculate (safe numeric arithmetic over retrieved facts) ──


@tool(timeout=30)
async def _calculate_impl(expression: str) -> str:
    """Safely evaluate ONE Python arithmetic expression to compute a number the
    retrieved facts imply but no source states outright (an age, an age
    difference, a percentage, a difference of speeds after unit conversion, a
    rate times a count).

    Use it for numeric answers. When the answer needs arithmetic (e.g. "how
    much younger", "what percentage", "difference in m/s", "total dollars over
    a fast"), write ONE Python expression with every figure as a literal pulled
    EXACTLY from the retrieved facts, and let this tool compute it precisely —
    do NOT do the arithmetic by hand in prose (LLMs get it wrong often enough
    to matter). The expression is evaluated in a sandbox: only plain arithmetic
    on numbers is allowed, plus the functions abs, round, min, max, sum, len,
    int, float, sorted, letters, digit_sum and date_diff. No variables, imports,
    attributes, subscripts, or non-numeric operations.

    Examples:
    - age difference -> 2010 - 1971
    - percentage      -> 100 * 2.7 / 556
    - speed difference in m/s (km/h ÷ 3.6) -> 132 / 3.6 - 50 / 21.07
    - rate × count    -> 25 * 49
    - days between two dates -> date_diff("1941-07-28", "1959-07-17")
    - letters in names -> letters("Ada Lovelace", "Alan Turing")

    :param expression: A single Python arithmetic expression (numbers + operators
        + the whitelisted functions above). Every figure must come from the
        retrieved evidence — never invent or round inputs.
    :return: XML <calculation> document with the exact computed value, or an
        <error> describing why the expression was refused.
    """
    from rag.advanced_rag.harness.arithmetic import compute

    expr = str(expression or "").strip()
    if not expr:
        return '<calculation error="empty expression">\n</calculation>'
    value, err = compute(expr)
    if err:
        return f'<calculation error="{_xml_escape(err)}">\n</calculation>'
    return f'<calculation value="{_xml_escape(value)}" expression="{_xml_escape(expr)}">\n</calculation>'


def _render_chunk_block(c: dict, doc_id: str) -> str:
    """Render one chunk as an XML <chunk> block with full <content>."""
    cid = _chunk_id(c)
    dsid = _dataset_id(c)
    title = _doc_title(c)
    page_num = c.get("page_num", 0) or c.get("page", 0) or 0
    cidx = c.get("chunk_index", 0) or 0
    lines = [f'  <chunk chunk_id="{_xml_escape(cid)}" doc_id="{_xml_escape(doc_id)}" page_num="{page_num}" chunk_index="{cidx}" dataset_id="{_xml_escape(dsid)}" doc_title="{_xml_escape(title)}">']
    content = _chunk_text(c)
    if content:
        lines.append(f"    <content>{_xml_escape(content)}</content>")
    lines.append("  </chunk>")
    return "\n".join(lines)


async def _load_specific_chunks(tools_slot, chunk_ids: list[str], doc_scope: list[str] | None = None) -> list[dict]:
    """Load specific chunks (from navigate_structure outline pointers) by id.

    Fetches each ``doc_scope`` document's full chunk list and filters to the
    requested ``chunk_ids``. When ``doc_scope`` is omitted, scans all bound
    documents (expensive) — so prefer passing doc_scope with the owning doc(s).
    Zero LLM.
    """
    wanted = {str(c).strip() for c in chunk_ids if str(c).strip()}
    if not wanted:
        return []
    if not doc_scope:
        # Derive candidate docs from the tools' kb_ids' documents is not available
        # here; require doc_scope for the precise path.
        return []
    found: list[dict] = []
    seen: set[str] = set()
    for doc_id in doc_scope[:8]:
        try:
            full = await tools_slot.fetch_full_document(doc_id)
        except Exception:
            _LOG.exception("[grep_chunks] fetch_full_document failed for doc_id=%s", doc_id)
            continue
        for c in full.get("chunks", []) or []:
            cid = _chunk_id(c)
            if cid in wanted and cid not in seen:
                seen.add(cid)
                found.append(c)
    return found


def _rank_chunks_by_terms(candidates: list[dict], queries: list[str]) -> list[dict]:
    """Rank candidate chunks by how many query terms overlap with their text.

    Zero-LLM keyword relevance for the precise ``chunk_ids`` search path (the
    scope is a handful of chunks, so a cheap overlap score suffices). Returns
    chunks sorted most-relevant-first.
    """
    # Collect significant terms from all queries.
    terms: list[str] = []
    for q in queries:
        for tok in re.findall(r"[A-Za-z0-9_]{2,}", (q or "").lower()):
            if tok not in terms:
                terms.append(tok)
    if not terms:
        return list(candidates)
    scored = []
    for c in candidates:
        text = _chunk_text(c).lower()
        hits = sum(1 for t in terms if t in text)
        if hits:
            scored.append((hits, c))
    scored.sort(key=lambda x: x[0], reverse=True)
    return [c for _, c in scored]


# The dynamic runner sets this module slot to the active RAGTools instance for
# the current request, so the tools above (defined once at import) read the
# request-scoped retrieval context.
_tools_ref: dict[str, Any] = {}


def _tools_slot():
    return _tools_ref.get("tools")


def _get_kb_ids(tools_slot) -> list[str]:
    if tools_slot is None:
        return []
    ids = getattr(tools_slot, "kb_ids", None) or []
    return list(ids)


def _query_to_terms(query: str) -> list[str]:
    """Derive plain grep terms from a regex query for narrow_by_terms.

    Splits the alternation into its constituents (capped) so the regex locate
    and the term-based context window both fire on the same broad set of names.
    """
    q = (query or "").strip()
    if not q:
        return []
    # Strip regex anchors/quantifiers that are meaningless as grep terms.
    q = re.sub(r"^\(\?i\)", "", q)
    q = q.replace("\\b", "").replace("(?i)", "")
    parts = re.split(r"\|", q)
    terms = []
    for p in parts:
        p = re.sub(r"[.*+?^$()\[\]{}]", " ", p).strip()
        if not p:
            continue
        for tok in re.split(r"\s+", p):
            tok = tok.strip()
            if tok and len(tok) >= 2 and tok not in terms:
                terms.append(tok)
        if len(terms) >= 16:
            break
    return terms


def _base_chat_mdl(tools_slot):
    """Return the concrete model instance that runs the tool-calling loop.

    The chain is ``RAGTools.chat_mdl`` → ``CountingChatModel`` (proxy) →
    ``LLMBundle`` (tenant bundle) → ``mdl`` (a ``rag.llm.chat_model.Base``
    instance). We must bind and run the loop on the *innermost* ``Base`` object,
    because ``LLMBundle.bind_tools`` is the legacy ``(toolcall_session, tools)``
    style (gated on ``is_tools``) and does NOT expose the decorator
    ``bind_tools(tools=[...])`` style nor ``async_chat_streamly_with_tools``.
    """
    try:
        chat_mdl = getattr(tools_slot, "chat_mdl", None)
        if chat_mdl is None:
            return None
        # CountingChatModel proxy → LLMBundle bundle.
        raw = getattr(chat_mdl, "_chat_mdl", None) or chat_mdl
        # LLMBundle → innermost Base/ChatModel instance.
        mdl = getattr(raw, "mdl", None) or raw
        if mdl is None:
            return None
        # Ensure we actually landed on a tool-calling-capable object; otherwise
        # surface the chain for debugging.
        if not callable(getattr(mdl, "bind_tools", None)) or not callable(getattr(mdl, "async_chat_streamly_with_tools", None)):
            _LOG.error(
                "[dynamic] resolved model %r lacks tool loop methods (chain: chat_mdl=%r raw=%r)",
                mdl,
                chat_mdl,
                raw,
            )
            return None
        return mdl
    except Exception:
        _LOG.exception("[dynamic] failed to resolve base chat model")
        return None


# Public tool name the model / prompt refers to, mapped from the internal
# ``@tool``-decorated function name. ``_build_openai_schema`` uses ``fn.__name__``
# (e.g. ``_grep_chunks_impl``), but the model should call the clean, documented
# names below (matching the Go port and the prompt's tool-selection guidelines).
_TOOL_NAME_BY_FUNC = {
    "_think_impl": "think",
    "_todo_write_impl": "todo_write",
    "_grep_chunks_impl": "grep_chunks",
    "_search_chunks_impl": "search_chunks",
    "_list_chunks_impl": "list_chunks",
    "_calculate_impl": "calculate",
    "_navigate_tree_impl": "navigate_tree",
    "_navigate_structure_impl": "navigate_structure",
}

# Thinking modes that enable the knowledge-compilation tools (``navigate_tree``
# + ``navigate_structure``). Mirrors the high/ultra static tool lists in
# ``harness/config.py``.
_COMPILED_TOOL_MODES = {"high", "ultra"}


def _with_clean_names(callables: list) -> list:
    """Return ``callables`` with their schema ``function.name`` set to the clean
    public name. Re-uses the cached schema to avoid re-deriving param docs."""
    for fn in callables:
        fname = fn.__name__
        clean = _TOOL_NAME_BY_FUNC.get(fname)
        if not clean:
            _LOG.warning("[dynamic] no public name mapped for tool function %r; leaving as-is", fname)
            continue
        schema = fn.openai_schema
        schema["function"]["name"] = clean
    return callables


async def _only_strings(stream):
    """Pass through only string items from a tool-loop stream, silently dropping
    non-string metadata (int token sentinels, provider-specific tuples)."""
    async for item in stream:
        if isinstance(item, str):
            yield item
