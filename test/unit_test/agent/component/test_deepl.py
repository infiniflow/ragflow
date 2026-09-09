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

import pytest

# DeepL component imports the `deepl` SDK at module load; skip where absent.
pytest.importorskip("deepl")

from agent.tools.deepl import DeepL, DeepLParam  # noqa: E402


def _make_tool(param=None):
    # Bypass the canvas-bound __init__ (mirrors test_akshare.py) and stub the
    # canvas-touching helpers so we can exercise _invoke's execution path.
    tool = DeepL.__new__(DeepL)
    tool._param = param or DeepLParam()
    tool.check_if_canceled = lambda *a, **k: False
    out = {}
    tool.set_output = lambda k, v: out.__setitem__(k, v)
    tool.output = lambda k=None: out.get(k) if k else out
    return tool, out


def test_param_instantiates():
    DeepLParam()


def test_check_passes_with_defaults():
    # Regression: check() validated an undefined self.top_n and always raised
    # AttributeError, so a DeepL component could never pass validation.
    DeepLParam().check()


def test_check_rejects_invalid_source_lang():
    param = DeepLParam()
    param.source_lang = "XX"
    with pytest.raises(ValueError):
        param.check()


def test_check_rejects_invalid_target_lang():
    param = DeepLParam()
    param.target_lang = "XX"
    with pytest.raises(ValueError):
        param.check()


def test_meta_exposes_query_parameter():
    # Regression: DeepL extended ComponentBase and defined no `meta`, so it had
    # no get_meta() and crashed agent_with_tools when added to an Agent.
    meta = DeepLParam().get_meta()
    params = meta["function"]["parameters"]
    assert "query" in params["properties"]
    assert "query" in params["required"]


@pytest.mark.p1
def test_invoke_returns_translation_and_sets_formalized_content():
    # Regression for the restored runtime path: DeepL only implemented the
    # legacy _run, so _invoke fell through to ComponentBase._invoke and raised
    # NotImplementedError.
    tool, out = _make_tool()

    with patch("agent.tools.deepl.deepl.Translator") as translator_cls:
        translator_cls.return_value.translate_text.return_value.text = "hello"
        res = tool._invoke(query="你好")

    assert res == "hello"
    assert out["formalized_content"] == "hello"


def test_invoke_passes_configured_languages():
    param = DeepLParam()
    param.source_lang = "ZH"
    param.target_lang = "EN-US"
    tool, _ = _make_tool(param)

    with patch("agent.tools.deepl.deepl.Translator") as translator_cls:
        translate_text = translator_cls.return_value.translate_text
        translate_text.return_value.text = "hello"
        tool._invoke(query="你好")

    translate_text.assert_called_once_with("你好", source_lang="ZH", target_lang="EN-US")


def test_invoke_empty_query_returns_empty():
    # Empty query short-circuits without calling the DeepL SDK.
    tool, out = _make_tool()
    assert tool._invoke(query="") == ""
    assert out.get("formalized_content") == ""


@pytest.mark.p1
def test_invoke_surfaces_error_on_translation_failure():
    tool, out = _make_tool()

    with patch("agent.tools.deepl.deepl.Translator") as translator_cls:
        translator_cls.return_value.translate_text.side_effect = RuntimeError("boom")
        res = tool._invoke(query="你好")

    assert "boom" in res
    assert "boom" in out["_ERROR"]
