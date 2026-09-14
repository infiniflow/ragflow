import asyncio
import sys
from types import SimpleNamespace

import pytest

from common import metadata_utils
from common.metadata_utils import apply_meta_data_filter


@pytest.fixture
def generator_stub(monkeypatch):
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", SimpleNamespace(gen_meta_filter=None))


MANUAL_RED = {"method": "manual", "logic": "and", "manual": [{"key": "color", "op": "=", "value": "red"}]}


def run(coro):
    return asyncio.run(coro)


@pytest.mark.usefixtures("generator_stub")
class TestManualScopeNarrowing:
    def test_filter_hits_intersect_the_base_scope(self):
        metas = {"color": {"red": ["docA", "docB"]}}
        result = run(apply_meta_data_filter(MANUAL_RED, metas, base_doc_ids=["docA"]))
        assert result == ["docA"]

    def test_intersection_preserves_base_order_and_dedupes(self):
        metas = {"color": {"red": ["docA", "docB"]}}
        result = run(apply_meta_data_filter(MANUAL_RED, metas, base_doc_ids=["docB", "docA", "docB"]))
        assert result == ["docB", "docA"]

    def test_base_doc_outside_the_filter_hits_is_dropped(self):
        metas = {"color": {"red": ["docA"]}}
        result = run(apply_meta_data_filter(MANUAL_RED, metas, base_doc_ids=["docA", "docC"]))
        assert result == ["docA"]

    def test_no_intersection_returns_sentinel(self):
        metas = {"color": {"red": ["docB"], "blue": ["docA"]}}
        result = run(apply_meta_data_filter(MANUAL_RED, metas, base_doc_ids=["docA"]))
        assert result == ["-999"]

    def test_no_hit_at_all_with_base_scope_returns_sentinel(self):
        metas = {"color": {"blue": ["docA"]}}
        result = run(apply_meta_data_filter(MANUAL_RED, metas, base_doc_ids=["docA"]))
        assert result == ["-999"]

    def test_without_base_scope_the_hits_define_the_scope(self):
        metas = {"color": {"red": ["docA", "docB"]}}
        result = run(apply_meta_data_filter(MANUAL_RED, metas))
        assert sorted(result) == ["docA", "docB"]

    def test_without_base_scope_no_hit_returns_sentinel(self):
        metas = {"color": {"blue": ["docA"]}}
        result = run(apply_meta_data_filter(MANUAL_RED, metas))
        assert result == ["-999"]

    def test_empty_condition_list_keeps_the_base_scope(self):
        metas = {"color": {"red": ["docA"]}}
        result = run(apply_meta_data_filter({"method": "manual", "logic": "and", "manual": []}, metas, base_doc_ids=["docA", "docB"]))
        assert result == ["docA", "docB"]

    def test_pushdown_route_dedupes_hits_without_base_scope(self, monkeypatch):
        monkeypatch.setattr(metadata_utils, "filter_doc_ids_by_metadata", lambda *_args, **_kwargs: ["docA", "docB", "docA"])
        result = run(apply_meta_data_filter(MANUAL_RED, kb_ids=["kb-1"], metas_loader=dict))
        assert result == ["docA", "docB"]

    def test_pushdown_route_intersects_the_base_scope(self, monkeypatch):
        monkeypatch.setattr(metadata_utils, "filter_doc_ids_by_metadata", lambda *_args, **_kwargs: ["doc-2"])
        result = run(apply_meta_data_filter(MANUAL_RED, kb_ids=["kb-1"], base_doc_ids=["doc-1", "doc-2"], metas_loader=dict))
        assert result == ["doc-2"]


@pytest.mark.usefixtures("generator_stub")
class TestAbsentFilter:
    def test_no_meta_data_filter_returns_base_scope(self):
        result = run(apply_meta_data_filter(None, base_doc_ids=["docA"]))
        assert result == ["docA"]

    def test_no_meta_data_filter_without_base_scope_returns_empty(self):
        assert run(apply_meta_data_filter(None)) == []


def install_async_generator(monkeypatch, generated):
    async def fake_gen_meta_filter(_chat_mdl, _metas, _question, constraints=None):
        return generated

    monkeypatch.setitem(sys.modules, "rag.prompts.generator", SimpleNamespace(gen_meta_filter=fake_gen_meta_filter))


class TestAutoScopeNarrowing:
    def test_generated_conditions_intersect_the_base_scope(self, monkeypatch):
        install_async_generator(monkeypatch, {"conditions": [{"key": "color", "op": "=", "value": "red"}], "logic": "and"})
        monkeypatch.setattr(metadata_utils, "filter_doc_ids_by_metadata", lambda *_args, **_kwargs: ["doc-2"])
        result = run(apply_meta_data_filter({"method": "auto"}, kb_ids=["kb-1"], base_doc_ids=["doc-1", "doc-2"], metas_loader=dict))
        assert result == ["doc-2"]

    def test_generated_conditions_without_intersection_return_sentinel(self, monkeypatch):
        install_async_generator(monkeypatch, {"conditions": [{"key": "color", "op": "=", "value": "red"}], "logic": "and"})
        monkeypatch.setattr(metadata_utils, "filter_doc_ids_by_metadata", lambda *_args, **_kwargs: ["doc-2"])
        result = run(apply_meta_data_filter({"method": "auto"}, kb_ids=["kb-1"], base_doc_ids=["doc-1"], metas_loader=dict))
        assert result == ["-999"]

    def test_generated_conditions_without_hits_return_sentinel_even_without_base_scope(self, monkeypatch):
        install_async_generator(monkeypatch, {"conditions": [{"key": "color", "op": "=", "value": "red"}], "logic": "and"})
        monkeypatch.setattr(metadata_utils, "filter_doc_ids_by_metadata", lambda *_args, **_kwargs: [])
        result = run(apply_meta_data_filter({"method": "auto"}, kb_ids=["kb-1"], metas_loader=dict))
        assert result == ["-999"]

    def test_no_generated_conditions_keep_the_base_scope(self, monkeypatch):
        install_async_generator(monkeypatch, {"conditions": [], "logic": "and"})
        result = run(apply_meta_data_filter({"method": "auto"}, metas={"color": {"red": ["docA"]}}, base_doc_ids=["docA", "docB"]))
        assert result == ["docA", "docB"]

    def test_no_generated_conditions_without_base_scope_return_none(self, monkeypatch):
        install_async_generator(monkeypatch, {"conditions": [], "logic": "and"})
        result = run(apply_meta_data_filter({"method": "auto"}, metas={"color": {"red": ["docA"]}}))
        assert result is None


class TestSemiAutoScopeNarrowing:
    def test_generated_conditions_intersect_the_base_scope(self, monkeypatch):
        install_async_generator(monkeypatch, {"conditions": [{"key": "color", "op": "=", "value": "red"}], "logic": "and"})
        monkeypatch.setattr(metadata_utils, "filter_doc_ids_by_metadata", lambda *_args, **_kwargs: ["doc-2"])
        result = run(
            apply_meta_data_filter(
                {"method": "semi_auto", "semi_auto": ["color"]},
                kb_ids=["kb-1"],
                base_doc_ids=["doc-1", "doc-2"],
                metas_loader=lambda: {"color": {"red": ["doc-2"]}},
            )
        )
        assert result == ["doc-2"]

    def test_generated_conditions_without_intersection_return_sentinel(self, monkeypatch):
        install_async_generator(monkeypatch, {"conditions": [{"key": "color", "op": "=", "value": "red"}], "logic": "and"})
        monkeypatch.setattr(metadata_utils, "filter_doc_ids_by_metadata", lambda *_args, **_kwargs: ["doc-2"])
        result = run(
            apply_meta_data_filter(
                {"method": "semi_auto", "semi_auto": ["color"]},
                kb_ids=["kb-1"],
                base_doc_ids=["doc-1"],
                metas_loader=lambda: {"color": {"red": ["doc-2"]}},
            )
        )
        assert result == ["-999"]

    def test_no_selected_keys_keep_the_base_scope(self, monkeypatch):
        install_async_generator(monkeypatch, {"conditions": [{"key": "color", "op": "=", "value": "red"}], "logic": "and"})
        result = run(apply_meta_data_filter({"method": "semi_auto", "semi_auto": []}, metas={"color": {"red": ["docA"]}}, base_doc_ids=["docA"]))
        assert result == ["docA"]
