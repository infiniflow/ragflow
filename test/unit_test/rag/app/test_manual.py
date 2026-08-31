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

"""Regression tests for the Manual parser legacy-``.doc`` rejection path.

``rag.app.manual.chunk`` routes by filename suffix: legacy ``.doc``
(OLE/CFB compound documents) must be rejected early with a clear
conversion message instead of being passed to python-docx, which can
only parse OOXML ``.docx`` packages.
"""

from __future__ import annotations

import sys
from importlib import import_module
from unittest.mock import MagicMock, patch

import pytest

from common.constants import MAXIMUM_PAGE_NUMBER


@pytest.fixture(scope="module")
def manual_module():
    """Load rag.app.manual with heavy optional dependencies stubbed."""
    stub_names = [
        "deepdoc.parser",
        "deepdoc.parser.figure_parser",
        "deepdoc.parser.utils",
        "docx",
        "api.db.joint_services",
    ]
    original_modules = {name: sys.modules.get(name) for name in stub_names}

    try:
        for name in stub_names:
            sys.modules[name] = MagicMock()
        module = import_module("rag.app.manual")
        yield module
    finally:
        for name, original in original_modules.items():
            if original is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = original


def dummy(prog=None, msg=""):
    pass


def test_legacy_doc_is_rejected_with_conversion_message(manual_module):
    with pytest.raises(NotImplementedError, match="convert"):
        manual_module.chunk(
            "x.doc",
            binary=b"",
            from_page=0,
            to_page=MAXIMUM_PAGE_NUMBER,
            callback=dummy,
        )


def test_docx_does_not_hit_doc_rejection(manual_module):
    parser_instance = MagicMock(return_value=((), []))
    with (
        patch.object(manual_module, "Docx", return_value=parser_instance),
        patch.object(
            manual_module,
            "vision_figure_parser_docx_wrapper",
            return_value=[],
        ),
        patch.object(manual_module, "tokenize_table", return_value=[]),
        patch.object(manual_module, "tokenize", return_value=None),
        patch.object(manual_module, "copy"),
        patch.object(manual_module, "attach_media_context"),
    ):
        result = manual_module.chunk(
            "x.docx",
            binary=b"",
            from_page=0,
            to_page=MAXIMUM_PAGE_NUMBER,
            callback=dummy,
        )

    assert result == []
