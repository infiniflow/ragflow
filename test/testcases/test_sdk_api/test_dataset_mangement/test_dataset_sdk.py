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

import pytest
from ragflow_sdk import RAGFlow
from ragflow_sdk.modules.dataset import DataSet


class FakeResponse:
    def __init__(self, payload):
        self._payload = payload

    def json(self):
        return self._payload


class RecordingRAGFlow(RAGFlow):
    def __init__(self, response_data=None, update_data=None):
        super().__init__(api_key="test-key", base_url="http://localhost:9380")
        self.calls = []
        self.response_data = response_data
        self.update_data = update_data

    def post(self, path, json=None, stream=False, files=None):
        self.calls.append(("POST", path, json))
        return FakeResponse({"code": 0, "data": self.response_data})

    def put(self, path, json):
        self.calls.append(("PUT", path, json))
        return FakeResponse({"code": 0, "data": self.update_data})


def test_create_dataset_defaults_to_builtin_general():
    rag = RecordingRAGFlow({"id": "ds-1", "name": "kb", "parser_id": "general"})

    dataset = rag.create_dataset(name="kb")

    assert rag.calls == [
        (
            "POST",
            "/datasets",
            {
                "name": "kb",
                "parse_type": 1,
                "permission": "me",
                "parser_id": "general",
            },
        )
    ]
    assert dataset.parser_id == "general"
    assert dataset.pipeline_id is None
    assert dataset.parse_type == 1


def test_create_dataset_builtin_payload_uses_parser_fields():
    rag = RecordingRAGFlow(
        {
            "id": "ds-1",
            "name": "kb",
            "parser_id": "table",
            "language": "English",
        }
    )
    parser_config = DataSet.ParserConfig(rag, {"GeneralChunker:SixApplesFall": {"chunk_token_size": 512}})

    dataset = rag.create_dataset(
        name="kb",
        parser_id="table",
        embedding_model="BAAI/bge-large-zh-v1.5@BAAI",
        parser_config=parser_config,
        language="English",
    )

    assert rag.calls[0][2] == {
        "name": "kb",
        "parse_type": 1,
        "permission": "me",
        "parser_id": "table",
        "embedding_model": "BAAI/bge-large-zh-v1.5@BAAI",
        "parser_config": {"GeneralChunker:SixApplesFall": {"chunk_token_size": 512}},
        "language": "English",
    }
    assert dataset.parser_id == "table"
    assert dataset.language == "English"


@pytest.mark.parametrize("pipeline_id", ["", "  "])
def test_create_dataset_treats_blank_pipeline_id_as_absent(pipeline_id):
    rag = RecordingRAGFlow({"id": "ds-1", "name": "kb", "parser_id": "general"})

    rag.create_dataset(name="kb", pipeline_id=pipeline_id)

    payload = rag.calls[0][2]
    assert payload["parser_id"] == "general"
    assert "pipeline_id" not in payload


@pytest.mark.parametrize("parser_id", ["", "  "])
def test_create_dataset_treats_blank_parser_id_as_absent_for_pipeline(parser_id):
    rag = RecordingRAGFlow(
        {
            "id": "ds-1",
            "name": "kb",
            "parser_id": "",
            "pipeline_id": "0123456789abcdef0123456789abcdef",
        }
    )

    rag.create_dataset(
        name="kb",
        parse_type=2,
        parser_id=parser_id,
        pipeline_id="0123456789abcdef0123456789abcdef",
    )

    payload = rag.calls[0][2]
    assert "parser_id" not in payload
    assert payload["pipeline_id"] == "0123456789abcdef0123456789abcdef"


@pytest.mark.parametrize(
    ("kwargs", "message"),
    [
        ({"parser_id": 123}, "parser_id must be a string"),
        ({"parse_type": 2, "pipeline_id": 123}, "pipeline_id must be a string"),
    ],
)
def test_create_dataset_rejects_non_string_parser_ids(kwargs, message):
    rag = RecordingRAGFlow({})
    # Exercise the runtime guard used when beartype is unavailable.
    create_dataset = getattr(RAGFlow.create_dataset, "__wrapped__", RAGFlow.create_dataset)

    with pytest.raises(ValueError, match=message):
        create_dataset(rag, name="kb", **kwargs)

    assert rag.calls == []


@pytest.mark.parametrize("parser_id", ["", "  "])
def test_create_dataset_rejects_blank_parser_id_for_builtin(parser_id):
    rag = RecordingRAGFlow({})

    with pytest.raises(ValueError, match="parser_id must not be empty"):
        rag.create_dataset(name="kb", parser_id=parser_id)

    assert rag.calls == []


def test_create_dataset_pipeline_payload():
    rag = RecordingRAGFlow(
        {
            "id": "ds-1",
            "name": "kb",
            "parser_id": "",
            "pipeline_id": "0123456789abcdef0123456789abcdef",
        }
    )

    dataset = rag.create_dataset(
        name="kb",
        parse_type=2,
        pipeline_id="0123456789abcdef0123456789abcdef",
        permission="team",
    )

    assert rag.calls[0][2] == {
        "name": "kb",
        "parse_type": 2,
        "permission": "team",
        "pipeline_id": "0123456789abcdef0123456789abcdef",
    }
    assert dataset.pipeline_id == "0123456789abcdef0123456789abcdef"
    assert dataset.parse_type == 2


@pytest.mark.parametrize(
    ("kwargs", "message"),
    [
        ({"parse_type": 1, "pipeline_id": "p1"}, "pipeline_id must not be set"),
        ({"parse_type": 2, "parser_id": "general", "pipeline_id": "p1"}, "parser_id must not be set"),
        ({"parse_type": 2}, "pipeline_id is required"),
        ({"parse_type": 3}, "parse_type must be"),
    ],
)
def test_create_dataset_rejects_invalid_parser_selection(kwargs, message):
    rag = RecordingRAGFlow({})

    with pytest.raises(ValueError, match=message):
        rag.create_dataset(name="kb", **kwargs)

    assert rag.calls == []


def test_dataset_response_keeps_parser_fields():
    rag = RecordingRAGFlow({})

    dataset = DataSet(
        rag,
        {
            "id": "ds-1",
            "name": "kb",
            "parser_id": "general",
            "pipeline_id": "p1",
            "language": "English",
        },
    )

    assert dataset.parser_id == "general"
    assert dataset.pipeline_id == "p1"
    assert dataset.language == "English"
    assert dataset.parse_type == 2
    assert not hasattr(dataset, "chunk_method")


def test_update_sends_parser_selection_and_refreshes_mode():
    rag = RecordingRAGFlow(update_data={"id": "ds-1", "name": "kb", "parser_id": "table"})
    dataset = DataSet(
        rag,
        {
            "id": "ds-1",
            "name": "kb",
            "parser_id": "general",
            "pipeline_id": "p1",
        },
    )

    dataset.update({"parse_type": 1, "parser_id": "table"})

    assert rag.calls == [("PUT", "/datasets/ds-1", {"parse_type": 1, "parser_id": "table"})]
    assert dataset.parser_id == "table"
    assert dataset.pipeline_id is None
    assert dataset.parse_type == 1


def test_update_rejects_legacy_chunk_method():
    rag = RecordingRAGFlow({})
    dataset = DataSet(rag, {"id": "ds-1", "name": "kb", "parser_id": "general"})

    with pytest.raises(ValueError, match="chunk_method"):
        dataset.update({"chunk_method": "manual"})

    assert rag.calls == []


@pytest.mark.parametrize(
    "field",
    [
        "avatar",
        "description",
        "chunk_method",
        "id",
        "tenant_id",
        "created_by",
        "create_date",
        "create_time",
        "update_date",
        "update_time",
        "document_count",
        "chunk_count",
        "token_num",
        "status",
        "pagerank",
        "auto_metadata_config",
        "unknown_field",
    ],
)
def test_create_rejects_removed_and_server_owned_arguments(field):
    rag = RecordingRAGFlow({})
    with pytest.raises(TypeError, match="unexpected keyword argument"):
        rag.create_dataset(name="kb", **{field: "unsupported"})
    assert rag.calls == []


@pytest.mark.parametrize("parser_config", [None, DataSet.ParserConfig(None, {})])
def test_create_omits_absent_config_and_serializes_explicit_empty_config(parser_config):
    rag = RecordingRAGFlow({"id": "ds-1", "name": "kb"})
    rag.create_dataset(name="kb", parser_config=parser_config)
    payload = rag.calls[0][2]
    if parser_config is None:
        assert "parser_config" not in payload
    else:
        assert payload["parser_config"] == {}
    assert "embedding_model" not in payload
    assert "language" not in payload


@pytest.mark.parametrize("kwargs", [{"name": None}, {"name": 0}, {"name": "kb", "permission": None}])
def test_create_rejects_invalid_types_before_transport(kwargs):
    from beartype.roar import BeartypeCallHintParamViolation

    rag = RecordingRAGFlow({})
    with pytest.raises(BeartypeCallHintParamViolation, match="not instance of str"):
        rag.create_dataset(**kwargs)
    assert rag.calls == []


@pytest.mark.parametrize("ids, delete_all", [(None, False), ([], False), (["ds-1"], False), (None, True), ([], True)])
def test_delete_payload_requires_explicit_delete_all(monkeypatch, ids, delete_all):
    rag = RecordingRAGFlow({})
    calls = []

    def delete(path, json):
        calls.append((path, json))
        return FakeResponse({"code": 0})

    monkeypatch.setattr(rag, "delete", delete)
    assert rag.delete_datasets(ids=ids, delete_all=delete_all) is None
    assert calls == [("/datasets", {"ids": ids, "delete_all": delete_all})]


def test_list_parses_dataset_response_without_losing_parser_config(monkeypatch):
    rag = RecordingRAGFlow({})
    data = {"id": "ds-1", "name": "kb", "parser_id": "general", "permission": "team", "parser_config": {"GeneralChunker:node": {"chunk_token_size": 512}}}
    monkeypatch.setattr(rag, "get", lambda *args, **kwargs: FakeResponse({"code": 0, "data": [data]}))
    datasets = rag.list_datasets(ids=["ds-1"])
    assert len(datasets) == 1
    assert isinstance(datasets[0], DataSet)
    assert datasets[0].id == "ds-1"
    assert datasets[0].permission == "team"
    assert datasets[0].parse_type == 1
    assert datasets[0].parser_config.to_json() == {"GeneralChunker:node": {"chunk_token_size": 512}}


@pytest.mark.parametrize("operation", ["create", "list", "update", "delete"])
def test_api_errors_are_propagated_without_mutating_dataset(monkeypatch, operation):
    rag = RecordingRAGFlow({})
    dataset = DataSet(rag, {"id": "ds-1", "name": "before", "pipeline_id": "p1"})
    error = FakeResponse({"code": 403, "message": "Permission denied", "data": None})
    for method in ("post", "get", "put", "delete"):
        monkeypatch.setattr(rag, method, lambda *args, **kwargs: error)
    operations = {
        "create": lambda: rag.create_dataset(name="kb"),
        "list": rag.list_datasets,
        "update": lambda: dataset.update({"name": "after"}),
        "delete": lambda: rag.delete_datasets(ids=["ds-1"]),
    }
    with pytest.raises(Exception, match="^Permission denied$"):
        operations[operation]()
    assert dataset.name == "before"
    assert dataset.pipeline_id == "p1"


@pytest.mark.parametrize(
    "method, path, payload",
    [
        ("post", "/datasets", {"name": "kb", "parse_type": 1, "parser_id": "general"}),
        ("put", "/datasets/ds-1", {"name": "renamed"}),
        ("delete", "/datasets", {"ids": ["ds-1"], "delete_all": False}),
    ],
)
def test_write_transport_sends_bearer_token_and_json(monkeypatch, method, path, payload):
    import requests

    rag = RAGFlow("secret-test-key", "http://example.invalid:9380")
    calls = []
    response = FakeResponse({"code": 0})

    def send(**kwargs):
        calls.append(kwargs)
        return response

    monkeypatch.setattr(requests, method, send)
    assert getattr(rag, method)(path, json=payload) is response
    assert len(calls) == 1
    assert calls[0]["url"] == "http://example.invalid:9380/api/v1" + path
    assert calls[0]["headers"] == {"Authorization": "Bearer secret-test-key"}
    assert calls[0]["json"] == payload
