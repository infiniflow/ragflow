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
import asyncio
import warnings

import pytest

# xgboost imports pkg_resources and emits a deprecation warning that is promoted
# to error in our pytest configuration; ignore it for this unit test module.
warnings.filterwarnings(
    "ignore",
    message="pkg_resources is deprecated as an API.*",
    category=UserWarning,
)

from api.db.services import dialog_service


class _StubChatModel:
    def __init__(self, outputs):
        self._outputs = outputs
        self.calls = []

    async def async_chat(self, system_prompt, messages, llm_setting):
        idx = len(self.calls)
        if idx >= len(self._outputs):
            raise AssertionError("async_chat called more times than expected")
        self.calls.append(
            {
                "system_prompt": system_prompt,
                "message": messages[0]["content"],
                "llm_setting": llm_setting,
            }
        )
        return self._outputs[idx]


class _StubRetriever:
    def __init__(self, results):
        self._results = results
        self.sql_calls = []

    def sql_retrieval(self, sql, format="json"):
        assert format == "json"
        idx = len(self.sql_calls)
        if idx >= len(self._results):
            raise AssertionError("sql_retrieval called more times than expected")
        self.sql_calls.append(sql)
        return self._results[idx]


@pytest.fixture
def force_es_engine(monkeypatch):
    monkeypatch.setattr(dialog_service.settings, "DOC_ENGINE_INFINITY", False)
    monkeypatch.setattr(dialog_service.settings, "DOC_ENGINE_OCEANBASE", False)


@pytest.mark.p2
def test_use_sql_repairs_missing_source_columns_for_non_aggregate(monkeypatch, force_es_engine):
    retriever = _StubRetriever(
        [
            {
                "columns": [{"name": "product"}],
                "rows": [["desk"], ["monitor"]],
            },
            {
                "columns": [{"name": "doc_id"}, {"name": "docnm_kwd"}, {"name": "product"}],
                "rows": [["doc-1", "products.xlsx", "desk"], ["doc-2", "products.xlsx", "monitor"]],
            },
        ]
    )
    chat_model = _StubChatModel(
        [
            "SELECT product FROM ragflow_tenant",
            "SELECT doc_id, docnm_kwd, product FROM ragflow_tenant",
        ]
    )
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)

    result = asyncio.run(
        dialog_service.use_sql(
            question="show me column of product",
            field_map={"product": "product"},
            tenant_id="tenant-id",
            chat_mdl=chat_model,
            quota=True,
            kb_ids=None,
        )
    )

    assert result is not None
    assert "|product|Source|" in result["answer"]
    assert len(chat_model.calls) == 2
    assert len(retriever.sql_calls) == 2


@pytest.mark.p2
def test_use_sql_keeps_aggregate_flow_without_source_repair(monkeypatch, force_es_engine):
    retriever = _StubRetriever(
        [
            {
                "columns": [{"name": "count(star)"}],
                "rows": [[6]],
            },
        ]
    )
    chat_model = _StubChatModel(
        [
            "SELECT COUNT(*) FROM ragflow_tenant",
        ]
    )
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)

    result = asyncio.run(
        dialog_service.use_sql(
            question="how many rows are there",
            field_map={"product": "product"},
            tenant_id="tenant-id",
            chat_mdl=chat_model,
            quota=True,
            kb_ids=None,
        )
    )

    assert result is not None
    assert "|COUNT(*)|" in result["answer"]
    assert "Source" not in result["answer"]
    assert len(chat_model.calls) == 1
    assert len(retriever.sql_calls) == 1


@pytest.mark.p2
def test_use_sql_source_repair_is_bounded_to_single_retry(monkeypatch, force_es_engine):
    retriever = _StubRetriever(
        [
            {
                "columns": [{"name": "product"}],
                "rows": [["desk"]],
            },
            {
                "columns": [{"name": "product"}],
                "rows": [["desk"]],
            },
        ]
    )
    chat_model = _StubChatModel(
        [
            "SELECT product FROM ragflow_tenant",
            "SELECT product FROM ragflow_tenant WHERE product IS NOT NULL",
        ]
    )
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)

    result = asyncio.run(
        dialog_service.use_sql(
            question="show me column of product",
            field_map={"product": "product"},
            tenant_id="tenant-id",
            chat_mdl=chat_model,
            quota=True,
            kb_ids=None,
        )
    )

    assert result is not None
    assert "|product|" in result["answer"]
    assert "Source" not in result["answer"]
    assert len(chat_model.calls) == 2
    assert len(retriever.sql_calls) == 2
