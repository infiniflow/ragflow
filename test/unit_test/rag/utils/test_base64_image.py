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
#

"""Unit tests for composite storage image ID parsing."""

from functools import partial

import pytest
from PIL import Image

from rag.utils import base64_image


@pytest.mark.p2
class TestParseStorageCompositeId:
    """Tests for parse_storage_composite_id."""

    def test_hyphenated_object_key(self):
        """Object keys with hyphens split only on the first separator."""
        result = base64_image.parse_storage_composite_id(
            "kb12345678901234567890123456789012-page-1.png"
        )
        assert result == ("kb12345678901234567890123456789012", "page-1.png")

    def test_single_hyphen(self):
        """Simple bucket-key pairs still parse correctly."""
        assert base64_image.parse_storage_composite_id("bucket-file.jpg") == (
            "bucket",
            "file.jpg",
        )

    def test_invalid_ids(self):
        """Missing separator or empty parts return None."""
        assert base64_image.parse_storage_composite_id("nohyphen") is None
        assert base64_image.parse_storage_composite_id("-only-key") is None
        assert base64_image.parse_storage_composite_id("only-bucket-") is None
        assert base64_image.parse_storage_composite_id("") is None


@pytest.mark.p2
class TestId2Image:
    """Tests for id2image loading via composite IDs."""

    def test_loads_image_when_object_key_contains_hyphens(self):
        """id2image resolves bucket/key when objname includes hyphens."""
        storage_calls = []

        def fake_get(*, bucket, fnm):
            storage_calls.append((bucket, fnm))
            return base64_image.test_image

        image_id = "imagetemps-page-1.jpg"
        img = base64_image.id2image(image_id, partial(fake_get))
        assert storage_calls == [("imagetemps", "page-1.jpg")]
        assert isinstance(img, Image.Image)

    def test_returns_none_for_invalid_composite_id(self):
        """Invalid composite IDs short-circuit without calling storage."""
        called = []

        def fake_get(*, bucket, fnm):
            called.append((bucket, fnm))
            return base64_image.test_image

        assert base64_image.id2image("invalid", partial(fake_get)) is None
        assert called == []
