"""Regression test for token_size-mode delimiter handling in TokenChunker.

The default ``delimiters`` a user supplies (e.g. ``["\\n"]``) are only honoured
when they are backtick-wrapped; a bare delimiter makes
``_compile_delimiter_pattern`` return ``""``, so ``TokenChunker._invoke`` falls
through to ``naive_merge(payload, chunk_token_size, "")`` (token_chunker.py:323).

Passing ``""`` discards the sentence-boundary preference that ``naive_merge``
carries by default (``"\\n。；！？"``). Without a forced ``\\n`` break, the token
budget flush can land *inside* a sentence, so a line like
``"Sentence number 7. alpha beta gamma delta epsilon zeta eta theta iota kappa"``
gets split across two chunks. The Go port (``sentenceDelimiter = (\\n|[!?。；！？])``
in token.go:340) always breaks on ``\\n``, so it never cuts a sentence mid-stream.

This test exposes that divergence: with ``delimiters=["\\n"]`` in token_size mode,
every original sentence must survive intact in a single chunk. It currently FAILS
because of the ``""`` passed at token_chunker.py:326; forwarding
``"".join(self._param.delimiters)`` to ``naive_merge`` (or at least
``"\\n。；！？"``) makes it pass and aligns Python with the Go side.

Note: the test needs a live tiktoken encoder. ``common.token_utils`` builds it on
first use, so an unavailable BPE table raises out of ``num_tokens_from_string``,
while a failure to encode a valid string is reported as 0. Only the zero produces
the false "budget never exceeded" baseline that would hide the bug behind a single
chunk; the raise just makes the test unable to run at all. Neither leaves a usable
token budget, so we treat both as a dead tokenizer and skip rather than pass,
mirroring ``capture_golden.py::_assert_tokenizer_alive``.
"""

import asyncio
import types

from rag.flow.chunker.token_chunker import TokenChunker, TokenChunkerParam
from rag.nlp import naive_merge
from common.token_utils import num_tokens_from_string


def _build_token_chunker(param: dict) -> TokenChunker:
    """Construct a TokenChunker without the real Graph the normal __init__ wants.

    ``ComponentBase.__init__`` asserts ``canvas`` is a real ``Graph``; a unit test
    has none. Bypass it and attach the three attributes ``_invoke`` actually reads
    (``_canvas``, ``_param``, ``callback``) so the genuine chunking path runs with
    the real ``naive_merge`` and the real ``num_tokens_from_string``. Mirrors
    ``capture_golden.py::_build_component``.
    """
    p = TokenChunkerParam()
    for key, value in param.items():
        setattr(p, key, value)
    p.check()  # __new__ bypassed the constructor's validation; reproduce it.

    comp = TokenChunker.__new__(TokenChunker)
    comp._canvas = types.SimpleNamespace(_doc_id=None, _tenant_id="t")
    comp._param = p
    comp.callback = lambda *_a, **_kw: None
    return comp


def _invoke_text(comp: TokenChunker, text: str) -> list[dict]:
    asyncio.run(comp._invoke(name="t", output_format="text", text=text))
    return comp._param.outputs["chunks"]["value"]


def _make_sentences(n: int) -> list[str]:
    # Each sentence is ~16 tokens of plain English; well under the 128 budget,
    # so several fit per chunk and the token-budget flush lands mid-sentence.
    return [f"Sentence number {i}. alpha beta gamma delta epsilon zeta eta theta iota kappa" for i in range(n)]


def _tokenizer_is_alive() -> bool:
    """True only when the real encoder counts tokens.

    An unavailable BPE table raises out of ``num_tokens_from_string``; a failed
    encode of a valid string returns 0. Both mean the token budget is useless here.
    """
    try:
        return num_tokens_from_string("alive tokenizer probe sentence") > 0
    except Exception:
        return False


def test_token_chunker_token_size_mode_does_not_split_sentences():
    # Guard: a dead tokenizer would collapse everything to one chunk and hide
    # the bug. Skip instead of recording a poisoned pass.
    if not _tokenizer_is_alive():
        import pytest

        pytest.skip("tiktoken encoder unavailable")

    sentences = _make_sentences(30)
    payload = "\n".join(sentences)

    comp = _build_token_chunker({"chunk_token_size": 128, "delimiters": ["\n"]})
    chunks = _invoke_text(comp, payload)
    texts = [c["text"] for c in chunks]

    # An original sentence is "intact" when its full text appears in exactly one
    # chunk. If the budget flush fell inside it, the sentence is split across two
    # chunks and this assertion fails.
    split_sentences = [s for s in sentences if not any(s in t for t in texts)]
    assert not split_sentences, (
        f"{len(split_sentences)} sentence(s) were cut mid-stream by "
        f"token_size mode with delimiters=['\\n'] (got {len(chunks)} chunks). "
        "naive_merge was called with an empty delimiter (token_chunker.py:326), "
        "dropping the '\\n' sentence-boundary preference. Example split: "
        f"{split_sentences[0]!r}"
    )


def test_naive_merge_empty_delimiter_keeps_unit_whole():
    """Empty delimiter -> no split; the whole payload is one chunk (no
    atom-split). A '\\n' delimiter still honours the newline boundary.

    Documents the new contract: without a delimiter there is nothing to split
    on, so the unit is kept whole and the model layer truncates it.
    """
    if not _tokenizer_is_alive():
        import pytest

        pytest.skip("tiktoken encoder unavailable")

    sentences = _make_sentences(30)
    payload = "\n".join(sentences)

    chunks_empty = naive_merge(payload, 128, "")
    chunks_nl = naive_merge(payload, 128, "\n")

    # Empty delimiter no longer cuts (no atom-split): everything stays in one chunk.
    assert len(chunks_empty) == 1
    split_nl = [s for s in sentences if not any(s in t for t in chunks_nl)]
    assert not split_nl, "naive_merge('\\n') should preserve every sentence boundary"
