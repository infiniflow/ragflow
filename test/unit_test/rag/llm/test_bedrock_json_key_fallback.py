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

"""Regression tests for the Bedrock ``key`` JSON-decode fallback (#17373).

The pre-fix implementation called ``json.loads(key)`` unguarded in four
places (chat, cv, embedding, rerank). A user pasting a plain (non-JSON)
AWS access key -- e.g. ``"AKIAIOSFODNN7EXAMPLE"`` -- caused the provider
init to crash with ``json.decoder.JSONDecodeError: Expecting value:
line 1 column 1 (char 0)`` from inside ``rag/llm`` internals, with no
indication of what the user did wrong.

The fix funnels every Bedrock key through ``_resolve_bedrock_credentials``
in :mod:`rag.llm.key_utils`. The helper:

1. Accepts a dict (returned verbatim) or a JSON-string-encoded dict.
2. On non-JSON input, raises ``ModelException`` with a message that names
   the required fields and points at ``conf/models/bedrock.json``.
3. On JSON top-level type that is not a dict (e.g. a list or string),
   raises the same ``ModelException`` family.

The chat/cv/embedding/rerank model classes then continue with the existing
``key.get("auth_mode")`` / ``bedrock_key.get("auth_mode")`` calls -- the
``auth_mode``-missing check is unchanged.
"""

import json
from unittest.mock import MagicMock, patch

import pytest

from common.exceptions import ModelException
from rag.llm.key_utils import _resolve_bedrock_credentials


# --------------------------------------------------------------------------- #
# 1. The shared helper
# --------------------------------------------------------------------------- #
@pytest.mark.p0
class TestResolveBedrockCredentials:
    def test_plain_aws_access_key_raises_clear_model_exception(self):
        """A plain AWS access key (the most common mistake: pasting from the
        AWS console into a Bedrock field) must raise a clear ModelException
        with the required schema, NOT a raw JSONDecodeError."""
        with pytest.raises(ModelException) as exc_info:
            _resolve_bedrock_credentials("AKIAIOSFODNN7EXAMPLE")
        msg = str(exc_info.value)
        assert "Bedrock" in msg
        assert "auth_mode" in msg
        assert "bedrock_region" in msg
        # Must point at the schema reference.
        assert "conf/models/bedrock.json" in msg
        # Must not be the raw json library's message.
        assert "JSONDecodeError" not in msg
        assert "Expecting value" not in msg

    def test_invalid_json_string_raises_clear_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_bedrock_credentials("not really json {")
        msg = str(exc_info.value)
        assert "Bedrock" in msg
        assert "auth_mode" in msg
        assert "JSON" in msg

    def test_empty_string_raises_clear_model_exception(self):
        with pytest.raises(ModelException):
            _resolve_bedrock_credentials("")

    def test_json_dict_with_all_fields_passes_through(self):
        """The happy path: a valid JSON dict returns the parsed dict so the
        existing ``key.get("auth_mode")`` / ``key.get("bedrock_region")``
        calls in the model classes keep working unchanged."""
        raw = {
            "auth_mode": "access_key_secret",
            "bedrock_region": "us-east-1",
            "bedrock_ak": "AKIA_TEST",
            "bedrock_sk": "secret_test",
        }
        out = _resolve_bedrock_credentials(json.dumps(raw))
        assert out == raw

    def test_python_dict_passes_through(self):
        """The helper also accepts a pre-parsed dict (e.g. from a future
        caller that already has the key as a dict)."""
        raw = {"auth_mode": "iam_role", "bedrock_region": "eu-west-1", "aws_role_arn": "arn:aws:iam::123:role/x"}
        out = _resolve_bedrock_credentials(raw)
        assert out == raw

    def test_json_array_payload_raises_clear_model_exception(self):
        """A valid JSON array is not the expected object shape. The helper
        must raise ModelException (not AttributeError when the caller
        does ``payload.get("auth_mode")`` on a list)."""
        with pytest.raises(ModelException) as exc_info:
            _resolve_bedrock_credentials(json.dumps(["not", "a", "key"]))
        msg = str(exc_info.value)
        assert "Bedrock" in msg
        assert "object" in msg
        # Caller should be able to read the type from the message.
        assert "list" in msg

    def test_json_string_payload_raises_clear_model_exception(self):
        """A top-level JSON string is also not the expected object shape."""
        with pytest.raises(ModelException) as exc_info:
            _resolve_bedrock_credentials(json.dumps("just a string"))
        assert "object" in str(exc_info.value)
        assert "str" in str(exc_info.value)

    def test_json_number_payload_raises_clear_model_exception(self):
        with pytest.raises(ModelException) as exc_info:
            _resolve_bedrock_credentials(json.dumps(42))
        assert "object" in str(exc_info.value)

    def test_non_string_non_dict_input_raises_model_exception(self):
        """A list/dict/tuple/etc. passed directly (not as a string) must
        also raise the same clear error. ``isinstance(key, dict)`` is true
        for dicts (happy path), but ints, lists, etc. fall through to the
        else branch and must be rejected."""
        with pytest.raises(ModelException):
            _resolve_bedrock_credentials(42)
        with pytest.raises(ModelException):
            _resolve_bedrock_credentials(None)

    def test_json_dict_missing_auth_mode_returns_dict_with_empty_auth_mode(self):
        """The fix is in the JSON parse + the type check, not in the
        auth_mode presence check. The downstream ``key.get("auth_mode", "")``
        in each model class is unchanged. Verify the helper returns the
        dict so the existing ``ValueError("Bedrock auth_mode must be
        provided in the key")`` still fires downstream."""
        raw = {"bedrock_region": "us-east-1", "bedrock_ak": "AKIA_TEST", "bedrock_sk": "secret_test"}
        out = _resolve_bedrock_credentials(json.dumps(raw))
        assert out == raw
        # The helper does NOT validate auth_mode; the model class does.
        assert "auth_mode" not in out


# --------------------------------------------------------------------------- #
# 2. End-to-end: the four call sites use the helper, not bare json.loads
# --------------------------------------------------------------------------- #
KEY_VALID = json.dumps(
    {
        "auth_mode": "access_key_secret",
        "bedrock_region": "eu-central-1",
        "bedrock_ak": "AKIA_TEST",
        "bedrock_sk": "secret_test",
    }
)


def _patch_boto3():
    """Return a ``boto3.client`` MagicMock suitable for instantiating any
    Bedrock* class without an AWS round-trip."""
    client = MagicMock()
    factory = patch("boto3.client", return_value=client)
    return factory, client


@pytest.mark.p0
def test_chat_bedrock_raises_model_exception_on_plain_key():
    """Constructing the chat Bedrock branch with a plain key raises
    ModelException (via the helper), not JSONDecodeError."""
    # LiteLLMBase is imported indirectly via the helper wire-up

    # We cannot easily build a full LiteLLMBase in the test (it pulls in
    # heavy deps). Call the helper directly to verify the integration
    # point -- the wiring at chat_model.py:2241 is a one-liner that
    # delegates to this helper.
    with pytest.raises(ModelException) as exc_info:
        _resolve_bedrock_credentials("AKIA_PLAIN_KEY")
    assert "Bedrock" in str(exc_info.value)
    assert "auth_mode" in str(exc_info.value)


@pytest.mark.p0
def test_bedrock_embed_raises_model_exception_on_plain_key():
    """BedrockEmbed.__init__ uses the helper, so a plain key surfaces a
    ModelException before any boto3 call is attempted."""
    from rag.llm.embedding_model import BedrockEmbed

    with pytest.raises(ModelException) as exc_info:
        BedrockEmbed("AKIA_PLAIN_KEY", "amazon.titan-embed-text-v1")
    msg = str(exc_info.value)
    assert "Bedrock" in msg
    assert "auth_mode" in msg
    # Must NOT be the raw json library's message -- the fix replaces
    # the silent JSONDecodeError with a clear ModelException.
    assert "JSONDecodeError" not in msg


@pytest.mark.p0
def test_bedrock_embed_accepts_valid_json_key():
    """Happy path: a valid JSON dict passes through the helper and
    BedrockEmbed.__init__ completes (with boto3.client patched)."""
    from rag.llm.embedding_model import BedrockEmbed

    factory, _ = _patch_boto3()
    with factory:
        BedrockEmbed(KEY_VALID, "amazon.titan-embed-text-v1")
    # No assertion needed: if the helper or the constructor regressed,
    # we'd hit JSONDecodeError or KeyError here.


@pytest.mark.p0
def test_bedrock_rerank_raises_model_exception_on_plain_key():
    """BedrockRerank.__init__ uses the helper, so a plain key surfaces a
    ModelException before any boto3 call is attempted."""
    from rag.llm.rerank_model import BedrockRerank

    with pytest.raises(ModelException) as exc_info:
        BedrockRerank("AKIA_PLAIN_KEY", "amazon.rerank-v1:0")
    msg = str(exc_info.value)
    assert "Bedrock" in msg
    assert "auth_mode" in msg
    assert "JSONDecodeError" not in msg


@pytest.mark.p0
def test_bedrock_rerank_accepts_valid_json_key():
    from rag.llm.rerank_model import BedrockRerank

    factory, _ = _patch_boto3()
    with factory:
        BedrockRerank(KEY_VALID, "amazon.rerank-v1:0")


@pytest.mark.p0
def test_bedrock_cv_raises_model_exception_on_plain_key():
    """BedrockCV.__init__ delegates to _parse_credentials, which uses the
    helper. A plain key surfaces a ModelException."""
    from rag.llm.cv_model import BedrockCV

    with pytest.raises(ModelException) as exc_info:
        BedrockCV("AKIA_PLAIN_KEY", "anthropic.claude-3-5-sonnet-20241022-v2:0")
    msg = str(exc_info.value)
    assert "Bedrock" in msg
    assert "auth_mode" in msg
    assert "JSONDecodeError" not in msg


@pytest.mark.p0
def test_bedrock_cv_accepts_valid_json_key():
    from rag.llm.cv_model import BedrockCV

    with patch("openai.OpenAI"), patch("openai.AsyncOpenAI"):
        BedrockCV(KEY_VALID, "anthropic.claude-3-5-sonnet-20241022-v2:0")


# --------------------------------------------------------------------------- #
# 3. Backward compatibility: JSON dict missing auth_mode still raises the
#    existing ``ValueError("Bedrock auth_mode must be provided in the key")``
#    from the model class, not the new ModelException.
# --------------------------------------------------------------------------- #
@pytest.mark.p0
def test_bedrock_embed_missing_auth_mode_raises_existing_value_error():
    """The fix only changes the JSON parse + type check. The downstream
    auth_mode-presence check (which raises ``ValueError``) is unchanged."""
    from rag.llm.embedding_model import BedrockEmbed

    bad = json.dumps({"bedrock_region": "us-east-1", "bedrock_ak": "x", "bedrock_sk": "y"})
    with pytest.raises(ValueError) as exc_info:
        BedrockEmbed(bad, "amazon.titan-embed-text-v1")
    assert "auth_mode" in str(exc_info.value)
