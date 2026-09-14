import sys
from types import ModuleType, SimpleNamespace

import pytest

from rag.advanced_rag.harness.tools.navigation import (
    _NAV_SEARCH_MAX_DOCS,
    dataset_navigation_search,
)


@pytest.mark.asyncio
async def test_dataset_navigation_search_ranks_across_all_bound_kbs(monkeypatch):
    kb1 = SimpleNamespace(id="kb-1", tenant_id="tenant-1")
    kb2 = SimpleNamespace(id="kb-2", tenant_id="tenant-2")
    tools = SimpleNamespace(kbs=[kb1, kb2], scoped_doc_ids=lambda doc_scope: doc_scope)

    kb1_items = [{"doc_id": f"kb1-doc-{i}", "score": 0.30 + i * 0.01} for i in range(_NAV_SEARCH_MAX_DOCS)]
    kb2_items = [{"doc_id": "kb2-best", "score": 0.99}]

    async def fake_search_dataset_layers(kb_id, tenant_id, query, mode, top_k, doc_scope):
        assert query == "topic keywords"
        # Diverges from main's "nav_doc" on purpose: routing now descends the
        # compiled nav TREE (BFS beam, `search_nav_tree_descent`) instead of
        # flat-sweeping every nav_doc row. Same hybrid ranking underneath.
        assert mode == "navigation_tree"
        assert top_k == _NAV_SEARCH_MAX_DOCS
        assert doc_scope is None
        if kb_id == kb1.id:
            return True, {"items": kb1_items}
        if kb_id == kb2.id:
            return True, {"items": kb2_items}
        raise AssertionError(f"unexpected kb_id: {kb_id}")

    dataset_api_service = ModuleType("dataset_api_service")
    dataset_api_service.search_dataset_layers = fake_search_dataset_layers
    services_module = ModuleType("api.apps.services")
    services_module.dataset_api_service = dataset_api_service

    monkeypatch.setitem(sys.modules, "api.apps.services", services_module)
    monkeypatch.setitem(sys.modules, "api.apps.services.dataset_api_service", dataset_api_service)

    routed = await dataset_navigation_search(tools, "topic", "keywords")

    assert len(routed) == _NAV_SEARCH_MAX_DOCS
    assert routed[0] == "kb2-best"
    assert "kb1-doc-0" not in routed
