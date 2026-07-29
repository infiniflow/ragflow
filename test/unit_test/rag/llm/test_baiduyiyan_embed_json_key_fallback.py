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

"""Regression tests for the BaiduYiyanEmbed (Baidu Qianfan embed) ``key``
JSON-decode fallback (#17504).

Pre-fix, ``BaiduYiyanEmbed.__init__`` did::

    key = json.loads(key)
    ak = key.get("yiyan_ak", "")
    sk = key.get("yiyan_sk", "")

The bare ``json.loads(key)`` has no try/except. A user pasting a JSON
string that is NOT an object -- e.g. ``"[1,2,3]"``, ``"42"``,
``'"hello"'``, ``"true"``, ``"null"`` -- parses fine and then crashes
on the ``.get(...)`` call with ``AttributeError: 'list' object has no
attribute 'get'`` (or ``'int'``, ``'str'``, ``'NoneType'``, ``'bool'``)
from inside :mod:`rag.llm` internals, with no indication of what the
user did wrong.

The fix wires ``BaiduYiyanEmbed.__init__`` through the existing
``_resolve_qianfan_credentials`` helper in :mod:`rag.llm.key_utils`
(the same helper PR #17390 added for the chat side). The helper:

1. Accepts a dict (returned verbatim) or a JSON-string-encoded dict.
2. On non-JSON input, raises ``ModelException`` with a message that
   names the required fields and points at ``conf/models/baidu.json``
   for the model class.
3. On JSON top-level type that is not a dict (e.g. a list, string, or
   number), raises the same ``ModelException`` family.

The embed class then continues with the existing
``key.get("yiyan_ak", "")`` / ``key.get("yiyan_sk", "")`` calls --
the field-presence check is unchanged.

This is the embed-side companion of PR #17390, which fixed the same
shape of bug on the chat side. The helper itself is the same; only
the call site changes. Test file is named
``test_baiduyiyan_embed_json_key_fallback.py`` (not the chat-side
``test_qianfan_json_key_fallback.py``) to avoid pytest collection
conflicts if both PRs land in the same release.
"""

import json
import sys
from unittest.mock import MagicMock

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
        # Must point at the schema reference.
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
        """The happy path: a valid JSON dict returns the parsed dict so
        the existing ``key.get("yiyan_ak", "")`` / ``.get("yiyan_sk",
        "")`` calls in the model classes keep working unchanged."""
        raw = {"yiyan_ak": "ak_test", "yiyan_sk": "sk_test"}
        out = _resolve_qianfan_credentials(json.dumps(raw))
        assert out == raw

    def test_python_dict_passes_through(self):
        """The helper also accepts a pre-parsed dict (e.g. from a future
        caller that already has the key as a dict)."""
        raw = {"yiyan_ak": "ak_test", "yiyan_sk": "sk_test"}
        out = _resolve_qianfan_credentials(raw)
        assert out == raw

    def test_json_array_payload_raises_clear_model_exception(self):
        """A valid JSON array is not the expected object shape. The helper
        must raise ModelException (not AttributeError when the caller
        does ``payload.get("yiyan_ak")`` on a list)."""
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
        assert "int" in str(exc_info.value)

    def test_non_string_non_dict_input_raises_model_exception(self):
        """A list / int / tuple / etc. passed directly (not as a string)
        must also raise the same clear error. ``isinstance(key, dict)``
        is true for dicts (happy path), but ints, lists, etc. fall
        through to the else branch and must be rejected."""
        with pytest.raises(ModelException):
            _resolve_qianfan_credentials(42)
        with pytest.raises(ModelException):
            _resolve_qianfan_credentials(None)


# --------------------------------------------------------------------------- #
# 2. End-to-end: BaiduYiyanEmbed.__init__ uses the helper, not bare json.loads
# --------------------------------------------------------------------------- #
KEY_VALID = json.dumps({"yiyan_ak": "ak_test", "yiyan_sk": "sk_test"})
KEY_JSON_ARRAY = json.dumps(["not", "a", "key"])
KEY_JSON_NUMBER = json.dumps(42)


def _install_qianfan_stub(monkeypatch):
    """Stub the ``qianfan`` SDK so ``BaiduYiyanEmbed.__init__`` can run
    without the real optional dependency installed.

    PR #17390 noted a pre-existing baidubce/litellm Python 3.13
    incompatibility in this venv; the stub sidesteps it. We only need
    the ``qianfan.Embedding`` symbol -- the helper test exercises the
    ModelException path before the SDK is even touched, so for the
    happy-path e2e we return a MagicMock."""
    qianfan = MagicMock()
    qianfan.Embedding = MagicMock(return_value=MagicMock())
    monkeypatch.setitem(sys.modules, "qianfan", qianfan)


@pytest.mark.p0
class TestBaiduYiyanEmbedCallSite:
    def test_json_dict_constructs_with_parsed_fields(self, monkeypatch):
        """Happy path: a valid JSON dict passes through the helper and
        BaiduYiyanEmbed.__init__ completes (with qianfan.Embedding
        stubbed). The stubbed qianfan.Embedding is called with the
        parsed yiyan_ak and yiyan_sk."""
        from rag.llm.embedding_model import BaiduYiyanEmbed

        _install_qianfan_stub(monkeypatch)
        BaiduYiyanEmbed(KEY_VALID, model_name="embedding-v1")
        # The qianfan SDK was called with the parsed ak + sk.
        called_kwargs = sys.modules["qianfan"].Embedding.call_args.kwargs
        assert called_kwargs["ak"] == "ak_test"
        assert called_kwargs["sk"] == "sk_test"

    def test_json_array_raises_model_exception(self, monkeypatch):
        """The bug case: a JSON array used to crash with
        ``AttributeError: 'list' object has no attribute 'get'`` deep
        inside ``rag.llm``. The fix surfaces a clear
        :class:`ModelException`."""
        from rag.llm.embedding_model import BaiduYiyanEmbed

        _install_qianfan_stub(monkeypatch)
        with pytest.raises(ModelException) as exc_info:
            BaiduYiyanEmbed(KEY_JSON_ARRAY, model_name="embedding-v1")
        msg = str(exc_info.value)
        assert "BaiduYiyan" in msg
        assert "object" in msg
        assert "list" in msg
        assert "AttributeError" not in msg
        assert exc_info.value.retryable is False

    def test_json_number_raises_model_exception(self, monkeypatch):
        from rag.llm.embedding_model import BaiduYiyanEmbed

        _install_qianfan_stub(monkeypatch)
        with pytest.raises(ModelException) as exc_info:
            BaiduYiyanEmbed(KEY_JSON_NUMBER, model_name="embedding-v1")
        assert "BaiduYiyan" in str(exc_info.value)
        assert "object" in str(exc_info.value)
        assert "int" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_plain_baidu_api_key_raises_model_exception(self, monkeypatch):
        """The most common user mistake: pasting a plain Baidu API key
        like ``"bce-v3/ALTAK-.../..."`` into the field. Pre-fix this
        crashed with ``json.decoder.JSONDecodeError: Expecting value``
        from inside ``rag.llm`` internals. The fix raises a clear
        :class:`ModelException`."""
        from rag.llm.embedding_model import BaiduYiyanEmbed

        _install_qianfan_stub(monkeypatch)
        with pytest.raises(ModelException) as exc_info:
            BaiduYiyanEmbed("bce-v3/ALTAK-EXAMPLE/EXAMPLE", model_name="embedding-v1")
        assert "BaiduYiyan" in str(exc_info.value)
        assert "yiyan_ak" in str(exc_info.value)
        assert "yiyan_sk" in str(exc_info.value)
        assert "JSONDecodeError" not in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_json_dict_missing_yiyan_ak_uses_empty_string(self, monkeypatch):
        """A valid dict without ``yiyan_ak`` is still a valid dict; the
        helper returns it and the existing ``key.get("yiyan_ak", "")``
        defaults to empty. This preserves the pre-fix tolerance for
        partial-JSON inputs."""
        from rag.llm.embedding_model import BaiduYiyanEmbed

        _install_qianfan_stub(monkeypatch)
        BaiduYiyanEmbed(json.dumps({"yiyan_sk": "sk_test"}), model_name="embedding-v1")
        called_kwargs = sys.modules["qianfan"].Embedding.call_args.kwargs
        assert called_kwargs["ak"] == ""
        assert called_kwargs["sk"] == "sk_test"
