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


class _Canvas:
    def is_canceled(self):
        return False


def _deepl(param=None):
    cpn = DeepL.__new__(DeepL)
    cpn._canvas = _Canvas()
    cpn._param = param or DeepLParam()
    return cpn


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


@pytest.mark.p1
def test_run_returns_error_on_translation_failure():
    cpn = _deepl()
    cpn._param.inputs = {"content": {"value": ["hello"]}}

    with patch.object(DeepL, "get_input", return_value={"content": ["hello"]}):
        with patch("agent.tools.deepl.deepl.Translator") as translator_cls:
            translator_cls.return_value.translate_text.side_effect = RuntimeError("boom")
            result = cpn._run([])

    assert "**Error**:boom" in result.iloc[0]["content"]
