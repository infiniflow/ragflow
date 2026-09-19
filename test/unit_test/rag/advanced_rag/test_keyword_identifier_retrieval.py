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

import sys
import types

import pytest

from rag.advanced_rag.harness.keywords import extract_weighted_keywords
from rag.advanced_rag.harness.tools.text_processing import _narrow_or_keep

pytestmark = pytest.mark.p1


class _LLM:
    max_length = 4096
    response = '{"entity": [], "aliases": ["LRP-1206"], "fact_type": ["limitation"], "qualifiers": []}'

    async def async_chat(self, *_args, **_kwargs):
        return self.response


@pytest.mark.asyncio
async def test_exact_identifier_is_preserved_when_keyword_model_misses_it(monkeypatch):
    generator = types.ModuleType("rag.prompts.generator")
    generator.form_message = lambda system, user: [
        {"role": "system", "content": system},
        {"role": "user", "content": user},
    ]
    generator.message_fit_in = lambda messages, _max_length: (None, messages)
    monkeypatch.setitem(sys.modules, "rag.prompts", types.ModuleType("rag.prompts"))
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", generator)
    question = "Tell me about the limitation LRP-1025."

    query, keywords = await extract_weighted_keywords(_LLM(), question)

    assert query.count("LRP-1025") == 3
    assert "LRP-1025" in keywords
    chunks = [
        {"content": "The limitation LRP-1025 applies to this device."},
        {"content": "The device has a documented maintenance schedule."},
    ]
    kept = _narrow_or_keep(chunks, keywords, "test")
    assert kept == [chunks[0]]


@pytest.mark.asyncio
async def test_extraction_failure_keeps_the_raw_question_fallback(monkeypatch):
    generator = types.ModuleType("rag.prompts.generator")
    generator.form_message = lambda system, user: [
        {"role": "system", "content": system},
        {"role": "user", "content": user},
    ]
    generator.message_fit_in = lambda messages, _max_length: (None, messages)
    monkeypatch.setitem(sys.modules, "rag.prompts", types.ModuleType("rag.prompts"))
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", generator)

    class _BrokenLLM(_LLM):
        async def async_chat(self, *_args, **_kwargs):
            raise RuntimeError("model unavailable")

    question = "Tell me about LRP-1025."
    query, keywords = await extract_weighted_keywords(_BrokenLLM(), question)

    assert (query, keywords) == (question, question)


def test_literal_identifier_scan_excludes_urls_and_dates():
    from rag.advanced_rag.harness.keywords import _literal_identifiers

    assert _literal_identifiers("LRP-1025 and A1") == ["LRP-1025", "A1"]
    assert _literal_identifiers("see https://example.com/a1 on 2025-07-01") == []
    assert _literal_identifiers("release v2.1, not A1.2") == ["v2.1"]


@pytest.mark.asyncio
async def test_malformed_keyword_json_keeps_the_raw_question_fallback(monkeypatch):
    generator = types.ModuleType("rag.prompts.generator")
    generator.form_message = lambda system, user: [
        {"role": "system", "content": system},
        {"role": "user", "content": user},
    ]
    generator.message_fit_in = lambda messages, _max_length: (None, messages)
    monkeypatch.setitem(sys.modules, "rag.prompts", types.ModuleType("rag.prompts"))
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", generator)

    class _MalformedLLM(_LLM):
        response = "not JSON"

    question = "Tell me about LRP-1025."
    assert await extract_weighted_keywords(_MalformedLLM(), question) == (question, question)


def test_repeated_literal_identifiers_are_deduplicated():
    from rag.advanced_rag.harness.keywords import _literal_identifiers

    assert _literal_identifiers("LRP-1025 and lrp-1025 and LRP-1025") == ["LRP-1025"]


@pytest.mark.asyncio
async def test_multiple_identifiers_are_kept_ahead_of_optional_terms(monkeypatch):
    generator = types.ModuleType("rag.prompts.generator")
    generator.form_message = lambda system, user: [
        {"role": "system", "content": system},
        {"role": "user", "content": user},
    ]
    generator.message_fit_in = lambda messages, _max_length: (None, messages)
    monkeypatch.setitem(sys.modules, "rag.prompts", types.ModuleType("rag.prompts"))
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", generator)

    model = _LLM()
    model.response = '{"entity": [], "aliases": ["' + ("alias " * 100) + '"], "fact_type": [], "qualifiers": []}'
    query, keywords = await extract_weighted_keywords(model, "Compare LRP-1025 with A1.")

    assert len(query) == 400
    assert len(keywords) == 400
    assert all(identifier in query and identifier in keywords for identifier in ("LRP-1025", "A1"))


@pytest.mark.asyncio
async def test_literal_identifier_is_kept_before_the_keyword_length_cap(monkeypatch):
    generator = types.ModuleType("rag.prompts.generator")
    generator.form_message = lambda system, user: [
        {"role": "system", "content": system},
        {"role": "user", "content": user},
    ]
    generator.message_fit_in = lambda messages, _max_length: (None, messages)
    monkeypatch.setitem(sys.modules, "rag.prompts", types.ModuleType("rag.prompts"))
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", generator)

    model = _LLM()
    model.response = '{"entity": [], "aliases": ["' + ("alias " * 100) + '"], "fact_type": [], "qualifiers": []}'
    query, keywords = await extract_weighted_keywords(model, "Explain LRP-1025.")

    assert len(query) <= 400
    assert len(keywords) <= 400
    assert "LRP-1025" in query
    assert "LRP-1025" in keywords
