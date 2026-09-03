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
"""Regression tests for RAGTools system prompt forwarding."""

import sys
import types
import warnings

warnings.filterwarnings(
    "ignore",
    message="pkg_resources is deprecated as an API.*",
    category=UserWarning,
)


def _install_cv2_stub_if_unavailable():
    try:
        import cv2  # noqa: F401

        return
    except Exception:
        pass
    stub = types.ModuleType("cv2")
    stub.INTER_LINEAR = 1
    stub.INTER_CUBIC = 2
    stub.BORDER_CONSTANT = 0
    stub.BORDER_REPLICATE = 1
    stub.COLOR_BGR2RGB = 0
    stub.COLOR_BGR2GRAY = 1
    stub.COLOR_GRAY2BGR = 2
    stub.IMREAD_IGNORE_ORIENTATION = 128
    stub.IMREAD_COLOR = 1
    stub.RETR_LIST = 1
    stub.CHAIN_APPROX_SIMPLE = 2

    def _module_getattr(name):
        if name.isupper():
            return 0
        raise RuntimeError(f"cv2.{name} is unavailable in this test environment")

    stub.__getattr__ = _module_getattr
    sys.modules["cv2"] = stub


_install_cv2_stub_if_unavailable()

from rag.advanced_rag.agentic_rag import RAGTools  # noqa: E402


class FakeChatModel:
    max_length = 8192

    def clone(self):
        return self


def test_sys_prompt_uses_configured_system_prompt():
    tools = RAGTools([], FakeChatModel(), system_prompt="Always respond in French.")
    prompt = tools.sys_prompt()
    assert prompt.startswith("Always respond in French.")
    assert "You are a smart agent." in prompt


def test_sys_prompt_falls_back_to_router_prompt_when_no_system_prompt():
    tools = RAGTools([], FakeChatModel())
    prompt = tools.sys_prompt()
    assert prompt.startswith("You are a smart agent.")
