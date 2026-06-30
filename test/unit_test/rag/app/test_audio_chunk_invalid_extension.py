#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

"""Regression tests for invalid audio chunk extensions."""

from __future__ import annotations

import sys
import types
from importlib import import_module, reload
from unittest.mock import MagicMock

import pytest


@pytest.fixture(scope="module")
def audio_module():
    """Load rag.app.audio with backend and tokenizer dependencies stubbed locally."""

    stub_names = [
        "api",
        "api.db",
        "api.db.services",
        "api.db.services.llm_service",
        "api.db.joint_services",
        "api.db.joint_services.tenant_model_service",
        "rag.nlp",
    ]
    original_modules = {name: sys.modules.get(name) for name in stub_names}

    try:
        for name in ["api", "api.db", "api.db.services", "api.db.joint_services"]:
            sys.modules[name] = types.ModuleType(name)

        llm_service_stub = types.ModuleType("api.db.services.llm_service")
        llm_service_stub.LLMBundle = MagicMock()
        sys.modules["api.db.services.llm_service"] = llm_service_stub

        tenant_model_service_stub = types.ModuleType(
            "api.db.joint_services.tenant_model_service"
        )
        tenant_model_service_stub.get_tenant_default_model_by_type = MagicMock()
        sys.modules["api.db.joint_services.tenant_model_service"] = tenant_model_service_stub

        nlp_stub = types.ModuleType("rag.nlp")
        nlp_stub.rag_tokenizer = types.SimpleNamespace(
            tokenize=str,
            fine_grained_tokenize=str,
        )
        nlp_stub.tokenize = MagicMock()
        sys.modules["rag.nlp"] = nlp_stub

        module = import_module("rag.app.audio")
        module = reload(module)
        yield module
    finally:
        for name, original in original_modules.items():
            if original is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = original


def test_chunk_unsupported_extension_reports_original_error(audio_module):
    callback_messages = []

    def callback(*args, **kwargs):
        callback_messages.append((args, kwargs))

    assert (
        audio_module.chunk(
            "sample.txt",
            b"audio",
            "tenant-id",
            "English",
            callback=callback,
        )
        == []
    )

    assert callback_messages[-1] == (
        (),
        {"prog": -1, "msg": "Extension .txt is not supported yet."},
    )
