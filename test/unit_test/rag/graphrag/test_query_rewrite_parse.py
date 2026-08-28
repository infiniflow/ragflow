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
"""Parsing of the query-rewrite reply in rag/graphrag/search.py."""

import pytest

from rag.graphrag.search import keywords_from_query_rewrite

_OBJECT = '{"answer_type_keywords": ["PERSON"], "entities_from_query": ["ada"]}'
_PARSED = {"answer_type_keywords": ["PERSON"], "entities_from_query": ["ada"]}


class TestKeywordsFromQueryRewrite:
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "reply",
        [_OBJECT, "Output:\n" + _OBJECT, "```json\n" + _OBJECT + "\n```"],
        ids=["bare", "preamble", "fenced"],
    )
    def test_a_reply_carrying_the_object_is_parsed(self, reply):
        assert keywords_from_query_rewrite(reply) == _PARSED

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "reply",
        ["[" + _OBJECT + "]", '{"answer_type_keywords": ["PERSON"]}\n{"entities_from_query": ["ada"]}'],
        ids=["array_wrapped", "consecutive_objects"],
    )
    def test_objects_inside_a_list_are_merged(self, reply):
        """json_repair returns a list for both shapes and the keywords are still in it."""
        assert keywords_from_query_rewrite(reply) == _PARSED

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "reply",
        ["I cannot answer that.", "", "[1, 2, 3]", '"PERSON"'],
        ids=["prose", "empty", "list_of_scalars", "bare_string"],
    )
    def test_an_unusable_reply_raises_value_error(self, reply):
        """Raising is the useful outcome here, not a shortcoming.

        KGSearch.retrieval catches this and falls back to searching on the question, which
        keeps entity, n-hop and community retrieval running. Returning empty keywords would
        skip that fallback, so a fix that swallows the failure is worse than the bug.

        The type matters as much as the raise. Before this change these replies produced an
        AttributeError blaming json_repair, and a parse that strips to the first brace pair
        produces IndexError. Neither tells an operator what the model sent.
        """
        with pytest.raises(ValueError) as excinfo:
            keywords_from_query_rewrite(reply)

        assert "expected a JSON object of keywords" in str(excinfo.value)

    @pytest.mark.p2
    def test_the_failure_does_not_carry_the_model_reply(self):
        """The reply can echo the question and the prompt's entity samples."""
        secret = "PATIENT NAME REDACTED EXAMPLE STRING"

        with pytest.raises(ValueError) as excinfo:
            keywords_from_query_rewrite(secret)

        assert secret not in str(excinfo.value)

    @pytest.mark.p2
    def test_a_reply_nested_past_the_parser_limit_propagates(self):
        """json_repair.loads does raise, as ValueError, once nesting passes its recursion
        limit. Documented here so nobody re-adds a handler claiming it never raises."""
        with pytest.raises(ValueError):
            keywords_from_query_rewrite("[" * 400)
