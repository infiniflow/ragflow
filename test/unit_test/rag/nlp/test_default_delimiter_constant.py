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

"""Pin the single source of truth for the default ``parser_config.delimiter``.

Before #18562 the backend carried four distinct copy-pasted defaults for the
same field: txt/markdown merged with ``"\\n!?;。；！？"``, docx/image/email with
``"\\n!?。；！？"`` (no ASCII ``;``), the ``naive_merge*`` signatures with the
Chinese-only ``"\\n。；！？"``, and the UI/API layer with ``"\\n"``. The
docx/image/email ``;`` omission was user-visible: English semicolon prose
chunked differently (often one oversized chunk) than the same text as
.txt/.md.

These tests pin the unified state:

* ``rag.nlp.delim.DEFAULT_DELIMITER`` is the 8-character set and parses to
  exactly those delimiters.
* Every backend ``parser_config.get("delimiter", ...)`` fallback, kwargs
  default dict, and ``naive_merge*`` / ``RAGFlowTxtParser`` signature default
  references the constant (AST-checked, so a future copy-pasted literal fails
  loudly instead of drifting silently).
* ``rag/app/book.py`` deliberately keeps its own Chinese-only defaults (the
  kwargs default dict and the ``naive_merge`` fallback) — a product decision,
  not drift, pinned here as a carve-out.
* Behaviourally, semicolon-separated English text splits at ``;`` under the
  unified default but stays whole under the pre-fix docx default.
"""

import ast
import inspect
from pathlib import Path

from rag.nlp import naive_merge, naive_merge_docx, naive_merge_with_images
from rag.nlp.delim import DEFAULT_DELIMITER, parse_delimiter_field

REPO_ROOT = Path(__file__).resolve().parents[4]

# Production files whose ``delimiter`` defaults must reference the constant,
# mapped to the exact number of defaults each file carries. Exact per-file
# counts (rather than an aggregate floor) make a deleted fallback or a newly
# copy-pasted literal fail loudly instead of silently re-drifting.
EXPECTED_DELIMITER_DEFAULTS = {
    "rag/app/naive.py": 6,
    "rag/app/email.py": 2,
    "rag/app/laws.py": 1,
    "rag/app/one.py": 1,
    "rag/app/manual.py": 1,
    "rag/app/paper.py": 1,
    "rag/svr/task_executor.py": 1,
    "rag/svr/task_executor_refactor/chunk_service.py": 1,
    "api/db/services/file_service.py": 1,
    "rag/flow/parser/parser.py": 1,
    "deepdoc/parser/txt_parser.py": 2,
    "rag/nlp/__init__.py": 3,
}


def test_constant_value():
    assert DEFAULT_DELIMITER == "\n!?;。；！？"
    # Newline + ASCII !, ?, ; + CJK 。；！？ — 8 delimiters, deduplicated.
    assert parse_delimiter_field(DEFAULT_DELIMITER) == ["\n", "!", "?", ";", "。", "；", "！", "？"]


def _delimiter_defaults(tree):
    """Yield (kind, default_node) for every ``delimiter`` default in the module."""
    for node in ast.walk(tree):
        if (
            isinstance(node, ast.Call)
            and isinstance(node.func, ast.Attribute)
            and node.func.attr == "get"
            and node.args
            and isinstance(node.args[0], ast.Constant)
            and node.args[0].value == "delimiter"
            and len(node.args) > 1
            and isinstance(node.args[1], (ast.Constant, ast.Name))
        ):
            yield ".get", node.args[1]
        if isinstance(node, ast.Dict):
            for key, value in zip(node.keys, node.values):
                if isinstance(key, ast.Constant) and key.value == "delimiter" and isinstance(value, (ast.Constant, ast.Name)):
                    yield "dict", value
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            pairs = list(zip(node.args.args[-len(node.args.defaults) :], node.args.defaults)) if node.args.defaults else []
            pairs += [(a, a.default) for a in node.args.kwonlyargs if a.default is not None]
            for arg, default in pairs:
                if arg.arg == "delimiter" and not (isinstance(default, ast.Constant) and default.value is None):
                    yield f"sig:{node.name}", default


def test_all_backend_defaults_reference_the_constant():
    for relpath, expected in EXPECTED_DELIMITER_DEFAULTS.items():
        tree = ast.parse((REPO_ROOT / relpath).read_text(encoding="utf-8"))
        defaults = list(_delimiter_defaults(tree))
        assert len(defaults) == expected, (
            f"{relpath}: expected exactly {expected} delimiter defaults, found {len(defaults)}; update EXPECTED_DELIMITER_DEFAULTS in the same change so a removed or new default stays loud"
        )
        for kind, default in defaults:
            assert isinstance(default, ast.Name) and default.id == "DEFAULT_DELIMITER", f"{relpath}: '{kind}' delimiter default is {ast.dump(default)}, expected the DEFAULT_DELIMITER constant"


def test_book_keeps_its_deliberate_chinese_only_default():
    """``book`` chunking targets Chinese prose; its defaults are a product choice."""
    tree = ast.parse((REPO_ROOT / "rag/app/book.py").read_text(encoding="utf-8"))
    found = list(_delimiter_defaults(tree))
    kwargs_dict_defaults = [d for kind, d in found if kind == "dict"]
    merge_defaults = [d for kind, d in found if kind == ".get"]
    # Two distinct Chinese-only sets, both deliberate carve-outs from
    # DEFAULT_DELIMITER: the chunk() kwargs default dict ("\n!?。；！？")
    # and the naive_merge fallback ("\n。；！？").
    assert any(isinstance(d, ast.Constant) and d.value == "\n!?。；！？" for d in kwargs_dict_defaults)
    assert any(isinstance(d, ast.Constant) and d.value == "\n。；！？" for d in merge_defaults)


def test_merge_signatures_carry_the_constant():
    for func in (naive_merge, naive_merge_with_images, naive_merge_docx):
        default = inspect.signature(func).parameters["delimiter"].default
        assert default == DEFAULT_DELIMITER, f"{func.__name__} signature default diverges from DEFAULT_DELIMITER"


def test_semicolon_english_text_splits_under_the_unified_default():
    """The reported symptom: ``;`` clauses must split like they do for txt/md."""
    text = "; ".join(
        [
            "The rain fell gently over the quiet town",
            "the sun broke through the grey clouds",
            "the wind stirred the leaves in the yard",
            "children ran along the wet stone path",
            "a distant bell rang twice before noon",
        ]
    )
    sections = [(text, "")]
    unified = naive_merge(sections, chunk_token_num=32, delimiter=DEFAULT_DELIMITER)
    assert len(unified) > 1, "semicolon-separated clauses should not land in a single chunk"

    # Pre-fix docx/image/email default (no ASCII ';'): same text, one blob.
    pre_fix_docx_default = "\n!?。；！？"
    legacy = naive_merge(sections, chunk_token_num=32, delimiter=pre_fix_docx_default)
    assert len(legacy) < len(unified), "the unified default must split semicolon prose more than the ';'less drift default"
