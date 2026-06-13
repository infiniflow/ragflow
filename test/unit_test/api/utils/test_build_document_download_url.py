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

"""Unit tests for build_document_download_url helper."""

import pytest

from api.utils.api_utils import build_document_download_url

pytestmark = pytest.mark.p2


class TestBuildDocumentDownloadUrl:
    """Behavioral tests for the chunk-response download URL helper (issue #14771)."""

    def test_returns_canonical_path(self):
        """Happy path: both ids present yields the SDK file-stream route path."""
        assert build_document_download_url("ds_abc", "doc_123") == "/api/v1/datasets/ds_abc/documents/doc_123"

    @pytest.mark.parametrize(
        "dataset_id,document_id",
        [
            (None, "doc_123"),
            ("ds_abc", None),
            (None, None),
            ("", "doc_123"),
            ("ds_abc", ""),
            ("", ""),
        ],
    )
    def test_returns_none_when_either_id_missing(self, dataset_id, document_id):
        """Missing/empty ids return None so callers don't advertise an unusable URL."""
        assert build_document_download_url(dataset_id, document_id) is None

    def test_does_not_url_encode_caller_inputs(self):
        """Inputs are pasted verbatim, matching how Flask/Quart route patterns work."""
        assert build_document_download_url("ds/with/slash", "doc?bad") == "/api/v1/datasets/ds/with/slash/documents/doc?bad"
