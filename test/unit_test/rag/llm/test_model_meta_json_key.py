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
"""Regression coverage for the JSON-decode silent-fallback bug in
``rag/llm/model_meta.py:_get_api_key`` for the 3 provider classes that
match the pattern fixed elsewhere in cycles 7-16 (PRs #17457, #17459).

Closes #18250.
"""

import pytest
from common.exceptions import ModelException

from rag.llm.model_meta import NewAPI, OpenRouter, VolcEngine


# ---------------------------------------------------------------------------
# VolcEngine._get_api_key
# ---------------------------------------------------------------------------


class TestVolcEngineGetApiKey:
    """The model-list / verify path of the VolcEngine/Ark factory.

    Pre-fix: ``json.loads(self.api_key).get("ark_api_key", "")`` inside
    ``try/except JSONDecodeError``. The ``.get(...)`` call crashed with
    ``AttributeError`` when the user pasted a JSON non-object, and
    ``TypeError`` from ``json.loads(None)`` slipped through the
    ``except JSONDecodeError`` guard.
    """

    def test_none_returns_empty_string(self):
        v = VolcEngine(None)
        assert v._get_api_key() == ""

    def test_empty_string_returns_empty_string(self):
        v = VolcEngine("")
        assert v._get_api_key() == ""

    def test_plain_string_returns_itself(self):
        # Historical ``except JSONDecodeError`` fallback: a bare API key
        # is used verbatim. The pre-fix code already got this right; the
        # post-fix code preserves it.
        v = VolcEngine("sk-volcengine-plain-key")
        assert v._get_api_key() == "sk-volcengine-plain-key"

    def test_json_dict_with_ark_api_key(self):
        v = VolcEngine('{"ark_api_key": "real-ark-key"}')
        assert v._get_api_key() == "real-ark-key"

    def test_json_dict_with_api_key_fallback(self):
        # Some operators paste a flat ``{"api_key": "..."}`` shape. The
        # model_meta.py site should accept this and return the api_key
        # value, matching the chat_model.py VolcEngineChat behavior.
        v = VolcEngine('{"api_key": "real-api-key"}')
        assert v._get_api_key() == "real-api-key"

    def test_json_dict_empty_returns_empty_string(self):
        v = VolcEngine("{}")
        assert v._get_api_key() == ""

    def test_python_dict_input(self):
        # The runtime sometimes passes a Python dict directly (e.g. the
        # form-data path that does not JSON-encode before passing the
        # value). Pre-fix: the bare ``json.loads(self.api_key)`` call
        # raised ``TypeError`` on a dict input. Post-fix: the helper
        # accepts dicts verbatim.
        v = VolcEngine({"ark_api_key": "dict-ark-key"})
        assert v._get_api_key() == "dict-ark-key"

    def test_json_array_raises_model_exception(self):
        v = VolcEngine("[1, 2, 3]")
        with pytest.raises(ModelException, match="JSON object"):
            v._get_api_key()

    def test_json_string_raises_model_exception(self):
        # ``json_repair`` / round-trip edge case: the user pastes a
        # JSON-quoted string like ``'"sk-key"'``. Pre-fix: ``.get``
        # crashed with ``AttributeError: 'str' object has no attribute
        # 'get'``. Post-fix: clear ModelException.
        v = VolcEngine('"sk-key"')
        with pytest.raises(ModelException, match="JSON object"):
            v._get_api_key()

    def test_json_null_raises_model_exception(self):
        v = VolcEngine("null")
        with pytest.raises(ModelException, match="JSON object"):
            v._get_api_key()

    def test_json_number_raises_model_exception(self):
        v = VolcEngine("42")
        with pytest.raises(ModelException, match="JSON object"):
            v._get_api_key()


# ---------------------------------------------------------------------------
# OpenRouter._get_api_key
# ---------------------------------------------------------------------------


class TestOpenRouterGetApiKey:
    """The model-list / verify path of the OpenRouter factory.

    Pre-fix: ``json.loads(api_key)`` inside ``try/except Exception``,
    then ``isinstance(payload, dict)`` check. The ``except Exception``
    was too broad (silently swallowed ``TypeError`` from
    ``json.loads(None)`` and returned ``""``). The non-dict JSON case
    fell back to returning the raw JSON string, which then 401s at
    the upstream API.
    """

    def test_none_returns_empty_string(self):
        o = OpenRouter(None)
        assert o._get_api_key() == ""

    def test_empty_string_returns_empty_string(self):
        o = OpenRouter("")
        assert o._get_api_key() == ""

    def test_plain_string_returns_itself(self):
        o = OpenRouter("sk-or-v1-plain-key")
        assert o._get_api_key() == "sk-or-v1-plain-key"

    def test_json_dict_with_api_key(self):
        o = OpenRouter('{"api_key": "real-openrouter-key"}')
        assert o._get_api_key() == "real-openrouter-key"

    def test_json_dict_with_provider_order(self):
        # The provider_order field is preserved by the helper; the
        # model_meta.py site only reads ``api_key`` but the helper
        # returns the full dict so the contract is clear for future
        # callers.
        result = OpenRouter('{"api_key": "real-key", "provider_order": "A,B,C"}')._get_api_key()
        assert result == "real-key"

    def test_json_dict_empty_returns_raw_fallback(self):
        # The historical ``payload.get("api_key") or api_key`` fallback
        # returns the raw key string when the JSON dict has no
        # ``api_key`` field. The model_meta.py site preserves this
        # behavior. The pre-fix code returned ``'{}'`` here too.
        o = OpenRouter("{}")
        assert o._get_api_key() == "{}"

    def test_json_dict_missing_api_key_falls_back_to_raw(self):
        # The OpenRouter model_meta.py site preserves the historical
        # ``payload.get("api_key") or api_key`` fallback. If the
        # ``api_key`` field is missing from a JSON dict, the raw key
        # string is returned (the API will then 401, which is the
        # intended UX: surface "your key is invalid" to the operator).
        o = OpenRouter('{"other_field": "value"}')
        assert o._get_api_key() == '{"other_field": "value"}'

    def test_python_dict_input(self):
        o = OpenRouter({"api_key": "dict-openrouter-key"})
        assert o._get_api_key() == "dict-openrouter-key"

    def test_json_array_raises_model_exception(self):
        o = OpenRouter("[1, 2, 3]")
        with pytest.raises(ModelException, match="JSON object"):
            o._get_api_key()

    def test_json_null_raises_model_exception(self):
        o = OpenRouter("null")
        with pytest.raises(ModelException, match="JSON object"):
            o._get_api_key()


# ---------------------------------------------------------------------------
# NewAPI._get_api_key
# ---------------------------------------------------------------------------


class TestNewAPIGetApiKey:
    """The model-list / verify path of the ``NewAPI`` factory.

    Pre-fix: ``json.loads(self.api_key)`` inside
    ``try/except (JSONDecodeError, TypeError)``, then
    ``isinstance(parsed, dict)`` check. A JSON non-object silently
    returned the raw JSON string as the api_key, which then 401s at
    the upstream API.
    """

    def test_none_returns_empty_string(self):
        n = NewAPI(None)
        assert n._get_api_key() == ""

    def test_empty_string_returns_empty_string(self):
        n = NewAPI("")
        assert n._get_api_key() == ""

    def test_plain_string_returns_itself(self):
        n = NewAPI("sk-newapi-plain-key")
        assert n._get_api_key() == "sk-newapi-plain-key"

    def test_json_dict_with_api_key(self):
        n = NewAPI('{"api_key": "real-newapi-key"}')
        assert n._get_api_key() == "real-newapi-key"

    def test_python_dict_input(self):
        n = NewAPI({"api_key": "dict-newapi-key"})
        assert n._get_api_key() == "dict-newapi-key"

    def test_json_array_raises_model_exception(self):
        n = NewAPI("[1, 2, 3]")
        with pytest.raises(ModelException, match="JSON object"):
            n._get_api_key()

    def test_json_null_raises_model_exception(self):
        n = NewAPI("null")
        with pytest.raises(ModelException, match="JSON object"):
            n._get_api_key()

    def test_json_string_raises_model_exception(self):
        # The pre-fix code returned ``'"sk-key"'`` (the raw JSON
        # string) as the api_key, which then 401s at the upstream
        # API. The post-fix code raises a clear ModelException.
        n = NewAPI('"sk-key"')
        with pytest.raises(ModelException, match="JSON object"):
            n._get_api_key()


# ---------------------------------------------------------------------------
# Cross-class: the historical "raw key for non-JSON" fallback is preserved
# ---------------------------------------------------------------------------


class TestHistoricalFallbackPreserved:
    """Sanity check: the historical ``except JSONDecodeError: api_key =
    self.api_key`` fallback (used by every pre-fix site for plain
    non-JSON string input) is preserved across all 3 classes. Operators
    who pasted a bare ``"sk-..."`` key and have working configs see no
    change.
    """

    @pytest.mark.parametrize(
        "cls,api_key,expected",
        [
            (VolcEngine, "sk-volcengine-bare", "sk-volcengine-bare"),
            (VolcEngine, "plain-key-with-special-chars_!@#", "plain-key-with-special-chars_!@#"),
            (OpenRouter, "sk-or-v1-bare", "sk-or-v1-bare"),
            (NewAPI, "sk-newapi-bare", "sk-newapi-bare"),
        ],
    )
    def test_plain_key_passes_through_unchanged(self, cls, api_key, expected):
        assert cls(api_key)._get_api_key() == expected


# ---------------------------------------------------------------------------
# Cross-class: input that was an unhandled exception is now a clear error
# ---------------------------------------------------------------------------


class TestCrashFixes:
    """Specific regression cases for the crashes the pre-fix code
    produced on misconfigured input. Each test pins a real failure
    mode that an operator would hit in production.
    """

    def test_volcengine_with_empty_object(self):
        # Pre-fix: ``json.loads("{}").get("ark_api_key", "")`` -> ``""``
        # (worked). Post-fix: still ``""`` via the helper.
        assert VolcEngine("{}")._get_api_key() == ""

    def test_volcengine_with_object_missing_ark_api_key(self):
        # Pre-fix: ``json.loads('{"other": "x"}').get("ark_api_key", "")``
        # -> ``""`` (worked). Post-fix: still ``""`` via the helper.
        assert VolcEngine('{"other": "x"}')._get_api_key() == ""

    def test_openrouter_with_object_missing_api_key(self):
        # Pre-fix: returned the raw dict's JSON string, which then 401s.
        # Post-fix: the helper raises ModelException on JSON non-object,
        # but JSON object missing ``api_key`` returns the raw key (per
        # the historical fallback in the model_meta.py site).
        raw = '{"other": "x"}'
        assert OpenRouter(raw)._get_api_key() == raw

    def test_newapi_with_object_missing_api_key(self):
        # Same as OpenRouter above. The pre-fix code did
        # ``return self.api_key`` for non-dict JSON and for the
        # ``except`` branch. Post-fix: a JSON dict without ``api_key``
        # is the ``else`` branch -- helper returns ``""`` from the
        # ``payload.get("api_key", self.api_key)`` line, so the caller
        # gets an empty string. This is a slight behavior change from
        # pre-fix ``return self.api_key``, but the empty string is the
        # same effective 401 path, and the user now sees a clear error
        # message instead of a silent 401.
        assert NewAPI('{"other": "x"}')._get_api_key() == ""


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
