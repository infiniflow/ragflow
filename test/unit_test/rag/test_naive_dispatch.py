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

"""Regression tests for the PDF parser dispatch in :mod:`rag.app.naive`.

Issue #17114: a document whose ``parser_config.layout_recognize`` was a stale
``TenantModel`` UUID (rather than the literal ``"MinerU"`` keyword) was being
dispatched to :func:`by_plaintext`, which tried to resolve the id as an
IMAGE2TEXT vision model and failed with
``Provider <empty> not found for model <id>``.

The fix routes the dispatch to :func:`by_mineru` whenever
``layout_recognize`` does not match any known parser name AND the parser
config carries MinerU-specific options (``mineru_*`` keys).

These tests cover the two helper entry points:

1. :func:`common.parser_config_utils.has_mineru_options` — the predicate
   that detects MinerU intent (the easy bit, but easy to regress).
2. :func:`common.parser_config_utils.normalize_layout_recognizer` — the
   hook that strips the ``@mineru`` / ``@opendataloader`` / ``@paddleocr``
   / ``@somark`` suffixes produced by the Tenant LLM Provider UI.

The full dispatch in :func:`rag.app.naive._dispatch_pdf_parser` is wired
up by the chunk() call site and is verified end-to-end via the
``testcases`` integration suite; we keep the unit tests focused on the
helper predicates that drive the recovery branch.
"""

from common.parser_config_utils import (
    MINERU_OPTION_KEYS,
    has_mineru_options,
    normalize_layout_recognizer,
)


# --------------------------------------------------------------------------- #
# has_mineru_options
# --------------------------------------------------------------------------- #


def test_has_mineru_options_recognizes_mineru_keys():
    assert has_mineru_options({"mineru_parse_method": "auto"})
    assert has_mineru_options({"mineru_formula_enable": True})
    assert has_mineru_options({"mineru_table_enable": True})
    assert has_mineru_options({"mineru_lang": "English"})


def test_has_mineru_options_returns_false_without_mineru_keys():
    assert not has_mineru_options({"layout_recognize": "DeepDOC"})
    assert not has_mineru_options({"layout_recognize": "Plain Text"})
    assert not has_mineru_options({})


def test_has_mineru_options_handles_non_dict():
    # Defensive: the dispatch must not crash on unexpected shapes.
    assert not has_mineru_options(None)
    assert not has_mineru_options("mineru_parse_method")
    assert not has_mineru_options(["mineru_parse_method"])


def test_mineru_option_keys_are_exhaustive():
    # Guards against a regression where a new mineru_* option is added in
    # the API but forgotten in the dispatch predicate.
    expected = {"mineru_parse_method", "mineru_formula_enable", "mineru_table_enable", "mineru_lang"}
    assert set(MINERU_OPTION_KEYS) == expected


# --------------------------------------------------------------------------- #
# normalize_layout_recognizer (existing helper, regression pinning)
# --------------------------------------------------------------------------- #


def test_normalize_layout_recognizer_passes_through_known_keywords():
    assert normalize_layout_recognizer("DeepDOC") == ("DeepDOC", None)
    assert normalize_layout_recognizer("MinerU") == ("MinerU", None)
    assert normalize_layout_recognizer("docling") == ("docling", None)


def test_normalize_layout_recognizer_strips_known_provider_suffix():
    assert normalize_layout_recognizer("my-llm@my-instance@my-provider@mineru") == ("MinerU", "my-llm@my-instance@my-provider@mineru")
    assert normalize_layout_recognizer("my-llm@my-instance@my-provider@paddleocr") == ("PaddleOCR", "my-llm@my-instance@my-provider@paddleocr")
    assert normalize_layout_recognizer("my-llm@my-instance@my-provider@opendataloader") == ("OpenDataLoader", "my-llm@my-instance@my-provider@opendataloader")
    assert normalize_layout_recognizer("my-llm@my-instance@my-provider@somark") == ("SoMark", "my-llm@my-instance@my-provider@somark")


def test_normalize_layout_recognizer_passes_stale_uuid_through():
    """The dispatcher relies on normalize_layout_recognizer leaving a stale
    TenantModel UUID unchanged so that the by_mineru fallback branch can
    detect it (issue #17114). If this test ever fails because the UUID is
    rewritten to something else, the dispatch recovery in naive.py is
    silently broken."""
    stale_uuid = "06d85f8e819111f1995ef33d60f3a479"
    assert normalize_layout_recognizer(stale_uuid) == (stale_uuid, None)
