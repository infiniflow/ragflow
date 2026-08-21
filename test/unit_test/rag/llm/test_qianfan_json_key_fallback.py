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

"""Regression tests for the BaiduYiyan (Baidu Qianfan) ``key`` JSON-decode
fallback (#17389).

The pre-fix implementation called ``json.loads(key)`` unguarded in
``BaiduYiyanChat.__init__``. A user pasting a plain (non-JSON) Baidu API
key -- e.g. ``"bce-v3/ALTAK-.../..."`` -- caused the provider init to crash
with ``json.decoder.JSONDecodeError: Expecting value: line 1 column 1
(char 0)`` from inside ``rag/llm`` internals, with no indication of what
the user did wrong.

The fix funnels the BaiduYiyan key through
``_resolve_qianfan_credentials`` in :mod:`rag.llm.key_utils`. The helper:

1. Accepts a dict (returned verbatim) or a JSON-string-encoded dict.
2. On non-JSON input, raises ``ModelException`` with a message that names
   the required fields and points at ``conf/models/baidu.json`` for the
   model class.
3. On JSON top-level type that is not a dict (e.g. a list, string, or
   number), raises the same ``ModelException`` family.

The chat class then continues with the existing ``key.get("yiyan_ak", "")``
/ ``key.get("yiyan_sk", "")`` calls -- the field-presence check is
unchanged.
"""

import json
import sys
from unittest.mock import MagicMock, patch

import pytest

from common.exceptions import ModelException
from rag.llm.key_utils import _resolve_qianfan_credentials


# --------------------------------------------------------------------------- #
# 1. The shared helper
# --------------------------------------------------------------------------- #
@pytest.mark.p0
class TestResolveQianfanCredentials:
    def test_plain_baidu_api_key_raises_clear_model_exception(self):
        """A plain Baidu API key (the most common mistake: pasting a
        Qianfan access key directly instead of wrapping it in JSON) must
        raise a clear ModelException with the required schema, NOT a raw
        JSONDecodeError."""
        with pytest.raises(ModelException) as exc_info:
            _resolve_qianfan_credentials("bce-v3/ALTAK-EXAMPLE/EXAMPLE")
        msg = str(exc_info.value)
        assert "BaiduYiyan" in msg
        assert "yiyan_ak" in msg
        assert "yiyan_sk" in msg
        # Must point at the model class reference.
        assert "conf/models/baidu.json" in msg
        # Must not be the raw json library's message.
        assert "JSONDecodeError" not in msg
        assert "Expecting value" not in msg

    def test_invalid_json_string_raises_clear_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_qianfan_credentials("not really json {")
        msg = str(exc_info.value)
        assert "BaiduYiyan" in msg
        assert "yiyan_ak" in msg
        assert "JSON" in msg

    def test_empty_string_raises_clear_model_exception(self):
        with pytest.raises(ModelException):
            _resolve_qianfan_credentials("")

    def test_json_dict_with_all_fields_passes_through(self):
        """The happy path: a valid JSON dict returns the parsed dict so the
        existing ``key.get(\"yiyan_ak\", \"\")`` / ``key.get(\"yiyan_sk\", \"\")``
        calls in BaiduYiyanChat.__init__ keep working unchanged."""
        raw = {"yiyan_ak": "ak_test", "yiyan_sk": "sk_test"}
        out = _resolve_qianfan_credentials(json.dumps(raw))
        assert out == raw

    def test_python_dict_passes_through(self):
        """The helper also accepts a pre-parsed dict (e.g. from a future
        caller that already has the key as a dict)."""
        raw = {"yiyan_ak": "ak_test", "yiyan_sk": "sk_test", "extra": "x"}
        out = _resolve_qianfan_credentials(raw)
        assert out == raw

    def test_json_array_payload_raises_clear_model_exception(self):
        """A valid JSON array is not the expected object shape. The helper
        must raise ModelException (not AttributeError when the caller
        does ``payload.get(\"yiyan_ak\")`` on a list)."""
        with pytest.raises(ModelException) as exc_info:
            _resolve_qianfan_credentials(json.dumps(["not", "a", "key"]))
        msg = str(exc_info.value)
        assert "BaiduYiyan" in msg
        assert "object" in msg
        # Caller should be able to read the type from the message.
        assert "list" in msg

    def test_json_string_payload_raises_clear_model_exception(self):
        """A top-level JSON string is also not the expected object shape."""
        with pytest.raises(ModelException) as exc_info:
            _resolve_qianfan_credentials(json.dumps("just a string"))
        assert "object" in str(exc_info.value)
        assert "str" in str(exc_info.value)

    def test_json_number_payload_raises_clear_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_qianfan_credentials(json.dumps(42))
        assert "object" in str(exc_info.value)

    def test_non_string_non_dict_input_raises_model_exception(self):
        """A list/dict/tuple/etc. passed directly (not as a string) must
        also raise the same clear error. ``isinstance(key, dict)`` is true
        for dicts (happy path), but ints, lists, etc. fall through to the
        else branch and must be rejected."""
        with pytest.raises(ModelException):
            _resolve_qianfan_credentials(42)
        with pytest.raises(ModelException):
            _resolve_qianfan_credentials(None)


# --------------------------------------------------------------------------- #
# 2. End-to-end: BaiduYiyanChat.__init__ uses the helper, not bare json.loads
# --------------------------------------------------------------------------- #
KEY_VALID = json.dumps({"yiyan_ak": "ak_test", "yiyan_sk": "sk_test"})


@pytest.fixture
def mock_qianfan():
    """Stub the ``qianfan`` module so BaiduYiyanChat.__init__ can be
    instantiated without the heavy baidubce SDK (which has a Python 3.13
    regex-compatibility issue unrelated to this fix)."""
    fake_qianfan = MagicMock()
    fake_qianfan.ChatCompletion = MagicMock()
    with patch.dict(sys.modules, {"qianfan": fake_qianfan}):
        yield fake_qianfan


@pytest.mark.p0
def test_baidu_yiyan_chat_raises_model_exception_on_plain_key(mock_qianfan):
    """Constructing BaiduYiyanChat with a plain key raises ModelException
    (via the helper), not JSONDecodeError.

    The happy-path e2e (constructing with a valid JSON key) is intentionally
    not included here: instantiating BaiduYiyanChat requires importing
    the full ``rag.llm.chat_model`` module, which transitively imports
    ``litellm`` and triggers a pre-existing pydantic 2.x / Python 3.13
    incompatibility in the venv (``KeyError: 'pydantic.root_model'``) that
    is unrelated to this fix. The helper-level tests above cover the
    happy path directly, and the e2e test below proves the wiring is
    correct: if the bare ``json.loads(key)`` were still in place, the
    test would see a ``JSONDecodeError`` instead of the helper's
    ``ModelException``.
    """
    with pytest.raises(ModelException) as exc_info:
        from rag.llm.chat_model import BaiduYiyanChat

        BaiduYiyanChat("bce-v3/ALTAK-EXAMPLE/EXAMPLE", "ERNIE-4.0-8K")
    msg = str(exc_info.value)
    assert "BaiduYiyan" in msg
    assert "yiyan_ak" in msg
    # Must NOT be the raw json library's message -- the fix replaces
    # the silent JSONDecodeError with a clear ModelException.
    assert "JSONDecodeError" not in msg
