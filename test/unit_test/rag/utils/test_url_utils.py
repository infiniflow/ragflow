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

"""
Unit tests for rag/utils/url_utils.py module.
"""

import pytest
from rag.utils.url_utils import ensure_v1


class TestEnsureV1:
    def test_empty_url_returned_unchanged(self):
        assert ensure_v1("") == ""

    def test_bare_host_gets_v1_appended(self):
        assert ensure_v1("https://api.example.com") == "https://api.example.com/v1"

    def test_already_versioned_unchanged(self):
        assert ensure_v1("https://api.example.com/v1") == "https://api.example.com/v1"

    def test_versioned_with_trailing_path_unchanged(self):
        assert ensure_v1("https://api.example.com/v2/chat") == "https://api.example.com/v2/chat"

    def test_versioned_with_leading_path_unchanged(self):
        assert ensure_v1("https://api.example.com/api/v3") == "https://api.example.com/api/v3"

    @pytest.mark.parametrize(
        "url",
        [
            "https://generativelanguage.googleapis.com/v1beta/openai/",
            "https://generativelanguage.googleapis.com/v1beta/openai",
            "https://example.com/v1alpha1",
        ],
    )
    def test_version_suffix_with_letters_is_recognized(self, url):
        """A path segment like v1beta/v1alpha1 is already versioned - real
        OpenAI-compatible endpoints (e.g. Google's Gemini API) use this
        pattern, and ensure_v1 must not append a second /v1 onto it."""
        assert ensure_v1(url) == url

    def test_non_version_segment_starting_with_v_still_gets_v1(self):
        """A segment like 'vendors' merely starting with the letter v (with
        no digit right after) is not a version segment - /v1 must still be
        appended."""
        assert ensure_v1("https://api.example.com/vendors/list") == "https://api.example.com/vendors/list/v1"

    @pytest.mark.parametrize(
        "url,expected",
        [
            # Issue #18965: a user pasting a full OpenAI-compatible endpoint
            # URL must not have the path doubled by the OpenAI client.
            ("https://api.pzero.studio/v1/chat/completions", "https://api.pzero.studio/v1"),
            ("https://api.openai.com/v1/embeddings", "https://api.openai.com/v1"),
            ("https://api.openai.com/v1/completions", "https://api.openai.com/v1"),
            # No version segment + endpoint: strip the endpoint, then add /v1.
            ("https://api.pzero.studio/chat/completions", "https://api.pzero.studio/v1"),
            # Trailing slash on the input is preserved when no endpoint is
            # stripped.
            ("https://generativelanguage.googleapis.com/v1beta/openai/", "https://generativelanguage.googleapis.com/v1beta/openai/"),
        ],
    )
    def test_trailing_openai_endpoint_is_stripped(self, url, expected):
        """A user pasting ``<base>/v1/chat/completions`` (or ``/embeddings`` /
        ``/completions``) gets the endpoint stripped so the OpenAI client
        can re-append it on the request without doubling the path. See
        issue #18965."""
        assert ensure_v1(url) == expected
