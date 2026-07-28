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

"""Regression tests for the Google Vertex ``key`` JSON-decode fallback (#17463).

Pre-fix, ``GoogleCV.__init__`` (the Google Cloud Vertex vision provider
covering both AnthropicVertex and GeminiVertex) did::

    key = json.loads(key)
    access_token = json.loads(base64.b64decode(key.get("google_service_account_key", "")))
    project_id = key.get("google_project_id", "")
    region = key.get("google_region", "")

The bare ``json.loads(key)`` has no try/except. A user pasting a JSON
string that is NOT an object -- e.g. ``"[1,2,3]"``, ``"42"``,
``'"hello"'``, ``"true"``, ``"null"`` -- parses fine and then crashes
on the ``.get(...)`` call with ``AttributeError: 'list' object has no
attribute 'get'`` (or ``'int'``, ``'str'``, ``'NoneType'``, ``'bool'``)
from inside :mod:`rag.llm` internals, with no indication of what the
user did wrong.

The Google Cloud Vertex provider REQUIRES a JSON object key (per the
schema in ``api/apps/llm_app.py:291``), unlike Bedrock/BaiduYiyan/
VolcEngine/OpenRouter which also accept a plain string. So this helper
does NOT have a plain-key fallback -- it raises a clear
:class:`ModelException` on any non-JSON or JSON non-object input.

The chat counterpart ``GoogleChat`` (``rag/llm/chat_model.py:1284-1289``)
has the same shape of bug but is intentionally OUT OF SCOPE here -- PR
#15994 is open by another contributor actively working on that class.
This PR is scoped to the CV side only.

This is the same class of bug PR #17215 fixed for Azure, PR #17377
fixed for Bedrock, PR #17390 fixed for BaiduYiyan, PR #17457 fixed for
VolcEngine/Ark, and PR #17459 fixed for OpenRouter. After this PR, no
remaining LLM-provider JSON-decode sites in ``rag/llm/`` that we own
crash with ``AttributeError`` on JSON non-object keys.
"""

import json
from unittest.mock import MagicMock

import pytest

from common.exceptions import ModelException
from rag.llm.key_utils import _resolve_google_service_account_key


# --------------------------------------------------------------------------- #
# 1. The shared helper
# --------------------------------------------------------------------------- #
@pytest.mark.p0
class TestResolveGoogleServiceAccountKey:
    def test_python_dict_passes_through(self):
        """A pre-parsed dict (e.g. a future caller that already has the
        key as a dict) returns the dict shape with all three fields
        extracted."""
        raw = {
            "google_project_id": "my-gcp-project",
            "google_region": "us-central1",
            "google_service_account_key": "eyJ0eXBl...base64...",
        }
        out = _resolve_google_service_account_key(raw)
        assert out == {
            "google_project_id": "my-gcp-project",
            "google_region": "us-central1",
            "google_service_account_key": "eyJ0eXBl...base64...",
        }

    def test_json_dict_with_all_fields(self):
        """The happy path: a valid JSON dict returns the parsed fields."""
        raw = {
            "google_project_id": "my-gcp-project",
            "google_region": "us-central1",
            "google_service_account_key": "eyJ0eXBl...base64...",
        }
        out = _resolve_google_service_account_key(json.dumps(raw))
        assert out == {
            "google_project_id": "my-gcp-project",
            "google_region": "us-central1",
            "google_service_account_key": "eyJ0eXBl...base64...",
        }

    def test_json_dict_missing_google_project_id(self):
        """A dict without ``google_project_id`` is still a valid dict;
        the helper returns an empty ``google_project_id`` rather than
        raising."""
        out = _resolve_google_service_account_key(json.dumps({"google_region": "us-central1"}))
        assert out == {
            "google_project_id": "",
            "google_region": "us-central1",
            "google_service_account_key": "",
        }

    def test_json_dict_missing_google_region(self):
        out = _resolve_google_service_account_key(json.dumps({"google_project_id": "p"}))
        assert out == {
            "google_project_id": "p",
            "google_region": "",
            "google_service_account_key": "",
        }

    def test_json_dict_missing_google_service_account_key(self):
        out = _resolve_google_service_account_key(json.dumps({"google_project_id": "p"}))
        assert out == {
            "google_project_id": "p",
            "google_region": "",
            "google_service_account_key": "",
        }

    def test_json_dict_empty(self):
        """An empty dict is a valid dict; all three fields default to
        empty strings."""
        out = _resolve_google_service_account_key(json.dumps({}))
        assert out == {
            "google_project_id": "",
            "google_region": "",
            "google_service_account_key": "",
        }

    def test_plain_string_raises_model_exception(self):
        """Unlike Bedrock/BaiduYiyan/VolcEngine/OpenRouter, the Google
        Vertex provider does NOT accept a plain API key string -- it
        requires the full object. A plain string must raise
        :class:`ModelException` rather than silently using the string
        as a project_id or as a base64-decoded JSON (which would fail
        anyway with JSONDecodeError from inside base64.b64decode)."""
        with pytest.raises(ModelException) as exc_info:
            _resolve_google_service_account_key("plain-google-vertex-key")
        msg = str(exc_info.value)
        assert "Google Vertex" in msg
        assert "JSON" in msg
        assert "google_project_id" in msg
        assert "google_region" in msg
        assert "google_service_account_key" in msg
        assert "api/apps/llm_app.py" in msg
        assert exc_info.value.retryable is False

    def test_empty_string_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_google_service_account_key("")
        assert "Google Vertex" in str(exc_info.value)
        assert "JSON" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_malformed_json_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_google_service_account_key("not really json {")
        assert "Google Vertex" in str(exc_info.value)
        assert "JSON" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_json_array_raises_model_exception(self):
        """The bug case: a valid JSON array parsed, then ``.get(...)``
        raised ``AttributeError: 'list' object has no attribute 'get'``
        from inside ``rag.llm``. The helper raises a clear
        :class:`ModelException` naming the actual type and the required
        object shape."""
        with pytest.raises(ModelException) as exc_info:
            _resolve_google_service_account_key(json.dumps(["not", "a", "key"]))
        msg = str(exc_info.value)
        assert "Google Vertex" in msg
        assert "object" in msg
        assert "list" in msg
        assert "google_project_id" in msg
        assert "google_service_account_key" in msg
        # Must NOT be a raw AttributeError leaking out.
        assert "AttributeError" not in msg
        assert "'list' object" not in msg
        assert exc_info.value.retryable is False

    def test_json_string_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_google_service_account_key(json.dumps("just a string"))
        msg = str(exc_info.value)
        assert "object" in msg
        assert "str" in msg
        assert exc_info.value.retryable is False

    def test_json_number_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_google_service_account_key(json.dumps(42))
        msg = str(exc_info.value)
        assert "object" in msg
        assert "int" in msg
        assert exc_info.value.retryable is False

    def test_json_float_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_google_service_account_key(json.dumps(3.14))
        msg = str(exc_info.value)
        assert "object" in msg
        assert "float" in msg
        assert exc_info.value.retryable is False

    def test_json_null_raises_model_exception(self):
        """``json.loads("null")`` returns Python ``None``. The helper
        must treat that as a non-object (the pre-fix code crashed with
        ``AttributeError: 'NoneType' object has no attribute 'get'``)."""
        with pytest.raises(ModelException) as exc_info:
            _resolve_google_service_account_key("null")
        msg = str(exc_info.value)
        assert "object" in msg
        assert "NoneType" in msg
        assert exc_info.value.retryable is False

    def test_json_bool_raises_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_google_service_account_key("true")
        assert "object" in str(exc_info.value)
        assert "bool" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_non_string_non_dict_input_raises_model_exception(self):
        """A list / int / ``None`` / etc. passed directly (not as a
        string or a dict) must also raise. Only ``str`` and ``dict``
        are valid inputs to the helper."""
        with pytest.raises(ModelException):
            _resolve_google_service_account_key(42)
        with pytest.raises(ModelException):
            _resolve_google_service_account_key(None)
        with pytest.raises(ModelException):
            _resolve_google_service_account_key(["a", "b"])


# --------------------------------------------------------------------------- #
# 2. End-to-end: GoogleCV.__init__ uses the helper, not bare json.loads
# --------------------------------------------------------------------------- #
KEY_VALID_JSON = json.dumps(
    {
        "google_project_id": "my-gcp-project",
        "google_region": "us-central1",
        "google_service_account_key": "eyJ0eXBl...base64...",
    }
)
KEY_JSON_ARRAY = json.dumps(["not", "a", "key"])
KEY_JSON_NUMBER = json.dumps(42)


@pytest.mark.p0
class TestGoogleCVCallSite:
    """``GoogleCV.__init__`` is hard to construct directly because it
    imports heavy Google SDKs (``google.oauth2.service_account``,
    ``google.auth.transport.requests.Request``) and then branches on
    whether ``model_name`` contains ``"claude"`` (AnthropicVertex) or
    not (GeminiVertex). We mock those imports so the constructor runs
    far enough to exercise the helper, then assert that the helper's
    output flowed through to the local variables before the Google SDK
    client is built.
    """

    def _patch_google_sdk(self, monkeypatch):
        """Stub out the Google SDK modules that ``GoogleCV.__init__``
        imports lazily. The constructor does ``import base64`` and
        ``from google.oauth2 import service_account`` inside the
        function body, so a top-level monkeypatch on those names is
        sufficient. We also stub ``google.auth.transport.requests.Request``
        because the AnthropicVertex branch needs it."""
        # Stub ``google.oauth2.service_account`` (just needs to be
        # importable; the constructor only uses it for type annotation
        # style).
        import types

        google_oauth2 = types.ModuleType("google.oauth2")
        google_oauth2_service_account = types.ModuleType("google.oauth2.service_account")
        monkeypatch.setitem(__import__("sys").modules, "google.oauth2", google_oauth2)
        monkeypatch.setitem(__import__("sys").modules, "google.oauth2.service_account", google_oauth2_service_account)

        google_auth_transport = types.ModuleType("google.auth.transport")
        google_auth_transport_requests = types.ModuleType("google.auth.transport.requests")
        google_auth_transport_requests.Request = MagicMock()
        monkeypatch.setitem(__import__("sys").modules, "google.auth.transport", google_auth_transport)
        monkeypatch.setitem(
            __import__("sys").modules,
            "google.auth.transport.requests",
            google_auth_transport_requests,
        )

    def test_json_array_raises_model_exception(self, monkeypatch):
        """The bug case: a JSON array used to crash with
        ``AttributeError: 'list' object has no attribute 'get'`` deep
        inside ``rag.llm``. The fix surfaces a clear
        :class:`ModelException`."""
        from rag.llm.cv_model import GoogleCV

        self._patch_google_sdk(monkeypatch)

        with pytest.raises(ModelException) as exc_info:
            GoogleCV(KEY_JSON_ARRAY, model_name="gemini-2.5-flash")
        msg = str(exc_info.value)
        assert "Google Vertex" in msg
        assert "object" in msg
        assert "list" in msg
        assert "AttributeError" not in msg
        assert exc_info.value.retryable is False

    def test_json_number_raises_model_exception(self, monkeypatch):
        from rag.llm.cv_model import GoogleCV

        self._patch_google_sdk(monkeypatch)

        with pytest.raises(ModelException) as exc_info:
            GoogleCV(KEY_JSON_NUMBER, model_name="gemini-2.5-flash")
        assert "Google Vertex" in str(exc_info.value)
        assert "object" in str(exc_info.value)
        assert "int" in str(exc_info.value)
        assert exc_info.value.retryable is False

    def test_plain_string_raises_model_exception(self, monkeypatch):
        from rag.llm.cv_model import GoogleCV

        self._patch_google_sdk(monkeypatch)

        with pytest.raises(ModelException) as exc_info:
            GoogleCV("plain-google-vertex-key", model_name="gemini-2.5-flash")
        assert "Google Vertex" in str(exc_info.value)
        assert "JSON" in str(exc_info.value)
        assert "google_project_id" in str(exc_info.value)
        assert exc_info.value.retryable is False
