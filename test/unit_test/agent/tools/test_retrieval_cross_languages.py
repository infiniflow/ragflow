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
"""Regression tests for issue #18417.

The agent Retrieval tool has two execution paths selected by
``retrieval_from``: ``dataset`` and ``memory``. The ``cross_languages``
setting is applied to the query before search in the dataset path
(``_retrieve_kb``) but was silently ignored in the memory path
(``_retrieve_memory``) -- the setting was accepted and persisted on the
tool's parameter, read back unchanged by the API, and produced no
translation at invocation time. Same class of bug as #18413 (memory
system_prompt) and #18415 (memory valid_at): a parameter that is
accepted, persisted, returned by the API, but not applied.

These tests pin the memory-mode ``cross_languages`` translation on the
same conditions the dataset-mode path already honours, so the two paths
stay in lockstep.
"""

import asyncio
from types import SimpleNamespace

import pytest

import agent.tools.retrieval as retrieval_module
from agent.tools.retrieval import Retrieval, RetrievalParam


class _StubMessage:
    """Dict-shaped stand-in for a memory search result row.

    ``memory_prompt`` reads the message via ``message["content"]``, so the
    stub must support subscript access rather than only attribute access.
    """

    def __init__(self, content):
        self._content = content

    def __getitem__(self, key):
        if key == "content":
            return self._content
        raise KeyError(key)


def _make_memory(memory_id="mem-1", tenant_id="tenant-1", embd_id="embd-1"):
    return SimpleNamespace(id=memory_id, tenant_id=tenant_id, embd_id=embd_id)


def _make_tool(cross_languages=None, memory_ids=None, empty_response=""):
    tool = Retrieval.__new__(Retrieval)
    param = RetrievalParam()
    param.cross_languages = list(cross_languages) if cross_languages is not None else []
    param.memory_ids = list(memory_ids) if memory_ids is not None else ["mem-1"]
    param.user_id = ""
    param.empty_response = empty_response
    param.similarity_threshold = 0.2
    param.keywords_similarity_weight = 0.5
    param.top_n = 8
    tool._param = param
    tool.check_if_canceled = lambda *args, **kwargs: False

    outputs = {}

    def _set_output(key, value):
        outputs[key] = value

    tool.set_output = _set_output
    tool._outputs = outputs
    return tool, outputs


def test_cross_languages_is_applied_to_query_in_memory_mode(monkeypatch):
    """The cross_languages call must run with the memory's tenant_id and
    the original query, and the translated query must reach
    ``memory_message_service.query_message``.
    """
    tool, _ = _make_tool(cross_languages=["English"], memory_ids=["mem-1"])
    memory = _make_memory(memory_id="mem-1", tenant_id="tenant-42")
    translated_query = "bonjour le monde"

    cross_calls = []
    query_calls = []

    async def fake_cross_languages(tenant_id, llm_id, query, languages):
        cross_calls.append({"tenant_id": tenant_id, "llm_id": llm_id, "query": query, "languages": languages})
        return translated_query

    def fake_query_message(filter_dict, search_kwargs):
        query_calls.append({"filter_dict": filter_dict, "search_kwargs": search_kwargs})
        return [_StubMessage("matching memory content")]

    monkeypatch.setattr(retrieval_module.MemoryService, "get_by_ids", staticmethod(lambda ids: [memory]))
    monkeypatch.setattr(retrieval_module, "cross_languages", fake_cross_languages)
    monkeypatch.setattr(retrieval_module.memory_message_service, "query_message", fake_query_message)

    result = asyncio.run(tool._retrieve_memory("hello world"))

    assert result == "\n".join(retrieval_module.memory_prompt([_StubMessage("matching memory content")], 200000))
    assert len(cross_calls) == 1
    assert cross_calls[0] == {
        "tenant_id": "tenant-42",
        "llm_id": None,
        "query": "hello world",
        "languages": ["English"],
    }
    assert len(query_calls) == 1
    assert query_calls[0]["filter_dict"] == {"memory_id": ["mem-1"]}
    assert query_calls[0]["search_kwargs"]["query"] == translated_query
    assert query_calls[0]["search_kwargs"]["query"] != "hello world"


def test_empty_cross_languages_passes_original_query_through(monkeypatch):
    """An empty ``cross_languages`` list must not call the translator and
    must not modify the query -- backward-compatible with operators who
    never set the option.
    """
    tool, _ = _make_tool(cross_languages=[], memory_ids=["mem-1"])
    memory = _make_memory()

    cross_calls = []
    query_calls = []

    async def fake_cross_languages(*args, **kwargs):
        cross_calls.append((args, kwargs))
        return "should-not-be-called"

    def fake_query_message(filter_dict, search_kwargs):
        query_calls.append(search_kwargs)
        return []

    monkeypatch.setattr(retrieval_module.MemoryService, "get_by_ids", staticmethod(lambda ids: [memory]))
    monkeypatch.setattr(retrieval_module, "cross_languages", fake_cross_languages)
    monkeypatch.setattr(retrieval_module.memory_message_service, "query_message", fake_query_message)

    result = asyncio.run(tool._retrieve_memory("hello world"))

    assert result == ""
    assert cross_calls == []
    assert len(query_calls) == 1
    assert query_calls[0]["query"] == "hello world"


def test_cross_languages_translator_error_falls_back_to_original_query(monkeypatch):
    """``cross_languages`` returns the original query on translator error
    (``**ERROR**`` path in ``rag.prompts.generator``). The memory path
    must forward that returned value to ``query_message`` -- whatever the
    translator returned is what reaches the search.
    """
    tool, _ = _make_tool(cross_languages=["English"], memory_ids=["mem-1"])
    memory = _make_memory()

    cross_calls = []
    query_calls = []

    async def fake_cross_languages(tenant_id, llm_id, query, languages):
        cross_calls.append(query)
        return query

    def fake_query_message(filter_dict, search_kwargs):
        query_calls.append(search_kwargs)
        return []

    monkeypatch.setattr(retrieval_module.MemoryService, "get_by_ids", staticmethod(lambda ids: [memory]))
    monkeypatch.setattr(retrieval_module, "cross_languages", fake_cross_languages)
    monkeypatch.setattr(retrieval_module.memory_message_service, "query_message", fake_query_message)

    asyncio.run(tool._retrieve_memory("hello world"))

    assert cross_calls == ["hello world"]
    assert query_calls[0]["query"] == "hello world"


def test_no_memory_selected_raises_before_translator(monkeypatch):
    """An empty memory list raises before ``cross_languages`` is reached.
    The error path must not depend on a real database lookup.
    """
    tool, _ = _make_tool(cross_languages=["English"], memory_ids=["mem-missing"])

    cross_calls = []

    async def fake_cross_languages(*args, **kwargs):
        cross_calls.append((args, kwargs))
        return "should-not-be-called"

    monkeypatch.setattr(retrieval_module.MemoryService, "get_by_ids", staticmethod(lambda ids: []))
    monkeypatch.setattr(retrieval_module, "cross_languages", fake_cross_languages)
    monkeypatch.setattr(retrieval_module.memory_message_service, "query_message", lambda *a, **k: [])

    with pytest.raises(Exception, match="No memory is selected"):
        asyncio.run(tool._retrieve_memory("hello world"))

    assert cross_calls == []


@pytest.mark.parametrize(
    ("cross_languages", "should_translate"),
    [
        ([], False),
        (["English"], True),
        (["English", "Chinese"], True),
    ],
)
def test_translator_invocation_matches_cross_languages_setting(monkeypatch, cross_languages, should_translate):
    """The translator runs iff ``cross_languages`` is a non-empty list.
    Multi-language settings forward the full list to the translator.
    """
    tool, _ = _make_tool(cross_languages=cross_languages, memory_ids=["mem-1"])
    memory = _make_memory()

    cross_calls = []

    async def fake_cross_languages(tenant_id, llm_id, query, languages):
        cross_calls.append(list(languages))
        return query + " [translated]"

    def fake_query_message(filter_dict, search_kwargs):
        return []

    monkeypatch.setattr(retrieval_module.MemoryService, "get_by_ids", staticmethod(lambda ids: [memory]))
    monkeypatch.setattr(retrieval_module, "cross_languages", fake_cross_languages)
    monkeypatch.setattr(retrieval_module.memory_message_service, "query_message", fake_query_message)

    asyncio.run(tool._retrieve_memory("hello world"))

    if should_translate:
        assert cross_calls == [cross_languages]
    else:
        assert cross_calls == []


def test_memory_mode_uses_first_memory_tenant_id(monkeypatch):
    """The translator receives the first memory's tenant_id, mirroring how
    ``_retrieve_kb`` uses ``kbs[0].tenant_id``. The choice of the first
    memory is the documented contract -- memories are required to share
    one tenant for retrieval to be a sensible operation.
    """
    tool, _ = _make_tool(cross_languages=["English"], memory_ids=["mem-1", "mem-2"])
    memories = [
        _make_memory(memory_id="mem-1", tenant_id="tenant-A"),
        _make_memory(memory_id="mem-2", tenant_id="tenant-B", embd_id="embd-1"),
    ]

    cross_calls = []

    async def fake_cross_languages(tenant_id, llm_id, query, languages):
        cross_calls.append(tenant_id)
        return query

    monkeypatch.setattr(retrieval_module.MemoryService, "get_by_ids", staticmethod(lambda ids: memories))
    monkeypatch.setattr(retrieval_module, "cross_languages", fake_cross_languages)
    monkeypatch.setattr(retrieval_module.memory_message_service, "query_message", lambda *a, **k: [])

    asyncio.run(tool._retrieve_memory("hello world"))

    assert cross_calls == ["tenant-A"]


def test_retrieval_class_is_available_through_dynamic_tool_discovery():
    """Pin the public export so the tool remains importable from the
    agent tool registry. Mirrors the ``test_querit_classes_are_available_...``
    pattern -- catches accidental rename / removal during refactors.
    """
    import agent.tools as tools_package

    assert tools_package.Retrieval is Retrieval
    assert tools_package.RetrievalParam is RetrievalParam
