"""In-memory grep+sed narrowing engine (term-driven, zero extra LLM rounds).

Mirrors the Claude Code / Codex ``grep`` + ``sed`` workflow, but operates on
retrieval chunks held in memory rather than on the filesystem: grep terms already
produced by the main-analysis LLM (entities, numbers, key phrases) are turned into
word-boundary regexes for locating (grep), and simple string transforms narrow the
text (sed), dropping unrelated boilerplate instead of crude head-truncation.

Key design (Claude Code semantics):
- The model that writes grep words is the same model that reasons about the
  answer: terms come from main-analysis output, no extra LLM call.
- Grep words are lightweight byproducts, not a separate LLM round (no extra
  call, no timeout, no wasted tokens).
- The engine executes mechanically (regex match + string transform), no LLM.

Fallback chain (never drops the answer):
    narrow_by_terms (grep locate + sed transform)
        -> no hits / no terms
        -> _narrow_by_keywords (keyword sentence-level, zero LLM)
        -> original chunks returned as-is (upper _build_compact_evidence
           head-truncation as the final safety valve)

Safety: only ``re.compile`` + pure string ops, no eval, no arbitrary code;
grep-term count cap + context clamp.
"""

import logging
import re

from rag.advanced_rag.harness.tools.search import (
    _split_sentences,
    _is_fact_dense_sentence,
    _narrow_by_keywords,
)

_LOG = logging.getLogger(__name__)

# Cost / safety caps.
_MAX_GREP_TERMS = 16
_MAX_CONTEXT = 2
_DEFAULT_OUT_CHARS_PER_CHUNK = 1200
_DEFAULT_OUT_TOTAL_CHARS = 16000
# Head length kept per chunk when there is no match.
_HEAD_FALLBACK_CHARS = 400
# Absolute char budget per side during context expansion, so an over-long line
# (or a chunk that the line splitter folded into one line) cannot inflate the
# narrowed fragment.
_CONTEXT_CHAR_BUDGET = 600
# Short chunks (<= this many chars) are not narrowed: they are already 1-2 lines,
# keeping them whole is safer (answers often live in short chunks).
_MIN_NARROW_CHARS = 200


def _escape_term(term: str) -> str:
    """Escape a plain grep term into a safe, word-boundary regex fragment.

    Numbers/entities are matched literally; very short terms (<=2 chars) and pure
    tokens are handled without \b so they don't vanish inside other words.
    """
    t = str(term).strip()
    if not t:
        return ""
    # Strip punctuation casing that could pollute the regex (inner digits/ hyphens
    # are kept).
    t = re.sub(r"^[\s.,:;!?'\"()\[\]{}]+|[\s.,:;!?'\"()\[\]{}]+$", "", t)
    if not t:
        return ""
    escaped = re.escape(t)
    # \b only works for ASCII word chars in Python re; it silently fails for CJK
    # (Chinese/Japanese/Korean), so never wrap CJK terms in \b.
    if re.search(r"[\u4e00-\u9fff\u3040-\u30ff\uac00-\ud7af]", t):
        return escaped
    # No \b for short terms (avoid failing to match "pop" inside "population");
    # use \b only for 3+ char words with alphanumeric edges.
    if len(t) >= 3 and t[0].isalnum() and t[-1].isalnum():
        return rf"\b{escaped}\b"
    return escaped


def _terms_to_patterns(terms) -> list[re.Pattern]:
    """Turn grep terms into a list of compiled regexes (one per term)."""
    out: list[re.Pattern] = []
    for term in (terms or [])[:_MAX_GREP_TERMS]:
        frag = _escape_term(term)
        if not frag:
            continue
        try:
            out.append(re.compile(frag, re.IGNORECASE))
        except re.error:
            continue
    return out


def _line_spans(content: str) -> list[tuple[int, int]]:
    """Line (start,end) spans, boundaries at ``\\n`` (grep semantics).

    Line boundaries are exact, unlike sentence splitting (which is lossy and may
    merge/trim whitespace). Start of line ``i`` is after the ``i``-th ``\\n``.
    """
    spans: list[tuple[int, int]] = []
    start = 0
    for nl in re.finditer(r"\n", content):
        spans.append((start, nl.start()))
        start = nl.end()
    if start <= len(content):
        spans.append((start, len(content)))
    if not spans:
        spans = [(0, len(content))]
    return spans


def _exec_on_text(
    content: str,
    patterns: list[re.Pattern],
    context: dict,
    out_chars_per_chunk: int,
) -> tuple[str, bool]:
    """Run term-grep + line-context expansion against one chunk's text.

    Mirrors ``grep -n -C N``: matches are located by ``match.start()/end()`` (exact),
    then expanded to whole lines, with ``before``/``after`` extra lines of context.
    Returns ``(narrowed, matched)``. On no hit, falls back to fact-dense-sentence
    keeping (never drops everything).
    """
    if not content:
        return "", False
    before = context.get("before", 0)
    after = context.get("after", 0)

    # Step 1: locate matches (exact positions from the regex engine).
    hit_ranges: list[tuple[int, int]] = []
    for pattern in patterns:
        try:
            for m in pattern.finditer(content):
                hit_ranges.append((m.start(), m.end()))
        except re.error:
            continue
    if not hit_ranges:
        # Keep fact-dense sentences to avoid dropping numbers/entities.
        kept: list[str] = []
        for s in _split_sentences(content):
            if _is_fact_dense_sentence(s):
                kept.append(s)
        narrowed = "".join(kept).strip()
        if narrowed:
            return narrowed[: _HEAD_FALLBACK_CHARS * 4], False
        return content[:_HEAD_FALLBACK_CHARS], False

    # Step 2: merge overlapping/adjacent matches, expand to line range + context.
    hit_ranges.sort()
    merged: list[tuple[int, int]] = []
    for s, e in hit_ranges:
        if merged and s <= merged[-1][1]:
            merged[-1] = (merged[-1][0], max(merged[-1][1], e))
        else:
            merged.append((s, e))
    lines = _line_spans(content)
    expanded: list[tuple[int, int]] = []
    for s, e in merged:
        lo = hi = 0
        for i, (ls, le) in enumerate(lines):
            if s >= ls and s < le:
                lo = i
            if e > ls and e <= le:
                hi = i
        lo = max(0, lo - before)
        hi = min(len(lines) - 1, hi + after)
        frag_s, frag_e = lines[lo][0], lines[hi][1]
        # Per-side character budget fallback: if the expanded window exceeds the
        # budget on either side, clamp to a compact window around the match.
        if frag_e - frag_s > _CONTEXT_CHAR_BUDGET * 2 and (frag_e - frag_s) > (e - s):
            frag_s = max(0, s - _CONTEXT_CHAR_BUDGET)
            frag_e = min(len(content), e + _CONTEXT_CHAR_BUDGET)
        expanded.append((frag_s, frag_e))

    # Step 3: dedupe, join, truncate.
    seen: set[str] = set()
    out_parts: list[str] = []
    for s, e in expanded:
        p = content[s:e].strip()
        if not p:
            continue
        key = p[:200]
        if key in seen:
            continue
        seen.add(key)
        out_parts.append(p)
    narrowed = "\n\n".join(out_parts).strip()
    if len(narrowed) > out_chars_per_chunk:
        narrowed = narrowed[:out_chars_per_chunk]
    return narrowed or content[:_HEAD_FALLBACK_CHARS], True


def _chunk_text(chunk) -> str:
    if isinstance(chunk, dict):
        return str(chunk.get("content_with_weight") or chunk.get("content") or chunk.get("text") or "")
    return str(chunk or "")


def _apply_narrow(chunks: list[dict], kept_texts: list[str], matched: list[bool]) -> list[dict]:
    out: list[dict] = []
    for ck, text, ok in zip(chunks, kept_texts, matched):
        d = dict(ck)
        if ok:
            d["content_with_weight"] = text
            if "content" in d:
                d["content"] = text
            d.pop("highlight", None)
        out.append(d)
    return out


def _fallback_narrow_by_keywords(chunks: list[dict], keywords: str) -> list[dict]:
    try:
        return _narrow_by_keywords(chunks, keywords) or chunks
    except Exception:
        return chunks


def narrow_by_terms(
    chunks: list[dict],
    terms,
    *,
    fallback_terms=None,
    context: dict | None = None,
    keywords: str = "",
    max_out_chars_per_chunk: int = _DEFAULT_OUT_CHARS_PER_CHUNK,
    max_out_total_chars: int = _DEFAULT_OUT_TOTAL_CHARS,
) -> dict:
    """Narrow retrieval chunks by locating grep terms.

    ``terms`` are plain strings (entities / numbers / key phrases) used to grep the
    chunks. If the primary terms produce no hit at all, ``fallback_terms`` are tried
    once mechanically (zero extra LLM). Still no hit -> narrowing is abandoned and
    the original chunks are returned (matched=False); the caller must NOT treat a
    failed narrow as an answer failure. Never raises.
    """
    ctx = context or {"before": 0, "after": 0}
    try:
        before = max(0, min(int(ctx.get("before", 0)), _MAX_CONTEXT))
        after = max(0, min(int(ctx.get("after", 0)), _MAX_CONTEXT))
    except (TypeError, ValueError):
        before = after = 0
    context = {"before": before, "after": after}

    patterns = _terms_to_patterns(terms)
    stats = {
        "chunks_in": len(chunks),
        "chunks_kept": 0,
        "chars_in": sum(len(_chunk_text(c)) for c in chunks),
        "chars_out": 0,
        "matched": False,
        "used_terms": len(patterns),
    }
    if not chunks:
        return {"kept": [], "stats": stats}

    # No usable grep terms -> fall back to keyword narrowing (zero LLM).
    if not patterns:
        narrowed = _fallback_narrow_by_keywords(chunks, keywords)
        stats["chunks_kept"] = len(narrowed)
        stats["chars_out"] = sum(len(_chunk_text(c)) for c in narrowed)
        return {"kept": narrowed, "stats": stats}

    def _run(active_patterns) -> tuple[list[str], list[bool]]:
        texts: list[str] = []
        flags: list[bool] = []
        for c in chunks:
            raw = _chunk_text(c)
            if len(raw) <= _MIN_NARROW_CHARS:
                texts.append(raw)
                flags.append(True)
                continue
            text, ok = _exec_on_text(raw, active_patterns, context, max_out_chars_per_chunk)
            texts.append(text)
            flags.append(ok)
        return texts, flags

    kept_texts, matched_flags = _run(patterns)

    # Gentle retry: if the primary terms hit nothing, try the fallback terms once
    # (mechanical, no extra LLM). Mirrors Claude Code re-grepping with a different
    # word before giving up on a region.
    if fallback_terms and not any(matched_flags):
        fb_patterns = _terms_to_patterns(fallback_terms)
        if fb_patterns:
            kept_texts, matched_flags = _run(fb_patterns)
            stats["used_terms"] = max(stats["used_terms"], len(fb_patterns))

    kept = _apply_narrow(chunks, kept_texts, matched_flags)
    # Only apply the total-length cap when the grep actually matched. When matched
    # is False (no term hit, no narrowing happened), the chunks are returned
    # untouched so the caller's own compaction decides how to truncate. Truncating
    # here on a no-match would otherwise drop most chunks to a single one, losing
    # evidence needed for multi-hop/enumeration answers.
    if any(matched_flags):
        total_out = sum(len(_chunk_text(c)) for c in kept)
        if total_out > max_out_total_chars:
            # Distribute the total budget across as many matched chunks as
            # possible, instead of letting the FIRST chunk swallow the whole cap
            # and dropping every later chunk. Each chunk is capped to
            # max_out_chars_per_chunk, and chunks are kept while the running
            # total fits in max_out_total_chars. This preserves evidence spread
            # across chunks (needed for multi-hop/enumeration) instead of a
            # single 16K blob.
            per_chunk_cap = max(200, min(max_out_chars_per_chunk, max_out_total_chars // max(1, len(kept))))
            acc = 0
            trimmed = []
            for c in kept:
                t = _chunk_text(c)
                room = max_out_total_chars - acc
                if room <= 0:
                    break
                take = min(len(t), per_chunk_cap, room)
                if take <= 0:
                    break
                if take < len(t):
                    c = dict(c)
                    c["content_with_weight"] = t[:take]
                    if "content" in c:
                        c["content"] = t[:take]
                trimmed.append(c)
                acc += take
            kept = trimmed

    stats["chunks_kept"] = len(kept)
    stats["chars_out"] = sum(len(_chunk_text(c)) for c in kept)
    stats["matched"] = any(matched_flags)
    _LOG.info(
        "[grep-sed] chunks=%d->%d chars=%d->%d matched=%s terms=%d",
        stats["chunks_in"],
        stats["chunks_kept"],
        stats["chars_in"],
        stats["chars_out"],
        stats["matched"],
        stats["used_terms"],
    )
    return {"kept": kept, "stats": stats}


def split_fallback_terms(*texts: str) -> list[str]:
    """Split free text into fallback grep terms (zero LLM).

    Used as the gentle-retry terms when the LLM-generated grep terms hit nothing.
    Any language: splits on sentence/comma boundaries, drops short/stopword-like
    tokens, keeps numbers and multi-word phrases as whole \b terms.
    """
    import re as _re

    terms: list[str] = []
    seen: set[str] = set()
    for v in texts:
        for part in _re.split(r"[\n。；;,.?!?]+", str(v or "")):
            part = part.strip().strip("'\"()[]{}")
            if not part or len(part) < 3:
                continue
            if part.lower() in _FALLBACK_STOPWORDS:
                continue
            if part in seen:
                continue
            seen.add(part)
            terms.append(part)
    return terms[:_MAX_GREP_TERMS]


_FALLBACK_STOPWORDS = {
    "what",
    "which",
    "who",
    "where",
    "when",
    "how",
    "the",
    "a",
    "an",
    "of",
    "in",
    "on",
    "for",
    "to",
    "and",
    "or",
    "with",
    "is",
    "are",
    "was",
    "were",
    "list",
    "name",
    "give",
    "find",
    "tell",
    "me",
    "about",
    "from",
    "that",
    "this",
    "it",
    "its",
    "their",
    "they",
    "have",
    "has",
    "do",
    "does",
    "did",
    "based",
    "per",
    "according",
    "not",
}


def grep_sed_narrow(
    chunks: list[dict],
    *,
    claim_sources: tuple[str, ...] = (),
    max_out_chars_per_chunk: int = _DEFAULT_OUT_CHARS_PER_CHUNK,
    max_out_total_chars: int = _DEFAULT_OUT_TOTAL_CHARS,
) -> dict:
    """Narrow chunks by grepping terms extracted directly from the claim (zero LLM).

    Grep terms are derived mechanically from the claim/question text (entities,
    numbers, proper nouns via ``split_fallback_terms``) — NO extra LLM call. The
    engine then greps+seds, with a gentle mechanical retry on a second-pass term
    set. Never raises; on any failure the original chunks are returned untouched
    so the caller's existing compaction is the final safety valve.
    """
    stats = {
        "chunks_in": len(chunks),
        "chunks_kept": 0,
        "chars_in": sum(len(_chunk_text(c)) for c in chunks),
        "chars_out": 0,
        "matched": False,
        "used_terms": 0,
    }
    if not chunks:
        return {"kept": chunks, "stats": stats}

    terms = split_fallback_terms(*claim_sources)
    stats["used_terms"] = len(terms)
    res = narrow_by_terms(
        chunks,
        terms,
        keywords=" ".join(claim_sources),
        max_out_chars_per_chunk=max_out_chars_per_chunk,
        max_out_total_chars=max_out_total_chars,
    )
    return res
