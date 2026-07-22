#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

"""Unit tests for ``RAGFlowTxtParser.parser_txt`` strict-cap behaviour.

The pre-fix ``add_chunk`` fired its size check *after* the append, so each
chunk could overshoot ``chunk_token_num`` by up to the size of one line. These
tests assert the proactive projected-total invariant: no produced chunk may
contain more than ``chunk_token_num`` tokens.
"""

import importlib.util
import os
import sys
from unittest import mock


_MOCK_MODULES = [
    "xgboost",
    "pdfplumber",
    "huggingface_hub",
    "PIL",
    "PIL.Image",
    "pypdf",
    "sklearn",
    "deepdoc.vision",
]
for _m in _MOCK_MODULES:
    if _m not in sys.modules:
        sys.modules[_m] = mock.MagicMock()

# Avoid pulling heavy parsers / rag_tokenizer via deepdoc.parser.__init__.
if "deepdoc" not in sys.modules:
    sys.modules["deepdoc"] = mock.MagicMock()
if "deepdoc.parser" not in sys.modules:
    sys.modules["deepdoc.parser"] = mock.MagicMock()
if "deepdoc.parser.utils" not in sys.modules:
    sys.modules["deepdoc.parser.utils"] = mock.MagicMock()
# ``get_text`` is invoked by ``RAGFlowTxtParser.__call__`` only, not by
# ``parser_txt``. Provide a permissive stub so the module loads.
sys.modules["deepdoc.parser.utils"].get_text = lambda *a, **kw: ""


def _find_project_root(marker="pyproject.toml"):
    d = os.path.dirname(os.path.abspath(__file__))
    while d != os.path.dirname(d):
        if os.path.exists(os.path.join(d, marker)):
            return d
        d = os.path.dirname(d)
    return None


_PROJECT_ROOT = _find_project_root()

_spec = importlib.util.spec_from_file_location(
    "deepdoc.parser._txt_parser_under_test",
    os.path.join(_PROJECT_ROOT, "deepdoc", "parser", "txt_parser.py"),
)
_mod = importlib.util.module_from_spec(_spec)
sys.modules["deepdoc.parser._txt_parser_under_test"] = _mod
_spec.loader.exec_module(_mod)

RAGFlowTxtParser = _mod.RAGFlowTxtParser


# A deterministic, tokenizer-free stand-in for ``num_tokens_from_string`` so
# the assertions below reason in plain words and are independent of tiktoken.


def _patch_word_count(monkeypatch_module):
    def fake_num_tokens(s):
        return len((s or "").split())

    monkeypatch_module.setattr(_mod, "num_tokens_from_string", fake_num_tokens)


def test_no_overshoot_when_packing_short_lines(monkeypatch):
    """Lines of 25 tokens, budget 100 — every chunk must be <= 100 tokens."""
    _patch_word_count(monkeypatch)
    txt = " ".join(["alpha"] * 25) + "\n" + " ".join(["beta"] * 25) + "\n" + " ".join(["gamma"] * 25)
    chunks = RAGFlowTxtParser.parser_txt(txt, chunk_token_num=100, delimiter="\n")
    sizes = [len(c[0].split()) for c in chunks if c[0].strip()]
    assert all(s <= 100 for s in sizes), sizes
    # 75 tokens of content, expected a single 75-token chunk.
    assert sum(sizes) == 75


def test_no_overshoot_at_chunk_boundary(monkeypatch):
    """Lines of 30 tokens, budget 100. Pre-fix the boundary chunk was 130 tokens."""
    _patch_word_count(monkeypatch)
    lines = [" ".join([f"w{i}"] * 30) for i in range(10)]  # 10 lines, 300 tokens
    chunks = RAGFlowTxtParser.parser_txt("\n".join(lines), chunk_token_num=100, delimiter="\n")
    sizes = [len(c[0].split()) for c in chunks if c[0].strip()]
    assert all(s <= 100 for s in sizes), sizes


def test_atomic_oversized_line_is_sub_split_on_whitespace(monkeypatch):
    """A single line that exceeds the budget is split on whitespace atoms."""
    _patch_word_count(monkeypatch)
    huge_line = " ".join(["alpha"] * 80)  # 80 tokens, no internal delimiter
    chunks = RAGFlowTxtParser.parser_txt(huge_line, chunk_token_num=50, delimiter="\n")
    sizes = [len(c[0].split()) for c in chunks if c[0].strip()]
    assert all(s <= 50 for s in sizes), sizes
    assert sum(sizes) == 80
    assert len(chunks) >= 2


def test_empty_text_returns_empty(monkeypatch):
    _patch_word_count(monkeypatch)
    # Empty input produces a single empty chunk placeholder (existing
    # behaviour the callers rely on). The hard-cap guarantee is that any
    # chunk carrying content stays within the budget.
    result = RAGFlowTxtParser.parser_txt("", chunk_token_num=128, delimiter="\n")
    non_empty = [c for c in result if c[0].strip()]
    assert non_empty == []
    result2 = RAGFlowTxtParser.parser_txt("   \n\n  ", chunk_token_num=128, delimiter="\n")
    non_empty2 = [c for c in result2 if c[0].strip()]
    assert non_empty2 == []


def test_unbroken_token_exceeding_budget_fallback(monkeypatch):
    """A single unbroken non-whitespace string exceeding the budget is split
    via the character-window/token-slicing fallback.
    """

    def char_count_tokens(s):
        return len(s or "")

    monkeypatch.setattr(_mod, "num_tokens_from_string", char_count_tokens)

    huge_word = "a" * 80  # 80 characters/tokens, no whitespace
    chunks = RAGFlowTxtParser.parser_txt(huge_word, chunk_token_num=30, delimiter="\n")

    non_empty = [c[0] for c in chunks if c[0].strip()]
    assert len(non_empty) >= 3
    assert all(char_count_tokens(c) <= 30 for c in non_empty)
    assert "".join(non_empty) == huge_word


def test_newline_join_token_count_strict_cap(monkeypatch):
    """Verify that joining chunks with newline does not overshoot chunk_token_num
    even when individual token counts sum to <= budget but the newline pushes it over.
    """

    def char_count_tokens(s):
        return len(s or "")

    monkeypatch.setattr(_mod, "num_tokens_from_string", char_count_tokens)

    # Two lines of 10 chars each. Budget = 20.
    # line1 + "\n" + line2 = 10 + 1 + 10 = 21 chars/tokens, exceeding budget of 20.
    line1 = "a" * 10
    line2 = "b" * 10
    txt = f"{line1}\n{line2}"
    chunks = RAGFlowTxtParser.parser_txt(txt, chunk_token_num=20, delimiter="\n")
    non_empty = [c[0] for c in chunks if c[0].strip()]
    assert all(char_count_tokens(c) <= 20 for c in non_empty)
    assert len(non_empty) == 2

