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

import asyncio
from types import SimpleNamespace

import networkx as nx
import pytest

import rag.graphrag.general.community_reports_extractor as community_reports_module
from rag.graphrag.general.community_reports_extractor import CommunityReportsExtractor
from rag.graphrag.general.graph_extractor import GraphExtractor


def _build_llm_stub():
    return SimpleNamespace(llm_name="test-llm", max_length=4096)


class TestGraphExtractor:
    @pytest.mark.p2
    @pytest.mark.asyncio
    async def test_process_single_content_passes_task_id_to_gleaning_calls(self, monkeypatch):
        extractor = GraphExtractor(_build_llm_stub(), entity_types=["person"])
        extractor.callback = None
        seen_task_ids = []
        responses = iter(["seed-response", "glean-response", "N"])

        async def fake_async_chat(_system, _history, _gen_conf=None, task_id=""):
            seen_task_ids.append(task_id)
            return next(responses)

        monkeypatch.setattr(extractor, "_async_chat", fake_async_chat)
        monkeypatch.setattr(extractor, "_entities_and_relations", lambda *_args, **_kwargs: ({}, {}))

        out_results = []
        await extractor._process_single_content(("chunk-1", "alpha beta"), 0, 1, out_results, task_id="task-123")

        assert seen_task_ids == ["task-123", "task-123", "task-123"]


class TestCommunityReportsExtractor:
    @pytest.mark.p2
    @pytest.mark.asyncio
    async def test_call_does_not_use_outer_timeout_shorter_than_llm_timeout(self, monkeypatch):
        extractor = CommunityReportsExtractor(_build_llm_stub())
        graph = nx.Graph()
        graph.add_node("A", description="alpha")
        graph.add_node("B", description="beta")
        graph.add_edge("A", "B", description="related")

        monkeypatch.setenv("ENABLE_TIMEOUT_ASSERTION", "1")

        original_wait_for = asyncio.wait_for

        def fake_timeout(_seconds, _attempts=2, **_kwargs):
            def decorator(fn):
                async def wrapper(*args, **kwargs):
                    return await original_wait_for(fn(*args, **kwargs), timeout=0.01)

                return wrapper

            return decorator

        async def slow_async_chat(*_args, **_kwargs):
            await asyncio.sleep(0.02)
            return '{"title":"Community","summary":"Summary","findings":[],"rating":1.0,"rating_explanation":"Clear"}'

        monkeypatch.setattr(community_reports_module, "timeout", fake_timeout, raising=False)
        monkeypatch.setattr(
            community_reports_module.leiden,
            "run",
            lambda *_args, **_kwargs: {0: {"0": {"weight": 1.0, "nodes": ["A", "B"]}}},
        )
        monkeypatch.setattr(community_reports_module, "add_community_info2graph", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(extractor, "_async_chat", slow_async_chat)

        result = await extractor(graph)

        assert len(result.structured_output) == 1
        assert result.structured_output[0]["title"] == "Community"

    @staticmethod
    def _extract_with_rating(monkeypatch, extractor, rating_literal):
        report = '{"title":"Community","summary":"Summary","findings":[],"rating":' + rating_literal + ',"rating_explanation":"Clear"}'
        return TestCommunityReportsExtractor._extract_with_response(monkeypatch, extractor, report)

    @staticmethod
    def _extract_with_response(monkeypatch, extractor, response):
        graph = nx.Graph()
        graph.add_node("A", description="alpha")
        graph.add_node("B", description="beta")
        graph.add_edge("A", "B", description="related")

        async def fake_async_chat(*_args, **_kwargs):
            return response

        monkeypatch.setattr(
            community_reports_module.leiden,
            "run",
            lambda *_args, **_kwargs: {0: {"0": {"weight": 1.0, "nodes": ["A", "B"]}}},
        )
        monkeypatch.setattr(community_reports_module, "add_community_info2graph", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(extractor, "_async_chat", fake_async_chat)
        return graph

    @pytest.mark.p2
    @pytest.mark.asyncio
    @pytest.mark.parametrize("rating", ["5", "5.0"], ids=["int", "float"])
    async def test_report_is_kept_whatever_json_number_the_rating_uses(self, monkeypatch, rating):
        """JSON has one number type, so a compliant 0-10 score may arrive as 5 or 5.0.

        json.loads yields an int for the first, and isinstance(5, float) is False, so
        requiring float alone dropped the report on a bare return.
        """
        extractor = CommunityReportsExtractor(_build_llm_stub())
        graph = self._extract_with_rating(monkeypatch, extractor, rating)

        result = await extractor(graph)

        assert len(result.structured_output) == 1
        assert result.structured_output[0]["title"] == "Community"

    @pytest.mark.p2
    @pytest.mark.asyncio
    @pytest.mark.parametrize("response", ["[]", '["a","b"]', "5", "null"], ids=["array", "string_array", "number", "null"])
    async def test_json_that_is_not_an_object_never_reaches_the_schema_check(self, monkeypatch, response):
        """The two re.sub calls above strip everything outside the outermost braces.

        A reply with no braces is reduced to "" and json.loads rejects it, so the
        schema check and the logging below only ever see a dict. Pinning that, because
        it is what makes the field-type log safe.
        """
        extractor = CommunityReportsExtractor(_build_llm_stub())
        graph = self._extract_with_response(monkeypatch, extractor, response)

        result = await extractor(graph)

        assert result.structured_output == []

    @pytest.mark.p2
    @pytest.mark.asyncio
    async def test_a_boolean_rating_is_accepted_rather_than_costing_the_report(self, monkeypatch):
        """isinstance(True, int) is True, so "rating": true passes the widened check.

        Deliberate. Nothing reads the rating, so rejecting the value would discard a
        usable title, summary and findings, which is the loss this check caused in the
        first place. dict_has_keys_with_types treats bool as int elsewhere too, pinned
        by test_graphrag_utils.py.
        """
        extractor = CommunityReportsExtractor(_build_llm_stub())
        graph = self._extract_with_rating(monkeypatch, extractor, "true")

        result = await extractor(graph)

        assert len(result.structured_output) == 1

    @pytest.mark.p2
    @pytest.mark.asyncio
    async def test_report_with_a_non_numeric_rating_is_still_rejected(self, monkeypatch):
        """The rating check must be widened, not switched off.

        Without this case the suite also passes against ("rating", object) and against
        deleting the rating check outright, so it would guard nothing.
        """
        extractor = CommunityReportsExtractor(_build_llm_stub())
        graph = self._extract_with_rating(monkeypatch, extractor, '"high"')

        result = await extractor(graph)

        assert result.structured_output == []
