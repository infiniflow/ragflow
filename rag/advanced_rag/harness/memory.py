"""Retrieval memory — a central store of every raw chunk any claim has retrieved.

Purpose
-------
Throughout a multi-hop answer the system retrieves many chunks (search is cheap)
but hands the LLM only a small narrowed slice (LLM calls are slow & token-heavy).
The raw chunks must not be thrown away: the LLM's per-claim report/grounded
extraction can compress away a fact the answer needs (e.g. a candidate set
"R, RMP, RA, RF, RIF" collapsed to "RIF"). ``RetrievalMemory`` keeps ALL raw
chunks that any search returned, so the finalizer can do a cheap, deterministic,
no-LLM, no-knowledge-base relevance search over the already-retrieved corpus to
recover the missing fact instead of re-querying.

Design (in-memory, language-agnostic)
-------------------------------------
- ``add`` stores every raw chunk exactly as returned by the search backend,
  de-duplicated by chunk identity, BEFORE any narrowing — it is the lossless
  store backing the (lossy) ``kbinfos["chunks"]`` that feeds the LLM.
- ``search`` is the primary consumer-facing primitive: a relevance-ranked lookup
  over memory for a query string, language-agnostic (Latin words + numbers + CJK
  character 3-grams), requiring a normalized overlap bar so Chinese and English
  queries behave alike. It never invokes the LLM and never triggers a knowledge-
  base search.
- ``grep`` is a lower-level loose-keyword primitive (word-boundary + prefix
  tolerance) used as a building block / for tests; production finalize uses the
  stricter ``search``.

The store lives on ``tools.kbinfos["memory"]`` so it is serialized / carried with
the rest of the research state and is visible to any tool that has ``tools``.
"""

import logging
import re

_LOG = logging.getLogger(__name__)

# How many memory chunks a single grep query may return at most (keeps the
# injected evidence bounded).
_GREP_MAX_CHUNKS = 6
# Max sentences kept per chunk (hit + context) so a long chunk does not blow up
# the injection.
_GREP_MAX_SENTENCES = 4
# Absolute char budget per side of a hit when expanding context.
_GREP_CONTEXT_CHARS = 400
# Short chunks are kept whole (answers often live in short chunks).
_SHORT_CHUNK_CHARS = 200


def _chunk_key(ck: dict) -> str:
    return str(ck.get("chunk_id") or ck.get("id") or id(ck))


def _chunk_text(ck: dict) -> str:
    """The searchable text of a chunk, preferring the raw original."""
    for k in ("content", "content_with_weight"):
        v = ck.get(k)
        if v:
            return str(v)
    return ""


def _escape_term(term: str) -> str:
    t = str(term).strip()
    if not t:
        return ""
    t = re.sub(r"^[\s.,:;!?'\"()\[\]{}]+|[\s.,:;!?'\"()\[\]{}]+$", "", t)
    if not t:
        return ""
    escaped = re.escape(t)
    # CJK: no \b (Python re \b is ASCII-only and would never match).
    if re.search(r"[\u4e00-\u9fff\u3040-\u30ff\uac00-\ud7af]", t):
        return escaped
    if len(t) >= 3 and t[0].isalnum() and t[-1].isalnum():
        return rf"\b{escaped}\b"
    return escaped


def _split_sentences(text: str) -> list[str]:
    """Split into sentences, treating block HTML/markdown tables as atomic."""
    # Reuse the shared sentence splitter from tools.search when available, else a
    # minimal one. Import lazily to avoid a heavy module graph at load time.
    try:
        from rag.advanced_rag.harness.tools.search import _split_sentences as _ss

        return _ss(text)
    except Exception:
        pass
    return [s for s in re.split(r"(?<=[.!?。！？])\s+", (text or "").strip()) if s]


def _sentence_span_window(sents: list[str], idx: int) -> list[str]:
    """The hit sentence plus up to one neighbouring sentence on each side."""
    lo = max(0, idx - 1)
    hi = min(len(sents), idx + 2)
    window = sents[lo:hi]
    # Clamp total length.
    total = 0
    kept = []
    for s in window:
        total += len(s)
        if total > _GREP_CONTEXT_CHARS * 2:
            break
        kept.append(s)
    return kept or [sents[idx]]


def add(tools, chunks) -> None:
    """Merge raw retrieved chunks into the central memory store (lossless)."""
    if not chunks:
        return
    mem = tools.kbinfos.setdefault("memory", [])
    seen = {_chunk_key(c) for c in mem}
    added = 0
    for c in chunks:
        if not isinstance(c, dict) or not _chunk_text(c):
            continue
        k = _chunk_key(c)
        if k in seen:
            continue
        seen.add(k)
        mem.append(c)
        added += 1
    if added:
        _LOG.info("[Memory] stored %d new raw chunk(s); memory now has %d.", added, len(mem))


def grep(tools, terms, limit: int = _GREP_MAX_CHUNKS) -> list[dict]:
    """Return memory chunks that contain any of ``terms`` (word-boundary grep),
    narrowed to the matching sentence + small context.

    ``terms`` is a list of plain strings (entities / numbers / key phrases) as
    emitted by the analysis LLM. Returns a list of chunk dicts each carrying a
    narrowed ``content`` (a plain string) so the caller can splice them straight
    into an evidence list. Empty on no-hit / no-memory.
    """
    mem = tools.kbinfos.get("memory", []) or []
    if not mem or not terms:
        return []
    patterns = []
    # Prefix fallback: morphological tolerance. A gap term often differs from the
    # chunk's word by a suffix/prefix (gap "abbreviation" vs chunk "abbreviated";
    # gap "rifampin" vs chunk "Rifampicin"). We match the term's leading stem
    # (first 5 chars for terms >= 6) at the START of a chunk word, so a shared
    # root still hits without a short token over-matching. CJK terms are matched
    # verbatim (no stemming semantics).
    prefix_patterns = []
    for t in terms:
        frag = _escape_term(t)
        if frag:
            try:
                patterns.append(re.compile(frag, re.IGNORECASE))
            except re.error:
                continue
        _stripped = str(t).strip()
        _prefix = _stripped[:5] if len(_stripped) >= 6 else ""
        if _prefix and not re.search(r"[\u4e00-\u9fff\u3040-\u30ff\uac00-\ud7af]", _prefix):
            try:
                prefix_patterns.append(re.compile(rf"\b{re.escape(_prefix)}", re.IGNORECASE))
            except re.error:
                continue
    if not patterns and not prefix_patterns:
        return []

    def _match(text: str) -> bool:
        if any(p.search(text) for p in patterns):
            return True
        # Prefix fallback: the term's leading stem occurs at the START of a
        # chunk word (word boundary), tolerating inflectional suffixes.
        if prefix_patterns:
            for pp in prefix_patterns:
                if pp.search(text):
                    return True
        return False

    hits = []
    for c in mem:
        text = _chunk_text(c)
        if len(text) <= _SHORT_CHUNK_CHARS:
            # Short chunk: keep whole (its answer may live anywhere in it).
            if _match(text):
                hits.append({"content": text, "doc_id": c.get("doc_id"), "chunk_id": c.get("chunk_id")})
            continue
        sents = _split_sentences(text)
        kept = []
        for i, s in enumerate(sents):
            if _match(s):
                for w in _sentence_span_window(sents, i):
                    if w not in kept:
                        kept.append(w)
            if len(kept) >= _GREP_MAX_SENTENCES:
                break
        if kept:
            hits.append({"content": "\n".join(kept), "doc_id": c.get("doc_id"), "chunk_id": c.get("chunk_id")})
        if len(hits) >= limit:
            break
    return hits


def size(tools) -> int:
    return len(tools.kbinfos.get("memory", []) or [])


def clear(tools) -> None:
    tools.kbinfos["memory"] = []


# ─────────────────────────────────────────────────────────────────────────────
# Relevance-ranked retrieval over memory (used as a retrieval-reuse cache, NOT
# as a noise-injection source). Unlike ``grep`` (loose keyword hit), ``search``
# scores each memory chunk by how many SIGNIFICANT query terms it actually
# contains and returns only the chunks that clear a multi-term overlap bar —
# quality comparable to a knowledge-base retrieval, but served from memory so a
# fact retrieved earlier can be reused instead of re-querying the index.
# ─────────────────────────────────────────────────────────────────────────────

_STOPWORDS = {
    "what",
    "which",
    "how",
    "many",
    "much",
    "does",
    "did",
    "do",
    "the",
    "a",
    "an",
    "is",
    "are",
    "was",
    "were",
    "be",
    "been",
    "being",
    "of",
    "for",
    "to",
    "in",
    "on",
    "with",
    "and",
    "or",
    "by",
    "from",
    "at",
    "it",
    "its",
    "this",
    "that",
    "these",
    "those",
    "who",
    "when",
    "where",
    "why",
    "than",
    "then",
    "there",
    "their",
    "they",
    "them",
    "his",
    "her",
    "him",
    "she",
    "he",
    "we",
    "you",
    "your",
}

# CJK / other scripts have no word boundaries and no shared stopword list; match
# them as literal substrings. Script ranges: CJK unified ideographs, Hiragana,
# Katakana, Hangul, CJK punctuation is excluded.
_CJK_RE = re.compile(r"[\u4e00-\u9fff\u3040-\u30ff\uac00-\ud7af]+")
_LATIN_NUM_RE = re.compile(r"[A-Za-z0-9]+")


def _is_cjk(s: str) -> bool:
    return bool(_CJK_RE.fullmatch(s))


def _significant_terms(text: str, max_terms: int = 18) -> list[str]:
    """Language-agnostic significant-term extraction.

    - Latin alphanumeric runs → lowercased words (stopword-filtered, len >= 3).
    - CJK runs (Chinese/Japanese/Korean) → split into character 3-grams (no word
      boundaries exist, so n-grams are the language-agnostic way to match a
      substring like 利福平 inside a chunk); stopwords are NOT applied.
    - standalone numbers kept verbatim.
    De-duplicated, capped. Returns [] when nothing searchable.
    """
    out: list[str] = []
    seen: set[str] = set()

    def _push(tok: str) -> None:
        if tok and tok not in seen:
            seen.add(tok)
            out.append(tok)

    # Numbers anywhere.
    for m in re.finditer(r"\d+", text or ""):
        _push(m.group(0))
        if len(out) >= max_terms:
            return out
    # CJK runs → 3-grams (and the whole run if shorter than 3).
    for m in _CJK_RE.finditer(text or ""):
        run = m.group(0)
        if len(run) < 3:
            _push(run)
        else:
            for i in range(len(run) - 2):
                _push(run[i : i + 3])
        if len(out) >= max_terms:
            return out
    # Latin words (stopword-filtered).
    for m in _LATIN_NUM_RE.finditer(text or ""):
        raw = m.group(0)
        if raw.isdigit():
            continue  # numbers already handled
        low = raw.lower()
        if len(low) >= 3 and low not in _STOPWORDS:
            _push(low)
        if len(out) >= max_terms:
            return out
    return out


def _term_hits(text: str, terms: list[str]) -> int:
    """How many of ``terms`` occur in ``text``, language-agnostically.

    - CJK terms → literal substring (no word boundary).
    - Latin terms → word-boundary match with a short prefix fallback for
      inflectional variants ("abbreviation" also matches "abbreviated").
    - numbers → literal presence.
    """
    if not terms:
        return 0
    hits = 0
    for t in terms:
        if _is_cjk(t):
            if t in text:
                hits += 1
        elif t.isdigit():
            if t in text:
                hits += 1
        else:
            if re.search(rf"\b{re.escape(t)}\b", text, re.IGNORECASE):
                hits += 1
            elif len(t) >= 6:
                _prefix = re.escape(t[:5])
                if re.search(rf"\b{_prefix}", text, re.IGNORECASE):
                    hits += 1
    return hits


def search(tools, query: str, top_n: int = 6, min_overlap: int = 2, min_ratio: float = 0.12) -> list[dict]:
    """Relevance-ranked retrieval over memory for a query string (language-agnostic).

    Scores each memory chunk by how many of the query's significant terms
    (Latin words / numbers / CJK 3-grams) it matches, and keeps chunks that clear
    a *normalized* overlap bar so Chinese and English queries behave alike: a
    chunk is relevant when it shares ``>= 1`` term AND ``>= min_ratio`` of the
    query's terms (a long CJK query matches a short "利福平" via shared 3-grams;
    an English query matches "abbreviation → abbreviated" via word-boundary +
    prefix). Ranked by hit count. Cheap, deterministic, no-LLM — used to REUSE
    evidence already retrieved, never to inject loose noise. Returns [] when
    nothing clears the bar (caller falls back to the knowledge-base search).
    """
    mem = tools.kbinfos.get("memory", []) or []
    terms = _significant_terms(query)
    if not mem or not terms:
        return []
    _n = len(terms)
    scored = []
    for c in mem:
        text = _chunk_text(c)
        if not text:
            continue
        hits = _term_hits(text, terms)
        if hits >= 1 and (hits / _n) >= min_ratio:
            scored.append((hits, text, c))
    if not scored:
        return []
    scored.sort(key=lambda x: (-x[0], -len(x[1])))
    out = []
    for hits, text, c in scored[:top_n]:
        out.append({"content": text, "doc_id": c.get("doc_id"), "chunk_id": c.get("chunk_id"), "similarity": float(hits)})
    _LOG.info("[Memory.search] query=%r -> %d relevant chunk(s) (ratio>=%.2f, %d terms)", (query or "")[:60], len(out), min_ratio, _n)
    return out
