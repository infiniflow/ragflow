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

"""Unit tests for the PipelineChunker agent component (#14773).

These tests cover only the pieces that don't require a live Canvas/Graph:
parameter validation and the parser-id -> module lookup table. Full
end-to-end behavior is intentionally left to higher-level integration tests.
"""

from __future__ import annotations

import sys
from importlib import import_module, reload
from unittest.mock import MagicMock

import pytest

pytestmark = pytest.mark.p2


# The component pulls in api.db.services.file_service (-> quart_auth, peewee,
# the entire backend stack) and rag.app.* (-> deepdoc, OCR, xgboost,
# transformers). None of that is exercised by these unit tests, so replace
# the heavy modules with stubs to keep the test runnable without the full
# runtime environment. We track every key we install and restore the prior
# sys.modules state in teardown_module so the stubs don't leak into other
# test files.
@pytest.fixture(scope="module")
def pipeline_chunker_module():
    """Import pipeline_chunker with rag.app parser modules stubbed locally."""
    stubbed_names = [
        "api.db.services.file_service",
        "deepdoc.vision.ocr",
        "deepdoc.parser.figure_parser",
        "rag.app.picture",
        "rag.app.audio",
        "rag.app.resume",
        "rag.app.naive",
        "rag.app.paper",
        "rag.app.book",
        "rag.app.presentation",
        "rag.app.manual",
        "rag.app.laws",
        "rag.app.qa",
        "rag.app.table",
        "rag.app.one",
        "rag.app.email",
        "rag.app.tag",
    ]
    original_modules = {name: sys.modules.get(name) for name in stubbed_names}

    file_service_stub = MagicMock()
    file_service_stub.FileService = MagicMock()

    try:
        sys.modules["api.db.services.file_service"] = file_service_stub
        for name in stubbed_names[1:]:
            stub = MagicMock()
            stub.chunk = MagicMock(return_value=[{"content_with_weight": "stub"}])
            sys.modules[name] = stub

        module = import_module("agent.component.pipeline_chunker")
        module = reload(module)
        yield module
    finally:
        for name, original in original_modules.items():
            if original is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = original


class TestPipelineChunkerParam:
    """Validate parameter parsing and the strategy whitelist."""

    def test_default_param_validates(self, pipeline_chunker_module):
        """A freshly constructed param object should pass ``check()``."""
        p = pipeline_chunker_module.PipelineChunkerParam()
        assert p.check() is True

    def test_accepts_each_known_parser(self, pipeline_chunker_module):
        """Every parser id in the lookup table must validate."""
        for parser_id in pipeline_chunker_module._PARSER_MODULES:
            p = pipeline_chunker_module.PipelineChunkerParam()
            p.parser_id = parser_id
            assert p.check() is True

    def test_rejects_unknown_parser(self, pipeline_chunker_module):
        """Unknown parser ids must raise ``ValueError`` at validation time."""
        p = pipeline_chunker_module.PipelineChunkerParam()
        p.parser_id = "nonsense-parser"
        with pytest.raises(ValueError):
            p.check()

    def test_rejects_non_dict_parser_config(self, pipeline_chunker_module):
        """``parser_config`` must be a dict; anything else must raise."""
        p = pipeline_chunker_module.PipelineChunkerParam()
        p.parser_config = "not a dict"
        with pytest.raises(ValueError):
            p.check()

    def test_rejects_negative_pages(self, pipeline_chunker_module):
        """Negative page indices must raise ``ValueError``."""
        p = pipeline_chunker_module.PipelineChunkerParam()
        p.from_page = -1
        with pytest.raises(ValueError):
            p.check()

    def test_rejects_inverted_page_range(self, pipeline_chunker_module):
        """``from_page`` greater than ``to_page`` must raise ``ValueError``."""
        p = pipeline_chunker_module.PipelineChunkerParam()
        p.from_page = 10
        p.to_page = 5
        with pytest.raises(ValueError, match="from_page must be <= to_page"):
            p.check()


class TestLoadChunker:
    """Verify the lazy parser-id -> chunker callable resolver."""

    def test_load_chunker_returns_callable_for_each_known_parser(self, pipeline_chunker_module):
        """Every known parser id should resolve to a callable ``chunk`` function."""
        for parser_id in pipeline_chunker_module._PARSER_MODULES:
            chunker = pipeline_chunker_module._load_chunker(parser_id)
            assert callable(chunker)

    def test_load_chunker_raises_for_unknown_parser(self, pipeline_chunker_module):
        """Unknown parser ids should raise ``KeyError`` from the lookup."""
        with pytest.raises(KeyError):
            pipeline_chunker_module._load_chunker("not-a-real-parser")
