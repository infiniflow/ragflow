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
from api.db.joint_services import tenant_model_service


def test_ensure_monkeyocr_from_env_requires_apiserver(monkeypatch):
    monkeypatch.setenv("MONKEYOCR_DELETE_OUTPUT", "0")
    monkeypatch.delenv("MONKEYOCR_APISERVER", raising=False)

    called = {"count": 0}

    def _fake_ensure(*_args, **_kwargs):
        called["count"] += 1
        return "monkeyocr-from-env@monkeyocr-from-env@MonkeyOCR"

    monkeypatch.setattr(tenant_model_service, "_ensure_ocr_provider_from_env", _fake_ensure)

    assert tenant_model_service.ensure_monkeyocr_from_env("tenant-1") is None
    assert called["count"] == 0


def test_ensure_monkeyocr_from_env_provisions_when_apiserver_set(monkeypatch):
    monkeypatch.setenv("MONKEYOCR_APISERVER", "http://adapter:9000")

    called = {"config": None}

    def _fake_ensure(_tenant_id, _provider, _model, config):
        called["config"] = config
        return "monkeyocr-from-env@monkeyocr-from-env@MonkeyOCR"

    monkeypatch.setattr(tenant_model_service, "_ensure_ocr_provider_from_env", _fake_ensure)

    result = tenant_model_service.ensure_monkeyocr_from_env("tenant-1")
    assert result.endswith("@MonkeyOCR")
    assert called["config"]["MONKEYOCR_APISERVER"] == "http://adapter:9000"
