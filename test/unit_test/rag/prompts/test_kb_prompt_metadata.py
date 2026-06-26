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
import pytest

from rag.prompts.generator import kb_prompt


@pytest.mark.p1
class TestKbPromptDocumentMetadata:
    """Regression tests for kb_prompt's handling of `document_metadata` on chunks."""

    @pytest.mark.p1
    def test_null_document_metadata_does_not_crash(self):
        """A chunk with `document_metadata: None` must not raise AttributeError.

        Regression for issue #14651: chunks retrieved from the index can carry
        an explicit null metadata field, which made `dict.get(..., {})` return
        `None` and crash citation generation with
        `AttributeError: 'NoneType' object has no attribute 'items'`.
        """
        kbinfos = {
            "chunks": [
                {
                    "id": "chunk-1",
                    "content_with_weight": "hello world",
                    "docnm_kwd": "doc.pdf",
                    "document_metadata": None,
                }
            ]
        }

        rendered = kb_prompt(kbinfos, max_tokens=10000)

        assert len(rendered) == 1
        assert "hello world" in rendered[0]
        assert "doc.pdf" in rendered[0]

    @pytest.mark.p1
    def test_missing_document_metadata_key(self):
        """A chunk with no `document_metadata` key at all should also work."""
        kbinfos = {
            "chunks": [
                {
                    "id": "chunk-1",
                    "content_with_weight": "hello world",
                    "docnm_kwd": "doc.pdf",
                }
            ]
        }

        rendered = kb_prompt(kbinfos, max_tokens=10000)

        assert len(rendered) == 1
        assert "hello world" in rendered[0]

    @pytest.mark.p1
    def test_populated_document_metadata_renders_fields(self):
        """When metadata is a dict, its key/value pairs must be rendered."""
        kbinfos = {
            "chunks": [
                {
                    "id": "chunk-1",
                    "content_with_weight": "hello world",
                    "docnm_kwd": "doc.pdf",
                    "document_metadata": {"author": "alice", "year": "2026"},
                }
            ]
        }

        rendered = kb_prompt(kbinfos, max_tokens=10000)

        assert len(rendered) == 1
        assert "author: alice" in rendered[0]
        assert "year: 2026" in rendered[0]


@pytest.mark.p1
class TestKbPromptTokenBudget:
    """Regression tests for kb_prompt token-budget trimming."""

    @pytest.mark.p1
    def test_excludes_chunk_that_exceeds_token_budget(self):
        """The chunk that pushes usage over the budget must not be rendered."""
        kbinfos = {
            "chunks": [
                {
                    "id": "chunk-1",
                    "content_with_weight": "fits " * 20,
                    "docnm_kwd": "doc1.pdf",
                },
                {
                    "id": "chunk-2",
                    "content_with_weight": "overflow " * 200,
                    "docnm_kwd": "doc2.pdf",
                },
                {
                    "id": "chunk-3",
                    "content_with_weight": "never " * 200,
                    "docnm_kwd": "doc3.pdf",
                },
            ]
        }

        rendered = kb_prompt(kbinfos, max_tokens=50)

        assert len(rendered) == 1
        assert "doc1.pdf" in rendered[0]
        assert "overflow" not in rendered[0]
        assert "never" not in rendered[0]
