import asyncio
import collections
import sys
import types

api_module = types.ModuleType("api")
api_module.__path__ = []
db_module = types.ModuleType("api.db")
db_module.__path__ = []
services_module = types.ModuleType("api.db.services")
services_module.__path__ = []
task_service_module = types.ModuleType("api.db.services.task_service")
task_service_module.has_canceled = lambda *_args, **_kwargs: False

api_module.db = db_module
db_module.services = services_module
services_module.task_service = task_service_module

sys.modules.setdefault("api", api_module)
sys.modules.setdefault("api.db", db_module)
sys.modules.setdefault("api.db.services", services_module)
sys.modules.setdefault("api.db.services.task_service", task_service_module)

import rag.graphrag.general.extractor as extractor_module
import rag.graphrag.general.mind_map_extractor as mind_map_extractor_module
from rag.graphrag.general.mind_map_extractor import MindMapExtractor


class FakeLLM:
    llm_name = "fake-llm"
    max_length = 4096

    async def async_chat(self, system, history: list[dict[str, str]], gen_conf=None, **kwargs):
        return "{}"


class TupleLLM:
    llm_name = "tuple-llm"
    max_length = 4096

    async def async_chat(self, system, history: list[dict[str, str]], gen_conf=None, **kwargs):
        return "{}", 0


def test_mind_map_extractor_accepts_protocol_based_llm():
    extractor = MindMapExtractor(FakeLLM())

    assert extractor._llm.llm_name == "fake-llm"
    assert extractor._llm.max_length == 4096


def test_mind_map_extractor_accepts_tuple_chat_response(monkeypatch):
    extractor = MindMapExtractor(TupleLLM())
    monkeypatch.setattr(extractor_module, "get_llm_cache", lambda *args, **kwargs: None)
    monkeypatch.setattr(extractor_module, "set_llm_cache", lambda *args, **kwargs: None)

    assert extractor._chat("system", [{"role": "user", "content": "Output:"}], {}) == "{}"


def test_mind_map_extractor_todict_supports_list_leaves():
    extractor = MindMapExtractor(FakeLLM())
    layer = collections.OrderedDict(
        {
            "顶层": collections.OrderedDict(
                {
                    "部分A": [
                        "点1",
                        "点2",
                    ]
                }
            )
        }
    )

    assert extractor._todict(layer) == {"顶层": {"部分A": ["点1", "点2"]}}


def test_mind_map_extractor_be_children_supports_list_leaves():
    extractor = MindMapExtractor(FakeLLM())

    assert extractor._be_children(["点1", "点2"], {"顶层"}) == [
        {"id": "点1", "children": []},
        {"id": "点2", "children": []},
    ]


def test_mind_map_extractor_process_document_returns_none(monkeypatch):
    extractor = MindMapExtractor(FakeLLM())
    out_res = []

    async def fake_thread_pool_exec(*args, **kwargs):
        return "# 顶层\n## 部分A\n- 点1\n- 点2"

    monkeypatch.setattr(mind_map_extractor_module, "thread_pool_exec", fake_thread_pool_exec)

    result = asyncio.run(extractor._process_document("课堂纪要", {}, out_res))

    assert result is None
    assert out_res == [{"顶层": {"部分A": ["点1", "点2"]}}]
