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
"""Community report chunks must tolerate findings that are plain strings.

``CommunityReportsExtractor._get_text_output`` has handled that shape since it was
written, and the schema check only requires ``("findings", list)``, so a list of
strings reaches ``extract_community`` intact.
"""

import json
import logging
from types import SimpleNamespace
from unittest.mock import MagicMock

import networkx as nx
import pytest

import rag.graphrag.general.index as index_module
from rag.graphrag.general.community_reports_extractor import CommunityReportsExtractor, CommunityReportsResult


def _stub_extractor(structure, reports):
    class _Extractor:
        def __init__(self, *_args, **_kwargs):
            pass

        async def __call__(self, *_args, **_kwargs):
            return CommunityReportsResult(output=reports, structured_output=structure)

    return _Extractor


async def _noop(*_args, **_kwargs):
    return {}


async def _run_extract_community(monkeypatch, structure, reports):
    """Drive extract_community far enough to build its chunks, capturing them."""
    captured = {}

    async def fake_insert(chunks, *_args, **_kwargs):
        captured["chunks"] = chunks

    graph = nx.Graph()
    graph.graph["source_id"] = ["doc-1"]

    monkeypatch.setattr(index_module, "load_checkpoints", _noop)
    monkeypatch.setattr(index_module, "cleanup_checkpoints", _noop)
    monkeypatch.setattr(index_module, "save_checkpoint", _noop)
    monkeypatch.setattr(index_module, "CommunityReportsExtractor", _stub_extractor(structure, reports))
    monkeypatch.setattr(index_module, "insert_chunks_bounded", fake_insert)
    monkeypatch.setattr(index_module.settings, "docStoreConn", MagicMock(**{"get_fields.return_value": {}}))

    await index_module.extract_community(graph, "tenant-1", "kb-1", None, object(), object(), lambda **_kwargs: None)
    return captured["chunks"]


_DICT_FINDING = {"summary": "Alpha matters", "explanation": "Because of beta"}


class TestCommunityChunkFindings:
    @pytest.mark.p2
    @pytest.mark.asyncio
    async def test_string_findings_do_not_break_chunk_building(self, monkeypatch, caplog):
        structure = [{"title": "Community", "weight": 1.0, "entities": ["A"], "findings": ["Alpha matters", _DICT_FINDING]}]

        with caplog.at_level(logging.DEBUG, logger="root"):
            chunks = await _run_extract_community(monkeypatch, structure, ["report text"])

        assert len(chunks) == 1
        assert json.loads(chunks[0]["content_with_weight"])["evidences"] == "Because of beta"
        assert "Omitted 1 non-dict findings" in caplog.text

    @pytest.mark.p2
    @pytest.mark.asyncio
    async def test_dict_findings_still_contribute_their_explanations(self, monkeypatch):
        """Guards against a fix that simply drops the evidences field or empties it."""
        structure = [{"title": "Community", "weight": 1.0, "entities": ["A"], "findings": [_DICT_FINDING, {"explanation": "And gamma"}]}]

        chunks = await _run_extract_community(monkeypatch, structure, ["report text"])

        assert json.loads(chunks[0]["content_with_weight"])["evidences"] == "Because of beta\nAnd gamma"

    @pytest.mark.p2
    def test_a_string_finding_still_reaches_the_indexed_text(self):
        """Why skipping them in evidences is not data loss.

        _get_text_output renders a string finding as its own section of the report, and
        the chunk tokenizes report + evidences into content_ltks, so the text is indexed
        either way. Adding it to evidences as well would only duplicate it.
        """
        extractor = CommunityReportsExtractor(SimpleNamespace(llm_name="test-llm", max_length=4096))

        report = extractor._get_text_output({"title": "T", "summary": "S", "findings": ["Alpha matters", _DICT_FINDING]})

        assert "Alpha matters" in report
        assert "Because of beta" in report
