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
import types
from types import SimpleNamespace

import networkx as nx
import pytest

import rag.graphrag.general.community_reports_extractor as community_reports_module
from rag.graphrag.general.community_reports_extractor import CommunityReportsExtractor
from rag.graphrag.general.graph_extractor import GraphExtractor


def _build_llm_stub():
    return SimpleNamespace(llm_name="test-llm", max_length=4096)


class _RecordingAsyncio(types.ModuleType):
    """`asyncio` as a module sees it, with the timeouts handed to `wait_for` recorded."""

    def __init__(self):
        super().__init__("asyncio")
        self.timeouts = []

    def __getattr__(self, name):
        return getattr(asyncio, name)

    def wait_for(self, awaitable, timeout):
        self.timeouts.append(timeout)
        return asyncio.wait_for(awaitable, timeout)


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


class TestCommunityReportsTimeoutFlag:
    """`ENABLE_TIMEOUT_ASSERTION` decides the per-report LLM timeout.

    It was read with `os.environ.get`, so any non-empty value, `false` and `0`
    included, armed the 180 s timeout.
    """

    async def _report_timeouts(self, monkeypatch, raw):
        extractor = CommunityReportsExtractor(_build_llm_stub())
        graph = nx.Graph()
        graph.add_node("A", description="alpha")
        graph.add_node("B", description="beta")
        graph.add_edge("A", "B", description="related")
        if raw is None:
            monkeypatch.delenv("ENABLE_TIMEOUT_ASSERTION", raising=False)
        else:
            monkeypatch.setenv("ENABLE_TIMEOUT_ASSERTION", raw)

        async def fast_async_chat(*_args, **_kwargs):
            return '{"title":"Community","summary":"Summary","findings":[],"rating":1.0,"rating_explanation":"Clear"}'

        recording = _RecordingAsyncio()
        monkeypatch.setattr(community_reports_module, "asyncio", recording)
        monkeypatch.setattr(
            community_reports_module.leiden,
            "run",
            lambda *_args, **_kwargs: {0: {"0": {"weight": 1.0, "nodes": ["A", "B"]}}},
        )
        monkeypatch.setattr(community_reports_module, "add_community_info2graph", lambda *_args, **_kwargs: None)
        monkeypatch.setattr(extractor, "_async_chat", fast_async_chat)

        await extractor(graph)
        return recording.timeouts

    @pytest.mark.p2
    @pytest.mark.parametrize("raw", ["false", "0", "off", "no", ""])
    async def test_a_disabling_value_leaves_the_report_untimed(self, monkeypatch, raw):
        assert await self._report_timeouts(monkeypatch, raw) == [1000000000]

    @pytest.mark.p2
    async def test_an_unset_flag_leaves_the_report_untimed(self, monkeypatch):
        assert await self._report_timeouts(monkeypatch, None) == [1000000000]

    @pytest.mark.p2
    @pytest.mark.parametrize("raw", ["1", "true", "on"])
    async def test_an_enabling_value_times_the_report(self, monkeypatch, raw):
        assert await self._report_timeouts(monkeypatch, raw) == [180]
