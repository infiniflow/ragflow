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
