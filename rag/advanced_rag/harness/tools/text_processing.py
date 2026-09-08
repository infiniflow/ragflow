"""Keyword-driven text processing shared by the retrieval tools.

Sentence splitting, light stemming, and the keyword narrowing/highlighting that
keeps chunk payloads token-cheap: retrieval returns full chunks, and narrowing
cuts each one down to the sentences that actually carry the query terms.

Lives in its own module because it is pure text work — no retrieval, no store
access — and is reused well beyond ``search`` (grep/sed narrowing, memory,
navigation).
"""

import hashlib
import logging
import re
from functools import lru_cache

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
    # Markdown pipe tables (>=3 rows with >=2 pipes) get the same full-text pass:
    # their answer rows often sit mid-table (e.g. a rank row at ~62% of a 14.7K-char
    # table), and sentence-window narrowing truncates them to a header-only snippet.
    low_content = content.lower()
    if "<table" in low_content or "<tr" in low_content or "<td" in low_content:
        return "..." + _highlight_keywords(content, kwds) + "..."
    pipe_rows = sum(1 for line in content.splitlines() if line.count("|") >= 2)
    if pipe_rows >= 3:
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
