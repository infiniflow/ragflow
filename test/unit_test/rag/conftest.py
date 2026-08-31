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

"""Shared fixtures for ``rag`` unit tests.

Also restores the real ``common.data_source`` package before importing rag
unit tests.

``test/unit_test/data_source/conftest.py`` registers a lightweight
``sys.modules["common.data_source"]`` stub so submodule imports skip the heavy
package ``__init__.py``. Pytest collection order visits ``data_source/`` before
``rag/``, so without this hook ``rag.svr.sync_data_source`` fails on
``from common.data_source import BlobStorageConnector``.
"""

from __future__ import annotations

import importlib
import logging
import sys
import types

import pytest

_LOG = logging.getLogger(__name__)
_PDF_PARSER_KEY = "deepdoc.parser.pdf_parser"


def _restore_common_data_source_package() -> None:
    mod = sys.modules.get("common.data_source")
    if mod is None:
        return
    # Stub is a bare types.ModuleType with __path__ and no __file__; real package has __init__.py.
    if getattr(mod, "__file__", None) is not None:
        return
    if not isinstance(mod, types.ModuleType) or not getattr(mod, "__path__", None):
        return
    keys = [key for key in sys.modules if key == "common.data_source" or key.startswith("common.data_source.")]
    for key in keys:
        del sys.modules[key]
    importlib.invalidate_caches()
    try:
        importlib.import_module("common.data_source")
    except Exception as exc:  # pragma: no cover
        raise ImportError("conftest: failed to restore real common.data_source package") from exc


_restore_common_data_source_package()


def _make_pdf_parser_stub():
    pdf_parser = types.ModuleType(_PDF_PARSER_KEY)

    class _StubPdfParser:
        @staticmethod
        def remove_tag(text):
            return text

    pdf_parser.RAGFlowPdfParser = _StubPdfParser
    return pdf_parser


def _install_pdf_parser_stub() -> None:
    """Install a lightweight stub so ``rag.nlp`` imports without deepdoc/infinity.

    Must run at conftest import time: delimiter and naive_merge tests import
    ``rag.nlp`` at module scope, which is before any fixture runs.
    """
    if _PDF_PARSER_KEY in sys.modules:
        _LOG.debug(
            "pdf_parser_stub: retaining existing module for %s",
            _PDF_PARSER_KEY,
        )
        return
    sys.modules[_PDF_PARSER_KEY] = _make_pdf_parser_stub()
    _LOG.debug("pdf_parser_stub: installed stub for %s", _PDF_PARSER_KEY)


_install_pdf_parser_stub()


@pytest.fixture
def pdf_parser_stub():
    """Ensure the pdf_parser stub is installed for the duration of a test.

    Saves and restores any pre-existing ``sys.modules`` entry so tests that
    opt into this fixture do not permanently replace a real module loaded
    earlier in the session.
    """
    previous = sys.modules.get(_PDF_PARSER_KEY)
    stub = _make_pdf_parser_stub()
    sys.modules[_PDF_PARSER_KEY] = stub
    _LOG.debug("pdf_parser_stub fixture: installed stub for %s", _PDF_PARSER_KEY)
    try:
        yield stub
    finally:
        if previous is None:
            # Keep a stub in place so later module-level imports still work;
            # only restore when a real prior module existed.
            if sys.modules.get(_PDF_PARSER_KEY) is stub:
                pass
            _LOG.debug(
                "pdf_parser_stub fixture: left stub in place for %s",
                _PDF_PARSER_KEY,
            )
        else:
            sys.modules[_PDF_PARSER_KEY] = previous
            _LOG.debug(
                "pdf_parser_stub fixture: restored prior module for %s",
                _PDF_PARSER_KEY,
            )
