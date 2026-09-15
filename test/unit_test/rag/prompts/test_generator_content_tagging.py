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
import logging
from types import SimpleNamespace

import pytest

from common.constants import TAG_FLD
from rag.prompts.generator import content_tagging


def _chat_model(reply):
    async def async_chat(_system, _history, _gen_conf=None, **_kwargs):
        return reply

    return SimpleNamespace(llm_name="test-llm", max_length=8192, async_chat=async_chat)


_EXAMPLES = [{"content": "An example chunk", TAG_FLD: {"example": 1}}]


class TestContentTagging:
    @pytest.mark.p2
    @pytest.mark.asyncio
    async def test_json_object_reply_is_parsed(self):
        res = await content_tagging(_chat_model('{"alpha": 2, "beta": 1}'), "chunk", ["alpha", "beta"], _EXAMPLES)

        assert res == {"alpha": 2, "beta": 1}

    @pytest.mark.p2
    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        "reply",
        [
            "I could not identify any tags for this content.",
            "",
            '["alpha", "beta"]',
        ],
        ids=["prose", "empty", "json_array"],
    )
    async def test_reply_without_a_json_object_yields_no_tags(self, reply):
        """A reply holding no JSON object parses to "" and a bare array of strings to a
        list. Both used to reach .items() and abort the whole tagging batch through
        asyncio.gather(return_exceptions=False)."""
        res = await content_tagging(_chat_model(reply), "chunk", ["alpha", "beta"], _EXAMPLES)

        assert res == {}

    @pytest.mark.p2
    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        "reply",
        ['[{"alpha": 2, "beta": 1}]', '{"alpha": 2}\n{"beta": 1}'],
        ids=["array_wrapped", "consecutive_objects"],
    )
    async def test_objects_inside_a_list_still_yield_their_tags(self, reply):
        """json_repair returns a list for both shapes, and the tags are still in it."""
        res = await content_tagging(_chat_model(reply), "chunk", ["alpha", "beta"], _EXAMPLES)

        assert res == {"alpha": 2, "beta": 1}

    @pytest.mark.p2
    @pytest.mark.asyncio
    async def test_a_list_holding_no_object_is_reported_not_silently_empty(self, caplog):
        """An empty result from a malformed reply must stay distinguishable from real
        empty tags, so the list branch warns when it merged nothing."""
        with caplog.at_level(logging.WARNING):
            res = await content_tagging(_chat_model("[1, 2, 3]"), "chunk", ["alpha", "beta"], _EXAMPLES)

        assert res == {}
        assert "holding no object" in caplog.text

    @pytest.mark.p2
    @pytest.mark.asyncio
    async def test_the_warning_does_not_carry_the_model_reply(self, caplog):
        """The reply can echo the chunk back, so the log records shape, not content."""
        secret = "PATIENT NAME REDACTED EXAMPLE STRING"
        with caplog.at_level(logging.WARNING):
            await content_tagging(_chat_model(secret), "chunk", ["alpha", "beta"], _EXAMPLES)

        assert secret not in caplog.text
        assert "content_tagging" in caplog.text

    @pytest.mark.p2
    @pytest.mark.asyncio
    async def test_a_real_parse_failure_still_propagates(self):
        """Guards against "fix" it by wrapping the whole parse in except Exception.

        json_repair.loads does raise, as ValueError, once nesting passes the parser's
        recursion limit. That is a genuine failure and must not be swallowed as no tags.
        """
        with pytest.raises(ValueError):
            await content_tagging(_chat_model("[" * 400), "chunk", ["alpha", "beta"], _EXAMPLES)
