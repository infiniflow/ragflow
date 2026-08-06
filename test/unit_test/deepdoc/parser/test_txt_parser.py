#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#
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

"""Regression tests for ``RAGFlowTxtParser.parser_txt`` under the chunking contract.

The contract (see ``rag.nlp.merge_paragraphs``, refs #17799):

* delimiter = chunk boundary: delimiter text never enters a chunk;
* ``token_size`` = soft target + merge strategy (``OVER_CAP`` default); no
  atom-split — a paragraph larger than ``chunk_token_num`` stands alone and the
  model layer truncates it;
* ``UNDER_CAP`` is available as an explicit alternative strategy (never overflows
  ``chunk_token_num``; ``OVER_CAP`` allows one boundary overflow).
"""

from deepdoc.parser.txt_parser import RAGFlowTxtParser
import rag.nlp as nlp_mod


def _fake_word_tokens(s):
    return len(s.split())


def _nonempty(chunks):
    return [c for c, _ in chunks if c.strip()]


def test_over_cap_accumulates_adjacent_paragraphs(monkeypatch):
    monkeypatch.setattr(nlp_mod, "num_tokens_from_string", _fake_word_tokens)
    text = "\n".join(["alpha beta gamma delta" for _ in range(8)])  # 4 tokens each
    chunks = _nonempty(RAGFlowTxtParser.parser_txt(text, chunk_token_num=50, delimiter="\n"))
    # OVER_CAP greedily accumulates adjacent paragraphs while under cap, instead
    # of capping at fixed pairs: 8 * 4 = 32 tokens all fit under 50 -> 1 chunk.
    assert len(chunks) == 1
    assert len(chunks[0].split()) == 32
    # Content is preserved (32 tokens total).
    assert sum(len(c.split()) for c in chunks) == 32


def test_oversize_unit_not_atom_split(monkeypatch):
    monkeypatch.setattr(nlp_mod, "num_tokens_from_string", _fake_word_tokens)
    text = "word " * 200  # ~200 tokens, no delimiter -> one paragraph
    chunks = _nonempty(RAGFlowTxtParser.parser_txt(text, chunk_token_num=30, delimiter="\n!?;。；！？"))
    # No atom-split: the whole unit is a single chunk.
    assert len(chunks) == 1
    assert "".join(chunks).count("word") == 200


def test_delimiter_text_not_in_chunk(monkeypatch):
    monkeypatch.setattr(nlp_mod, "num_tokens_from_string", _fake_word_tokens)
    text = "first##second##third"
    chunks = _nonempty(RAGFlowTxtParser.parser_txt(text, chunk_token_num=1000, delimiter="##"))
    assert all("##" not in c for c in chunks)
    joined = "\n".join(chunks)
    assert "first" in joined and "second" in joined and "third" in joined


def test_consecutive_delimiters_do_not_leak_delimiter_text(monkeypatch):
    monkeypatch.setattr(nlp_mod, "num_tokens_from_string", _fake_word_tokens)
    # pattern "##": consecutive delimiters must not glue the sides with "##".
    text = "A####B"
    chunks = _nonempty(RAGFlowTxtParser.parser_txt(text, chunk_token_num=1000, delimiter="##"))
    joined = "\n".join(chunks)
    assert "##" not in joined
    assert "A" in joined and "B" in joined


def test_token_size_zero_keeps_each_paragraph_alone(monkeypatch):
    monkeypatch.setattr(nlp_mod, "num_tokens_from_string", _fake_word_tokens)
    text = "first second third"
    chunks = _nonempty(RAGFlowTxtParser.parser_txt(text, chunk_token_num=0, delimiter=" "))
    assert chunks == ["first", "second", "third"]


def test_delimiter_boundary_when_segment_exceeds_cap(monkeypatch):
    monkeypatch.setattr(nlp_mod, "num_tokens_from_string", _fake_word_tokens)
    # Each paragraph is 2 tokens (> cap=1) -> its own chunk.
    text = "aa aa\nbb bb\ncc cc"
    chunks = _nonempty(RAGFlowTxtParser.parser_txt(text, chunk_token_num=1, delimiter="\n"))
    assert chunks == ["aa aa", "bb bb", "cc cc"]


def test_keep_delimiters_preserves_delimiter(monkeypatch):
    monkeypatch.setattr(nlp_mod, "num_tokens_from_string", _fake_word_tokens)
    text = "first|second"
    chunks = _nonempty(RAGFlowTxtParser.parser_txt(text, chunk_token_num=1000, delimiter="|", keep_delimiters=True))
    # When keep_delimiters=True the delimiter is retained in the chunk.
    assert any("|" in c for c in chunks)
    joined = "\n".join(chunks)
    assert "first" in joined and "second" in joined


def test_empty_text_returns_empty():
    assert RAGFlowTxtParser.parser_txt("", chunk_token_num=128) == []
