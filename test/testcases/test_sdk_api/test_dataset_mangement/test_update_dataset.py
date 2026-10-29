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

from concurrent.futures import ThreadPoolExecutor
from uuid import uuid4

import pytest
from ragflow_sdk import DataSet, RAGFlow

pytestmark = pytest.mark.integration


class TestRequest:
    def test_payload_empty(self, add_dataset_func):
        with pytest.raises(Exception, match="no properties were modified"):
            add_dataset_func.update({})


class TestCapability:
    @pytest.mark.p3
    def test_update_dataset_concurrent(self, client, add_dataset_func):
        dataset = add_dataset_func
        names = [f"sdk-concurrent-{uuid4().hex}" for _ in range(10)]
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(dataset.update, {"name": name}) for name in names]
            for future in futures:
                assert future.result().id == dataset.id
        assert client.list_datasets(id=dataset.id)[0].name in names


class TestDatasetUpdate:
    @pytest.mark.parametrize("name", ["a", "a" * 128, "with spaces", "  trimmed  ", "资料"], ids=["min", "max", "spaces", "trim", "bmp"])
    def test_name(self, client, add_dataset_func, name):
        dataset = add_dataset_func
        dataset.update({"name": name})
        assert dataset.name == name.strip()
        assert client.list_datasets(id=dataset.id)[0].name == name.strip()

    @pytest.mark.skip(reason="Go internal/service/dataset/update.go uses len(name) bytes, rejecting documented 128-character BMP names")
    def test_name_unicode_character_limit(self, client, add_dataset_func):
        name = "资" * 128
        add_dataset_func.update({"name": name})
        assert add_dataset_func.name == name
        assert client.list_datasets(id=add_dataset_func.id)[0].name == name

    @pytest.mark.parametrize(
        "name,message",
        [
            ("", "at least 1 character"),
            (" ", "at least 1 character"),
            ("a" * 129, "at most 128 characters"),
            (0, "cannot unmarshal"),
        ],
    )
    def test_name_invalid(self, client, add_dataset_func, name, message):
        dataset = add_dataset_func
        original = dataset.name
        with pytest.raises(Exception, match=message):
            dataset.update({"name": name})
        assert client.list_datasets(id=dataset.id)[0].name == original

    @pytest.mark.parametrize("uppercase", [False, True])
    def test_name_duplicated(self, client, add_datasets_func, uppercase):
        first, second = add_datasets_func[:2]
        original = first.name
        name = second.name.upper() if uppercase else second.name
        with pytest.raises(Exception, match="already exists"):
            first.update({"name": name})
        assert client.list_datasets(id=first.id)[0].name == original

    @pytest.mark.parametrize("avatar", ["data:image/png;base64,iVBORw0KGgo=", "data:image/jpeg;base64,/9j/4AAQ=="])
    def test_avatar(self, client, add_dataset_func, avatar):
        dataset = add_dataset_func
        dataset.update({"avatar": avatar})
        assert dataset.avatar == avatar
        assert client.list_datasets(id=dataset.id)[0].avatar == avatar

    def test_avatar_exceeds_limit_length(self, add_dataset_func):
        with pytest.raises(Exception, match="at most 65535 characters"):
            add_dataset_func.update({"avatar": "a" * 65536})

    @pytest.mark.parametrize(
        "avatar,message",
        [
            ("iVBORw0KGgo=", "missing MIME prefix"),
            ("data:image/png;base64iVBORw0KGgo=", "missing MIME prefix"),
            ("invalid:image/png;base64,iVBORw0KGgo=", "invalid MIME prefix"),
            ("data:image/gif;base64,R0lGODlh", "unsupported MIME type"),
        ],
    )
    def test_avatar_invalid_prefix(self, add_dataset_func, avatar, message):
        with pytest.raises(Exception, match=message):
            add_dataset_func.update({"avatar": avatar})

    @pytest.mark.parametrize(
        "field,value",
        [
            ("avatar", "data:image/png;base64,iVBORw0KGgo="),
            ("description", "description"),
        ],
    )
    def test_clear_optional_text(self, client, add_dataset_func, field, value):
        dataset = add_dataset_func
        dataset.update({field: value})
        dataset.update({field: ""})
        assert getattr(dataset, field) == ""
        assert getattr(client.list_datasets(id=dataset.id)[0], field) == ""

    @pytest.mark.parametrize("description", ["description", "a" * 65535], ids=["text", "max-length"])
    def test_description(self, client, add_dataset_func, description):
        dataset = add_dataset_func
        dataset.update({"description": description})
        assert dataset.description == description
        assert client.list_datasets(id=dataset.id)[0].description == description

    def test_description_exceeds_limit_length(self, add_dataset_func):
        with pytest.raises(Exception, match="at most 65535 characters"):
            add_dataset_func.update({"description": "a" * 65536})

    @pytest.mark.parametrize("language", ["English", " 中文 ", "a" * 32])
    def test_language(self, client, add_dataset_func, language):
        add_dataset_func.update({"language": language})
        assert add_dataset_func.language == language.strip()
        assert client.list_datasets(id=add_dataset_func.id)[0].language == language.strip()

    @pytest.mark.parametrize("language,message", [("", "at least 1"), (" ", "at least 1"), ("a" * 33, "at most 32")])
    def test_language_invalid(self, client, add_dataset_func, language, message):
        before = add_dataset_func.language
        with pytest.raises(Exception, match=message):
            add_dataset_func.update({"language": language})
        assert client.list_datasets(id=add_dataset_func.id)[0].language == before

    def test_embedding_model(self, client, add_dataset_func, integration_settings):
        model = integration_settings.get("embedding_model")
        if not model:
            pytest.fail("Set RAGFLOW_SDK_EMBEDDING_MODEL to an accessible canonical model@instance@provider identifier")
        dataset = add_dataset_func
        dataset.update({"embedding_model": model})
        assert dataset.embedding_model == model
        assert client.list_datasets(id=dataset.id)[0].embedding_model == model

    def test_embedding_model_invalid(self, client, add_dataset_func):
        dataset = add_dataset_func
        original = dataset.embedding_model
        with pytest.raises(Exception, match="not found"):
            dataset.update({"embedding_model": f"sdk-missing-{uuid4().hex}@MissingProvider"})
        assert client.list_datasets(id=dataset.id)[0].embedding_model == original

    @pytest.mark.parametrize(
        "model,message",
        [
            ("", "must follow"),
            (" ", "must follow"),
            ("modelWithoutProvider", "must follow"),
            ("@Builtin", "non-empty"),
            ("model@", "non-empty"),
            (" @Builtin", "non-empty"),
            ("model@ ", "non-empty"),
        ],
    )
    def test_embedding_model_format(self, add_dataset_func, model, message):
        with pytest.raises(Exception, match=message):
            add_dataset_func.update({"embedding_model": model})

    @pytest.mark.parametrize("permission", ["me", "team", " me "])
    def test_permission(self, client, add_dataset_func, permission):
        dataset = add_dataset_func
        dataset.update({"permission": permission})
        assert dataset.permission == permission.strip()
        assert client.list_datasets(id=dataset.id)[0].permission == permission.strip()

    @pytest.mark.parametrize(
        "permission,message",
        [
            ("", "'me' or 'team'"),
            ("unknown", "'me' or 'team'"),
            ([], "cannot unmarshal"),
            ("ME", "'me' or 'team'"),
            ("TEAM", "'me' or 'team'"),
        ],
    )
    def test_permission_invalid(self, add_dataset_func, permission, message):
        with pytest.raises(Exception, match=message):
            add_dataset_func.update({"permission": permission})

    @pytest.mark.parametrize("field", ["name", "embedding_model", "permission", "pagerank"])
    def test_null_scalar_is_not_an_update(self, add_dataset_func, field):
        with pytest.raises(Exception, match="no properties were modified"):
            add_dataset_func.update({field: None})

    @pytest.mark.parametrize(
        "parser_id",
        [
            "general",
            "book",
            "email",
            "laws",
            "manual",
            "one",
            "paper",
            "picture",
            "presentation",
            "qa",
            "table",
        ],
    )
    def test_parser_selection(self, client, add_dataset_func, parser_id):
        dataset = add_dataset_func
        dataset.update({"parse_type": 1, "parser_id": parser_id})
        assert dataset.parser_id == parser_id
        assert dataset.parse_type == 1
        assert dataset.pipeline_id is None
        retrieved = client.list_datasets(id=dataset.id)[0]
        assert retrieved.parser_id == parser_id
        assert retrieved.parse_type == 1
        assert retrieved.pipeline_id is None

    @pytest.mark.parametrize(
        "payload,message",
        [
            ({"parser_id": "qa"}, "parse_type is required"),
            ({"pipeline_id": "12345678901234567890123456789012"}, "parse_type is required"),
            ({"parse_type": 1}, "parser_id is required"),
            ({"parse_type": 2}, "pipeline_id is required"),
            ({"parse_type": 3}, "invalid parse_type"),
            ({"parse_type": 1, "parser_id": "qa", "pipeline_id": "pipeline"}, "pipeline_id must not be set"),
            ({"parse_type": 2, "pipeline_id": "pipeline", "parser_id": "qa"}, "parser_id must not be set"),
            ({"parse_type": 1, "parser_id": "unknown"}, "input should be"),
            ({"parse_type": 1, "parser_id": []}, "cannot unmarshal"),
            ({"parse_type": 1, "parser_id": ""}, "parser_id is required"),
            ({"parse_type": 1, "parser_id": None}, "parser_id is required"),
        ],
    )
    def test_parser_selection_invalid(self, client, add_dataset_func, payload, message):
        dataset = add_dataset_func
        original = dataset.parser_id
        with pytest.raises(Exception, match=message):
            dataset.update(payload)
        assert client.list_datasets(id=dataset.id)[0].parser_id == original

    def test_pipeline_mode_and_switch_back(self, client, add_dataset_func, integration_settings):
        pipeline_id = integration_settings.get("pipeline_id")
        version_id = integration_settings.get("pipeline_version_id")
        if not pipeline_id or not version_id:
            pytest.fail("Set RAGFLOW_SDK_PIPELINE_ID and RAGFLOW_SDK_PIPELINE_VERSION_ID to an accessible versioned ingestion pipeline")
        dataset = add_dataset_func
        dataset.update({"parse_type": 2, "pipeline_id": pipeline_id, "pipeline_version_id": version_id})
        assert dataset.parse_type == 2
        assert dataset.pipeline_id == pipeline_id
        assert client.list_datasets(id=dataset.id)[0].pipeline_id == pipeline_id
        dataset.update({"parse_type": 1, "parser_id": "general"})
        assert dataset.parse_type == 1
        assert dataset.pipeline_id is None
        retrieved = client.list_datasets(id=dataset.id)[0]
        assert retrieved.parse_type == 1
        assert retrieved.pipeline_id is None

    def test_pagerank(self, client, add_dataset_func, integration_settings):
        engine = integration_settings.get("doc_engine")
        if not engine:
            pytest.fail("Set RAGFLOW_SDK_DOC_ENGINE for pagerank capability coverage")
        dataset = add_dataset_func
        if engine == "infinity":
            with pytest.raises(Exception, match="can only be set when doc_engine is elasticsearch"):
                dataset.update({"pagerank": 50})
            assert client.list_datasets(id=dataset.id)[0].pagerank == 0
            return
        if engine not in {"elasticsearch", "oceanbase", "seekdb", "vastbase", "serenedb"}:
            pytest.fail(f"Unrecognized pagerank engine configuration: {engine}")
        for value in [50, 100, 0]:
            dataset.update({"pagerank": value})
            assert dataset.pagerank == value
            assert client.list_datasets(id=dataset.id)[0].pagerank == value

    @pytest.mark.parametrize(
        "pagerank,message",
        [
            pytest.param(
                -1,
                "greater than or equal to 0",
                marks=pytest.mark.skip(reason="internal/service/dataset/update.go reports the upper bound for negative pagerank; preserve the correct lower-bound diagnostic"),
            ),
            (101, "less than or equal to 100"),
            (3.14, "cannot unmarshal"),
            ("invalid", "cannot unmarshal"),
        ],
    )
    def test_pagerank_invalid(self, add_dataset_func, pagerank, message):
        with pytest.raises(Exception, match=message):
            add_dataset_func.update({"pagerank": pagerank})

    @pytest.mark.parametrize("size", [128, 1024])
    def test_component_parser_config(self, client, add_dataset_func, size):
        dataset = add_dataset_func
        key = "GeneralChunker:SixApplesFall"
        dataset.update({"parser_config": {key: {"chunk_token_size": size}}})
        assert dataset.parser_config.to_json()[key]["chunk_token_size"] == size
        assert client.list_datasets(id=dataset.id)[0].parser_config.to_json()[key]["chunk_token_size"] == size

    @pytest.mark.parametrize("config", [{}, None, {"chunk_token_num": 64, "graphrag": {"use_graphrag": True}}])
    def test_parser_config_empty_or_unscoped(self, client, add_dataset_func, config):
        dataset = add_dataset_func
        original = dataset.parser_config.to_json()
        dataset.update({"parser_config": config})
        assert dataset.parser_config.to_json() == original
        assert client.list_datasets(id=dataset.id)[0].parser_config.to_json() == original

    @pytest.mark.parametrize("config", [[], "invalid", 1])
    def test_parser_config_invalid(self, add_dataset_func, config):
        with pytest.raises(Exception, match="cannot unmarshal"):
            add_dataset_func.update({"parser_config": config})

    def test_parser_config_size_limit(self, add_dataset_func):
        with pytest.raises(Exception, match="exceeds size limit"):
            add_dataset_func.update({"parser_config": {"GeneralChunker:SixApplesFall": {"delimiters": ["a" * 65536]}}})

    @pytest.mark.parametrize("config", [{}, None, "omit"])
    def test_parser_config_with_parser_change(self, client, add_dataset_func, config):
        dataset = add_dataset_func
        dataset.update({"parser_config": {"GeneralChunker:SixApplesFall": {"chunk_token_size": 1024}}})
        payload = {"parse_type": 1, "parser_id": "qa"}
        if config != "omit":
            payload["parser_config"] = config
        dataset.update(payload)
        assert dataset.parser_id == "qa"
        assert "GeneralChunker:SixApplesFall" not in dataset.parser_config.to_json()
        retrieved = client.list_datasets(id=dataset.id)[0]
        assert retrieved.parser_id == "qa"
        assert retrieved.parser_config.to_json() == dataset.parser_config.to_json()

    @pytest.mark.parametrize(
        "field",
        [
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
            "unknown_field",
        ],
    )
    def test_field_unsupported(self, add_dataset_func, field):
        with pytest.raises(Exception, match=f"Extra inputs are not permitted: {field}"):
            add_dataset_func.update({field: "unsupported"})

    def test_field_unset(self, client, add_dataset_func):
        dataset = add_dataset_func
        fields = ["avatar", "description", "embedding_model", "permission", "parser_id", "pipeline_id", "pagerank", "language"]
        original = client.list_datasets(id=dataset.id)[0]
        config = original.parser_config.to_json()
        name = f"sdk-renamed-{uuid4().hex}"
        dataset.update({"name": name})
        updated = client.list_datasets(id=dataset.id)[0]
        assert updated.name == name
        for field in fields:
            assert getattr(updated, field) == getattr(original, field), field
        assert updated.parser_config.to_json() == config

    def test_update_requires_authentication(self, client, add_dataset_func, integration_settings):
        dataset = add_dataset_func
        original = dataset.name
        unauthorized = RAGFlow(api_key="invalid-sdk-test-token", base_url=integration_settings["base_url"])
        untrusted_dataset = DataSet(unauthorized, {"id": dataset.id})
        with pytest.raises(Exception, match="(?i)(unauthorized|authorization|api.?key|token|authentication)"):
            untrusted_dataset.update({"name": f"sdk-denied-{uuid4().hex}"})
        assert client.list_datasets(id=dataset.id)[0].name == original
