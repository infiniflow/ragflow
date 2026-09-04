"""Parity: canvas TokenChunker default delimiters must match General's.

The General parser (``rag/app/naive.py``) chunks with
``rag/nlp/delim.DEFAULT_DELIMITER`` (``"\\n!?;。；！？"``). The canvas
TokenChunker historically defaulted to ``["\\n"]`` (introduced in PR #13826), so
even at the same ``chunk_token_size`` the two paths produced different chunks.

This test pins the canvas default to the canonical delimiter set so a canvas
node left at its default config chunks identically to the General parser.
"""

import asyncio
import types

import pytest

from common.token_utils import num_tokens_from_string
from rag.flow.chunker.token_chunker import TokenChunker, TokenChunkerParam
from rag.nlp import naive_merge
from rag.nlp.delim import DEFAULT_DELIMITER


def _build_token_chunker(param: dict) -> TokenChunker:
    """Build a TokenChunker without the real Graph (mirrors existing tests)."""
    p = TokenChunkerParam()
    for key, value in param.items():
        setattr(p, key, value)
    p.check()
    comp = TokenChunker.__new__(TokenChunker)
    comp._canvas = types.SimpleNamespace(_doc_id=None, _tenant_id="t")
    comp._param = p
    comp.callback = lambda *_a, **_kw: None
    return comp


def _invoke_text(comp: TokenChunker, text: str) -> list[dict]:
    asyncio.run(comp._invoke(name="t", output_format="text", text=text))
    return comp._param.outputs["chunks"]["value"]


def _make_sample(n: int) -> str:
    # No newlines on purpose: with a bare "\\n" delimiter this stays one
    # paragraph, while the sentence delimiters (.!?;。；！？) split it into many.
    eng = "Sentence number {}. This is a clause! Another clause? A semicolon split; more text. "
    chn = "中文句子{}. 这是一句话！另一句话？分号分隔；更多文本。 "
    parts = []
    for i in range(n):
        parts.append(eng.format(i))
        parts.append(chn.format(i))
    return "".join(parts)


SAMPLE = _make_sample(30)  # ~900 tokens, exceeds the 512 cap.


def test_canvas_default_delimiters_match_general():
    # Default delimiter set must reference the canonical DEFAULT_DELIMITER.
    assert TokenChunkerParam().delimiters == list(DEFAULT_DELIMITER)


def test_canvas_default_chunking_matches_general():
    # Guard: a dead tokenizer would collapse everything to one chunk and hide
    # the bug. Skip instead of recording a poisoned pass.
    if num_tokens_from_string("alive tokenizer probe sentence") <= 0:
        pytest.skip("tiktoken encoder unavailable; num_tokens_from_string returned 0")

    comp = _build_token_chunker({"chunk_token_size": 512})  # default delimiters
    canvas = _invoke_text(comp, SAMPLE)
    canvas_texts = [c["text"].strip() for c in canvas if c["text"].strip()]

    general = naive_merge(SAMPLE, 512, DEFAULT_DELIMITER, 0)
    general_texts = [c.strip() for c in general if c.strip()]

    assert canvas_texts == general_texts, f"Canvas default chunking differs from General.\ncanvas ({len(canvas_texts)} chunks): {canvas_texts}\ngeneral ({len(general_texts)} chunks): {general_texts}"
