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

from unittest.mock import patch

from core.config import AppConfig


def test_user_default_llm_defaults(monkeypatch):
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{}, {}]
        cfg = AppConfig()

    llm_cfg = cfg.user_default_llm
    assert llm_cfg.api_key == ""
    assert llm_cfg.factory == ""
    assert llm_cfg.allowed_factories is None
    assert llm_cfg.parsers == (
        "naive:General,qa:Q&A,resume:Resume,manual:Manual,table:Table,"
        "paper:Paper,book:Book,laws:Laws,presentation:Presentation,"
        "picture:Picture,one:One,audio:Audio,email:Email,tag:Tag"
    )

    models = ("embedding_model", "chat_model", "rerank_model", "asr_model", "image2text_model")
    for model in models:
        model_cfg = llm_cfg.default_models[model]
        assert model_cfg.model_dump() == {"name":"", "factory":"", "api_key":"", "base_url":""}


def test_user_default_llm_overrides(monkeypatch):
    expected_models = {
        "chat_model": {
            "name": "chat-name",
            "factory": "chat-factory",
            "api_key": "chat-api-key",
            "base_url": "https://api.chat.com"
        },
        "embedding_model": {
            "name": "embedding-name",
            "factory": "embedding-factory",
            "api_key": "embedding-api-key",
            "base_url": "https://api.embedding.com"
        },
        "rerank_model": {
            "name": "rerank-name",
            "factory": "rerank-factory",
            "api_key": "rerank-api-key",
            "base_url": "https://api.rerank.com"
        },
        "asr_model": {
            "name": "asr-name",
            "factory": "asr-factory",
            "api_key": "asr-api-key",
            "base_url": "https://api.asr.com"
        },
        "image2text_model": {
            "name": "image2text-name",
            "factory": "image2text-factory",
            "api_key": "image2text-api-key",
            "base_url": "https://api.image2text.com"
        }
    }
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{
            "user_default_llm": {
            "factory": "mock-factory",
            "api_key": "mock-api-key",
            "parsers": "mock-parsers",
            "default_models": expected_models}
        }, {}]
        cfg = AppConfig()

    llm_cfg = cfg.user_default_llm
    assert llm_cfg.api_key == "mock-api-key"
    assert llm_cfg.factory == "mock-factory"
    assert llm_cfg.allowed_factories is None
    assert llm_cfg.parsers == "mock-parsers"

    for model_name, model_cfg in llm_cfg.default_models.items():
        expected = expected_models[model_name]
        assert model_cfg.model_dump() == expected
