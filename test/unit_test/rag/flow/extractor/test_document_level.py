#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
"""
Extractor.document_level covers a real bug observed on a live deployment:
with the default per-chunk mode, a document-level metadata field (e.g.
document_description) ends up holding whatever the *last* processed chunk's
LLM call produced, because DataflowService._process_chunks() -> the doc
metadata merge overwrites non-list values rather than combining them. These
tests pin the fix (concatenate all chunks, ask once) and the defensive JSON
handling that goes with it: the model backing this component often ignores
"reply with only this JSON object" on a long whole-document prompt, so the
retry-until-parsable-JSON loop (mirroring LLM._invoke_async's own
"structured" output handling in agent/component/llm.py) must never let raw,
unparsable prose end up stored as metadata.
"""

import json

import pytest

from rag.flow.extractor.extractor import Extractor, ExtractorParam, _strip_markdown


class _FakeChatModel:
    """Stand-in for LLMBundle: records calls and returns queued replies."""

    def __init__(self, replies, max_length=8192):
        self._replies = list(replies)
        self.max_length = max_length
        self.calls = []

    async def async_chat(self, system, history, gen_conf):
        self.calls.append({"system": system, "history": history, "gen_conf": gen_conf})
        return self._replies.pop(0)


def _make_extractor(field_name="document_description", document_level=False, max_retries=0, chat_mdl=None):
    cpn = Extractor.__new__(Extractor)
    cpn._param = ExtractorParam()
    cpn._param.field_name = field_name
    cpn._param.document_level = document_level
    cpn._param.max_retries = max_retries
    cpn._param.sys_prompt = 'Reply with only {"document_description": "..."}.'
    cpn._param.prompts = [{"role": "user", "content": "{document}"}]
    cpn.chat_mdl = chat_mdl
    cpn.imgs = []
    cpn.callback = lambda *args, **kwargs: None
    return cpn


def _chunks(*texts):
    return [{"text": t} for t in texts]


@pytest.mark.p1
class TestStripMarkdown:
    def test_removes_headings_bold_and_bullets(self):
        raw = "# Heading\n**bold** and *italic*\n- item one\n- item two\n1. numbered"
        cleaned = _strip_markdown(raw)
        assert "#" not in cleaned
        assert "**" not in cleaned
        assert cleaned.count("*") == 0
        assert "- " not in cleaned
        assert "1. " not in cleaned
        assert "bold" in cleaned and "italic" in cleaned and "item one" in cleaned

    def test_removes_wrapping_code_fence(self):
        raw = '```json\n{"a": "b"}\n```'
        cleaned = _strip_markdown(raw)
        assert "```" not in cleaned

    def test_removes_table_syntax_without_losing_cell_content(self):
        raw = "summary text\n| a | b |\n|---|---|\n| 1 | 2 |"
        cleaned = _strip_markdown(raw)
        assert "|" not in cleaned
        assert "summary text" in cleaned
        # The separator row ("|---|---|") is pure formatting and should
        # disappear, but data-row cell content ("a", "b", "1", "2") must
        # survive -- an earlier version of this regex deleted whole table
        # rows (content included), not just the "|" syntax.
        assert "a" in cleaned and "b" in cleaned
        assert "1" in cleaned and "2" in cleaned
        assert "---" not in cleaned

    def test_leaves_plain_text_untouched_besides_whitespace(self):
        assert _strip_markdown("Just a plain sentence.") == "Just a plain sentence."


@pytest.mark.p1
@pytest.mark.asyncio
class TestDocumentLevelDisabled:
    async def test_per_chunk_path_is_unaffected(self):
        """Regression guard: document_level=False must still call the model
        once per chunk and let each chunk keep its own result.

        Drives this through Extractor._invoke() itself (not a hand-rolled
        re-implementation of its loop) so a future change to _invoke()'s
        per-chunk branch would actually be caught here.
        """
        chat_mdl = _FakeChatModel(["desc for chunk 1", "desc for chunk 2"])
        cpn = _make_extractor(document_level=False, chat_mdl=chat_mdl)
        input_chunks = _chunks("chunk one text", "chunk two text")
        cpn.get_input_elements = lambda: {"document": {"value": input_chunks, "name": "document"}}

        await cpn._invoke()

        result_chunks = cpn.output("chunks")
        assert len(chat_mdl.calls) == 2
        assert result_chunks[0]["document_description"] == "desc for chunk 1"
        assert result_chunks[1]["document_description"] == "desc for chunk 2"


@pytest.mark.p1
@pytest.mark.asyncio
class TestDocumentLevelExtract:
    async def test_concatenates_chunks_and_shares_result_across_all(self):
        chat_mdl = _FakeChatModel(['{"document_description": "Whole-document summary."}'])
        cpn = _make_extractor(document_level=True, chat_mdl=chat_mdl)
        chunks = _chunks("page one text", "page two text", "page three text")

        await cpn._document_level_extract(chunks, "document", {})

        assert len(chat_mdl.calls) == 1
        sent_history = chat_mdl.calls[0]["history"]
        sent_text = " ".join(m["content"] for m in sent_history)
        assert "page one text" in sent_text
        assert "page two text" in sent_text
        assert "page three text" in sent_text

        for ck in chunks:
            assert ck["document_description"] == '{"document_description": "Whole-document summary."}'

    async def test_retries_until_a_parsable_json_object_is_returned(self):
        chat_mdl = _FakeChatModel(
            [
                "Here is a rewritten version of the document with no JSON at all.",
                '{"document_description": "Second attempt succeeded."}',
            ]
        )
        cpn = _make_extractor(document_level=True, max_retries=1, chat_mdl=chat_mdl)
        chunks = _chunks("some document text")

        await cpn._document_level_extract(chunks, "document", {})

        assert len(chat_mdl.calls) == 2
        assert chunks[0]["document_description"] == '{"document_description": "Second attempt succeeded."}'

    async def test_gives_up_quietly_when_retries_exhausted(self):
        chat_mdl = _FakeChatModel(
            [
                "Free-form prose, attempt one.",
                "Free-form prose, attempt two.",
            ]
        )
        cpn = _make_extractor(document_level=True, max_retries=1, chat_mdl=chat_mdl)
        chunks = _chunks("some document text")

        await cpn._document_level_extract(chunks, "document", {})

        assert len(chat_mdl.calls) == 2
        assert "document_description" not in chunks[0]

    async def test_empty_json_object_does_not_count_as_success(self):
        """A syntactically valid but empty {} is not usable output -- it
        must be treated the same as an unparsable reply (retry, then give
        up), not accepted as "the model successfully returned JSON"."""
        chat_mdl = _FakeChatModel(["{}", '{"document_description": "Real content."}'])
        cpn = _make_extractor(document_level=True, max_retries=1, chat_mdl=chat_mdl)
        chunks = _chunks("some document text")

        await cpn._document_level_extract(chunks, "document", {})

        assert len(chat_mdl.calls) == 2
        assert chunks[0]["document_description"] == '{"document_description": "Real content."}'

    async def test_non_string_values_are_filtered_and_retried(self):
        """update_metadata_to() (common/metadata_utils.py) only accepts str
        or list-of-str values and silently drops anything else per-key.
        A reply that's valid JSON but carries an unusable value type (e.g. a
        nested object) must not be "accepted" here only to be dropped later
        -- filter it out and, if nothing usable is left, retry."""
        chat_mdl = _FakeChatModel(
            [
                '{"document_description": {"nested": "not a string"}}',
                '{"document_description": "Usable string.", "tags": ["a", "b"]}',
            ]
        )
        cpn = _make_extractor(document_level=True, max_retries=1, chat_mdl=chat_mdl)
        chunks = _chunks("some document text")

        await cpn._document_level_extract(chunks, "document", {})

        assert len(chat_mdl.calls) == 2
        stored = json.loads(chunks[0]["document_description"])
        assert stored == {"document_description": "Usable string.", "tags": ["a", "b"]}

    async def test_strips_markdown_from_stored_string_values(self):
        chat_mdl = _FakeChatModel(['{"document_description": "# Heading\\n**bold** summary text."}'])
        cpn = _make_extractor(document_level=True, chat_mdl=chat_mdl)
        chunks = _chunks("some document text")

        await cpn._document_level_extract(chunks, "document", {})

        stored = chunks[0]["document_description"]
        assert "#" not in stored
        assert "**" not in stored
        assert "bold summary text." in stored

    async def test_skips_without_calling_model_when_prompt_cannot_fit(self):
        # A tiny max_length forces message_fit_in() to trim the (huge)
        # concatenated document text down to an empty user turn -- the same
        # degenerate case LLM.validate_fitted_messages() treats as an error
        # everywhere else in agent/component/llm.py.
        chat_mdl = _FakeChatModel([], max_length=5)
        cpn = _make_extractor(document_level=True, chat_mdl=chat_mdl)
        chunks = _chunks("x" * 5000)

        await cpn._document_level_extract(chunks, "document", {})

        assert len(chat_mdl.calls) == 0
        assert "document_description" not in chunks[0]
