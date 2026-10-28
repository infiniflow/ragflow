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


@pytest.mark.parametrize("api_key", [None, "not-a-valid-sdk-token"])
def test_auth_invalid(integration_settings, api_key):
    unauthorized = RAGFlow(api_key, integration_settings["base_url"])
    with pytest.raises(Exception, match="Invalid access token"):
        unauthorized.create_dataset(name="unauthorized-dataset")


@pytest.mark.parametrize("name", ["a", "a" * 128, "dataset with spaces", "数据集", "name-with_符号"])
def test_name(dataset_factory, client, name):
    dataset = dataset_factory(name=name)
    assert dataset.name == name
    persisted = client.list_datasets(id=dataset.id)
    assert len(persisted) == 1
    assert persisted[0].name == name


@pytest.mark.skip(reason="internal/service/dataset/crud.go CreateDataset uses len(name) bytes despite the documented 128-character limit")
def test_name_unicode_limit(dataset_factory):
    name = "数" * 128
    assert dataset_factory(name=name).name == name


@pytest.mark.parametrize(
    "name,message",
    [
        ("", "required"),
        (" ", "dataset name can't be empty"),
        ("a" * 129, "large than 128"),
    ],
)
def test_name_invalid(client, name, message):
    with pytest.raises(Exception, match=message):
        client.create_dataset(name=name)


@pytest.mark.parametrize("case_sensitive", [True, False], ids=["duplicate", "case-insensitive"])
def test_name_duplicated(dataset_factory, client, case_sensitive):
    name = "SdkDuplicate" + uuid4().hex
    first = dataset_factory(name=name)
    requested = name if case_sensitive else name.lower()
    second = dataset_factory(name=requested)
    assert first.name == name
    assert second.name == requested + "(1)"
    assert first.id != second.id
    assert client.list_datasets(id=second.id)[0].name == second.name


@pytest.mark.p3
def test_create_dataset_1k(dataset_factory, client):
    created = {dataset_factory().id for _ in range(1000)}
    found = set()
    page = 1
    while True:
        datasets = client.list_datasets(page=page, page_size=100)
        found.update(dataset.id for dataset in datasets)
        if len(datasets) < 100:
            break
        page += 1
    assert created <= found


@pytest.mark.p3
def test_create_dataset_concurrent(dataset_factory, client):
    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [executor.submit(dataset_factory) for _ in range(100)]
        datasets = [future.result() for future in futures]
    assert len({dataset.id for dataset in datasets}) == 100
    for dataset in datasets:
        assert client.list_datasets(id=dataset.id)[0].name == dataset.name


def test_embedding_model(dataset_factory, integration_settings, client):
    model = integration_settings.get("embedding_model")
    if not model:
        pytest.fail("RAGFLOW_SDK_EMBEDDING_MODEL must identify an accessible embedding model")
    dataset = dataset_factory(embedding_model=model)
    assert dataset.embedding_model == model
    assert client.list_datasets(id=dataset.id)[0].embedding_model == model


@pytest.mark.parametrize("embedding_model", ["missing-model@MissingProvider", "0123456789abcdef0123456789abcdef"])
def test_embedding_model_invalid(client, embedding_model):
    with pytest.raises(Exception, match="(?i)not found|does not exist|unavailable|not authorized|no permission"):
        client.create_dataset(name="invalid-embedding", embedding_model=embedding_model)


@pytest.mark.parametrize(
    "embedding_model,message",
    [
        ("", "must follow"),
        (" ", "must follow"),
        ("model-without-provider", "must follow"),
        ("@Builtin", "must be non-empty"),
        ("model@", "must be non-empty"),
        (" @Builtin", "must be non-empty"),
        ("model@ ", "must be non-empty"),
    ],
)
def test_embedding_model_format(client, embedding_model, message):
    with pytest.raises(Exception, match=message):
        client.create_dataset(name="invalid-model-format", embedding_model=embedding_model)


@pytest.mark.parametrize("kwargs", [{}, {"embedding_model": None}], ids=["unset", "none"])
def test_embedding_model_default(dataset_factory, client, kwargs):
    dataset = dataset_factory(**kwargs)
    reference = dataset_factory()
    assert dataset.embedding_model
    assert dataset.embedding_model == reference.embedding_model
    persisted = client.list_datasets(id=dataset.id)[0]
    assert persisted.embedding_model == client.list_datasets(id=reference.id)[0].embedding_model


@pytest.mark.parametrize("permission", ["me", "team"])
def test_permission(dataset_factory, client, permission):
    dataset = dataset_factory(permission=permission)
    assert dataset.permission == permission
    assert client.list_datasets(id=dataset.id)[0].permission == permission


@pytest.mark.parametrize("permission", ["", "unknown", "ME", "TEAM"])
def test_permission_invalid(client, permission):
    with pytest.raises(Exception, match="Input should be 'me' or 'team'"):
        client.create_dataset(name="invalid-permission", permission=permission)


def test_permission_unset(dataset_factory):
    assert dataset_factory().permission == "me"


@pytest.mark.parametrize("permission", [" me ", " team "])
def test_permission_normalized(dataset_factory, permission):
    assert dataset_factory(permission=permission).permission == permission.strip()


@pytest.mark.parametrize("language", ["English", " 中文 ", "a" * 32])
def test_language(dataset_factory, client, language):
    dataset = dataset_factory(language=language)
    assert dataset.language == language.strip()
    assert client.list_datasets(id=dataset.id)[0].language == language.strip()


@pytest.mark.parametrize("language,message", [("", "at least 1"), (" ", "at least 1"), ("a" * 33, "at most 32")])
def test_language_invalid(client, language, message):
    with pytest.raises(Exception, match=message):
        client.create_dataset(name="invalid-language", language=language)


@pytest.mark.parametrize("parser_id", ["general", "book", "email", "laws", "manual", "one", "paper", "picture", "presentation", "qa", "table", "audio"])
def test_builtin_parser(dataset_factory, client, parser_id):
    dataset = dataset_factory(parse_type=1, parser_id=parser_id)
    assert dataset.parser_id == parser_id
    assert dataset.parse_type == 1
    assert dataset.pipeline_id is None
    persisted = client.list_datasets(id=dataset.id)[0]
    assert persisted.parser_id == parser_id
    assert persisted.pipeline_id is None


def test_builtin_parser_invalid(client):
    with pytest.raises(Exception, match="input should be"):
        client.create_dataset(name="invalid-parser", parser_id="unknown")


def test_builtin_parser_default(dataset_factory):
    dataset = dataset_factory()
    assert dataset.parser_id == "general"
    assert dataset.parse_type == 1
    assert dataset.pipeline_id is None


@pytest.mark.skip(reason="sdk/python/ragflow_sdk/ragflow.py create_dataset cannot send pipeline_version_id required by internal/service/dataset/crud.go CreateDataset (empty_version_test.go)")
def test_pipeline_parser(dataset_factory, client, integration_settings):
    pipeline_id = integration_settings.get("pipeline_id")
    if not pipeline_id:
        pytest.fail("RAGFLOW_SDK_PIPELINE_ID must identify an accessible versioned ingestion pipeline")
    dataset = dataset_factory(parse_type=2, pipeline_id=pipeline_id)
    assert dataset.pipeline_id == pipeline_id
    assert dataset.parse_type == 2
    persisted = client.list_datasets(id=dataset.id)[0]
    assert persisted.pipeline_id == pipeline_id
    assert persisted.parse_type == 2


@pytest.mark.parametrize("chunk_token_size", [1, 256, 1024, 2048])
def test_parser_config(dataset_factory, client, chunk_token_size):
    config = {"GeneralChunker:SixApplesFall": {"chunk_token_size": chunk_token_size}}
    dataset = dataset_factory(parser_config=DataSet.ParserConfig(client, config))
    assert dataset.parser_config.to_json()["GeneralChunker:SixApplesFall"]["chunk_token_size"] == chunk_token_size
    persisted = client.list_datasets(id=dataset.id)[0]
    assert persisted.parser_config.to_json()["GeneralChunker:SixApplesFall"]["chunk_token_size"] == chunk_token_size


def test_parser_config_parent_child(dataset_factory, client):
    config = {"GeneralChunker:SixApplesFall": {"parent_child": {"use_parent_child": True, "children_delimiter": "|"}}}
    dataset = dataset_factory(parser_config=DataSet.ParserConfig(client, config))
    for result in (dataset, client.list_datasets(id=dataset.id)[0]):
        chunker = result.parser_config.to_json()["GeneralChunker:SixApplesFall"]
        assert chunker["parent_child"] == config["GeneralChunker:SixApplesFall"]["parent_child"]
        assert chunker["children_delimiters"] == ["|"]


def test_parser_config_size(client):
    config = DataSet.ParserConfig(client, {"GeneralChunker:SixApplesFall": {"delimiter": "a" * 65536}})
    with pytest.raises(Exception, match="Parser config exceeds size limit"):
        client.create_dataset(name="oversized-config", parser_config=config)


@pytest.mark.parametrize("config", [None, {}], ids=["none", "empty"])
def test_parser_config_default(dataset_factory, client, config):
    kwargs = {} if config is None else {"parser_config": DataSet.ParserConfig(client, config)}
    dataset = dataset_factory(**kwargs)
    parsed = dataset.parser_config.to_json()
    assert parsed
    assert all(":" in key for key in parsed)
    assert parsed["GeneralChunker:SixApplesFall"]["chunk_token_size"] == 512
    assert client.list_datasets(id=dataset.id)[0].parser_config.to_json() == parsed


def test_parser_config_legacy_keys_ignored(dataset_factory, client):
    config = DataSet.ParserConfig(client, {"chunk_token_num": 1024, "raptor": {"use_raptor": True}, "graphrag": {"use_graphrag": True}})
    dataset = dataset_factory(parser_config=config)
    parsed = dataset.parser_config.to_json()
    assert "chunk_token_num" not in parsed
    assert "raptor" not in parsed
    assert "graphrag" not in parsed
    assert parsed["GeneralChunker:SixApplesFall"]["chunk_token_size"] == 512


@pytest.mark.parametrize("pages", [[[1, 100]], [[1, 10], [5, 20]]])
def test_parser_config_pages(dataset_factory, client, pages):
    config = {"Parser:HipSignsRhyme": {"pdf": {"pages": pages}}}
    dataset = dataset_factory(parser_config=DataSet.ParserConfig(client, config))
    expected = [[1, 100]] if len(pages) == 1 else [[1, 20]]
    assert dataset.parser_config.to_json()["Parser:HipSignsRhyme"]["pdf"]["pages"] == expected
    assert client.list_datasets(id=dataset.id)[0].parser_config.to_json()["Parser:HipSignsRhyme"]["pdf"]["pages"] == expected


@pytest.mark.parametrize("pages", ["1,2", ["1,2"], [["bad", "range"]], [[3, 1]], [[0, 1]]])
def test_parser_config_pages_invalid(client, pages):
    config = DataSet.ParserConfig(client, {"Parser:HipSignsRhyme": {"pdf": {"pages": pages}}})
    with pytest.raises(Exception, match="pages must|invalid page range"):
        client.create_dataset(name="invalid-pages", parser_config=config)
