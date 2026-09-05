#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
"""Regression tests for #18184.

``test/unit_test/rag/conftest.py`` installs a lightweight stub for
``deepdoc.parser.pdf_parser`` (which only exposes ``RAGFlowPdfParser``)
so the ``rag.nlp`` tests can import without the full deepdoc/infinity
dependency chain. Pre-fix, the runtime modules under
``rag.flow.parser`` imported ``PlainParser`` / ``VisionParser`` from
that same module at module load time, so the chain:

    rag.flow.parser.parser -> rag.flow.parser.utils
        -> deepdoc.parser.figure_parser
        -> deepdoc/parser/__init__.py -> pdf_parser.PlainParser

crashed with ``ImportError: cannot import name 'PlainParser'``.

The fix moves the deepdoc parser-backend imports from module load to
the function-local scope that actually instantiates the backend. The
tests below pin the post-fix contract on each of the two affected
runtime modules via AST inspection: the heavy ``from
deepdoc.parser... import X`` lines must be gone, and the lazy-import
map (or lazy getter) must be in place.
"""

from __future__ import annotations

import ast
import os

import pytest


# The source files we read are stable for the duration of the
# test, but pytest's PytestUnraisableExceptionWarning fires on
# every open() at module load (a CPython 3.13 resource warning
# about FileIO close-on-shutdown). The tests read each file
# once via ``with open(...)`` and explicitly close the handle.
pytestmark = pytest.mark.filterwarnings("ignore::pytest.PytestUnraisableExceptionWarning")


REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))


def _source(rel_path: str) -> str:
    with open(os.path.join(REPO_ROOT, rel_path), encoding="utf-8") as f:
        return f.read()


def _import_names(src: str) -> set:
    """Collect every name bound by a top-level ``from X import Y, Z``.

    Walks the AST and finds every ``ImportFrom`` node at module
    level. Strips ``as`` aliases so ``from X import Y as Y2`` is
    reported as ``{Y}``.
    """
    tree = ast.parse(src)
    names = set()
    for node in tree.body:
        if isinstance(node, ast.ImportFrom) and node.module:
            if node.module.startswith("deepdoc.parser") or node.module.startswith("rag.app.naive"):
                for alias in node.names:
                    names.add(alias.asname or alias.name)
    return names


def test_parser_module_no_eager_deepdoc_imports():
    """``rag/flow/parser/parser.py`` must not eagerly import
    ``PlainParser`` / ``VisionParser`` / etc. from the deepdoc parser
    modules at module load.

    Pre-fix the line was a top-level
    ``from deepdoc.parser.pdf_parser import PlainParser,
    RAGFlowPdfParser, VisionParser`` (plus three more for the other
    parser backends). The fix replaces those with a module-level
    ``_lazy_deepdoc_imports`` map and a ``_lazy`` helper that
    defers the actual ``importlib.import_module`` call until the
    method that uses the backend runs.
    """
    src = _source("rag/flow/parser/parser.py")
    eager = _import_names(src)
    # The lazy map lives at module level; its keys are not
    # ``from ... import`` bindings so they don't appear in
    # ``_import_names``.
    assert "PlainParser" not in eager, "parser.py still imports PlainParser at module load; use _lazy('PlainParser') inside the method instead."
    assert "VisionParser" not in eager, "parser.py still imports VisionParser at module load; use _lazy('VisionParser') inside the method instead."
    assert "RAGFlowPdfParser" not in eager, "parser.py still imports RAGFlowPdfParser at module load; use _lazy('RAGFlowPdfParser') inside the method instead."
    assert "TCADPParser" not in eager, "parser.py still imports TCADPParser at module load; use _lazy('TCADPParser') inside the method instead."
    assert "DoclingParser" not in eager, "parser.py still imports DoclingParser at module load; use _lazy('DoclingParser') inside the method instead."
    assert "ExcelParser" not in eager
    assert "HtmlParser" not in eager
    assert "TxtParser" not in eager
    assert "Docx" not in eager


def test_parser_module_exposes_lazy_import_map():
    """``rag/flow/parser/parser.py`` must define
    ``_lazy_deepdoc_imports`` with the same nine parser backends
    that the eager imports used to bind.

    The map is the single source of truth for the parser backends
    available at runtime. A new parser backend is added by
    extending the map (and the ``_lazy`` helper); no other line in
    parser.py needs to change.
    """
    src = _source("rag/flow/parser/parser.py")
    tree = ast.parse(src)
    # Find the module-level assignment to ``_lazy_deepdoc_imports``.
    found = None
    for node in tree.body:
        if isinstance(node, ast.Assign):
            for target in node.targets:
                if isinstance(target, ast.Name) and target.id == "_lazy_deepdoc_imports":
                    found = node
                    break
        if found:
            break
    assert found is not None, "_lazy_deepdoc_imports map missing"
    assert isinstance(found.value, ast.Dict)
    keys = {k.value for k in found.value.keys if isinstance(k, ast.Constant)}
    expected = {
        "ExcelParser",
        "HtmlParser",
        "TxtParser",
        "DoclingParser",
        "PlainParser",
        "RAGFlowPdfParser",
        "VisionParser",
        "TCADPParser",
        "Docx",
    }
    assert keys == expected, f"map keys = {keys}, expected {expected}"


def test_utils_module_no_eager_vision_figure_parser_import():
    """``rag/flow/parser/utils.py`` must not eagerly import
    ``VisionFigureParser``.

    Pre-fix the line was a top-level
    ``from deepdoc.parser.figure_parser import VisionFigureParser``.
    The eager import was the entry point for the
    ``deepdoc/parser/__init__.py`` -> ``pdf_parser.PlainParser``
    chain that crashed under the conftest stub. Post-fix the
    import is deferred to a ``_get_vision_figure_parser()`` helper
    that is called the first time ``enhance_media_sections_with_vision``
    actually needs the class.
    """
    src = _source("rag/flow/parser/utils.py")
    eager = _import_names(src)
    assert "VisionFigureParser" not in eager, "utils.py still imports VisionFigureParser at module load; use _get_vision_figure_parser() inside the method instead."


def test_utils_module_exposes_lazy_getter():
    """``rag/flow/parser/utils.py`` must define a
    ``_get_vision_figure_parser`` lazy getter.
    """
    src = _source("rag/flow/parser/utils.py")
    tree = ast.parse(src)
    fn_names = {node.name for node in ast.walk(tree) if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef))}
    assert "_get_vision_figure_parser" in fn_names


def test_task_service_no_eager_deepdoc_parser_imports():
    """``api/db/services/task_service.py`` must not eagerly import
    from any deepdoc parser module.

    Pre-fix the file had two top-level imports
    (``from deepdoc.parser import PdfParser`` and
    ``from deepdoc.parser.excel_parser import RAGFlowExcelParser``)
    even though it only used the two static helpers
    ``PdfParser.total_page_number`` and
    ``RAGFlowExcelParser.row_number``. Post-fix both are imported
    inside the two thin helpers
    ``_pdf_parser_total_page_number`` and
    ``_ragflow_excel_parser_row_number`` that wrap the calls.
    """
    src = _source("api/db/services/task_service.py")
    eager = _import_names(src)
    assert "PdfParser" not in eager, "task_service.py still imports PdfParser at module load; use the _pdf_parser_total_page_number helper instead."
    assert "RAGFlowExcelParser" not in eager, "task_service.py still imports RAGFlowExcelParser at module load; use the _ragflow_excel_parser_row_number helper instead."
    # The two helpers must exist.
    tree = ast.parse(src)
    fn_names = {node.name for node in ast.walk(tree) if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef))}
    assert "_pdf_parser_total_page_number" in fn_names
    assert "_ragflow_excel_parser_row_number" in fn_names
