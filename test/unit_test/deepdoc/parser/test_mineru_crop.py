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

"""Unit tests for MinerUParser.crop() overlay logic.

Regression tests for issue #14197: content images were incorrectly darkened
when the top context strip had zero height and was skipped.
"""

import logging

import pytest
from PIL import Image

from deepdoc.parser.mineru_parser import MinerUParser


def _make_parser(page_images):
    parser = MinerUParser.__new__(MinerUParser)
    parser.page_images = page_images
    parser.page_from = 0
    parser.logger = logging.getLogger("test_mineru_crop")
    return parser


def _solid_image(color, size=(500, 800)):
    return Image.new("RGB", size, color)


def _sample_center(img):
    w, h = img.size
    return img.getpixel((w // 2, h // 2))


class TestCropOverlay:
    @pytest.mark.p2
    def test_content_not_darkened_when_top_context_skipped(self):
        """
        Bug #14197: image at top of page (top=0) produces a zero-height top
        context strip that is skipped. The content image must NOT be darkened.
        """
        RED = (255, 0, 0)
        parser = _make_parser([_solid_image(RED)])

        # top=0 → context strip above has zero height → skipped
        tag = "@@1\t50.0\t450.0\t0.0\t200.0##"
        result = parser.crop(tag)

        assert result is not None
        r, g, b = _sample_center(result)
        # darkened overlay would halve the red channel to ~127
        assert r > 200, f"Content image darkened (r={r}); overlay incorrectly applied to content"

    @pytest.mark.p2
    def test_content_not_darkened_when_image_near_top(self):
        """Image within GAP(6px) of page top also produces zero-height top strip."""
        RED = (255, 0, 0)
        parser = _make_parser([_solid_image(RED)])

        # top=4 → max(4-6, 0)=0 → zero-height top context strip
        tag = "@@1\t50.0\t450.0\t4.0\t200.0##"
        result = parser.crop(tag)

        assert result is not None
        r, g, b = _sample_center(result)
        assert r > 200, f"Content image darkened (r={r}); overlay incorrectly applied to content"

    @pytest.mark.p2
    def test_context_strips_are_darkened(self):
        """Context strips above and below content must receive the overlay."""
        WHITE = (255, 255, 255)
        parser = _make_parser([_solid_image(WHITE, size=(500, 800))])

        # Content in the middle of the page — both context strips are valid
        tag = "@@1\t50.0\t450.0\t300.0\t400.0##"
        result = parser.crop(tag)

        assert result is not None
        # Top-most pixel row should be from the darkened top context strip
        r, g, b = result.getpixel((result.size[0] // 2, 0))
        assert r < 200, f"Top context strip not darkened (r={r})"

    @pytest.mark.p1
    def test_single_image_not_darkened_when_both_context_strips_skipped(self):
        """
        Core bug from #14197: when both context strips are skipped (len(imgs)==1),
        the single content image must NOT receive the overlay.
        Original code: ii==0 AND ii+1==len(imgs) both True → always darkened.
        """
        RED = (255, 0, 0)
        # Page height = 200px; content fills full page → both context strips zero-height
        parser = _make_parser([_solid_image(RED, size=(500, 200))])

        # top=0, bottom=200 = page height → both context strips are zero-height → skipped
        tag = "@@1\t50.0\t450.0\t0.0\t200.0##"
        result = parser.crop(tag)

        assert result is not None
        r, g, b = _sample_center(result)
        # Before fix: r≈127 (darkened). After fix: r≈255 (clear).
        assert r > 200, f"Single content image darkened (r={r}); both-strips-skipped bug not fixed"

    @pytest.mark.p2
    def test_multi_page_content_not_darkened(self):
        """Content spanning multiple pages must not be darkened."""
        RED = (255, 0, 0)
        parser = _make_parser([_solid_image(RED), _solid_image(RED)])

        # top=0 on page 1, spans to page 2
        tag = "@@1-2\t50.0\t450.0\t0.0\t100.0##"
        result = parser.crop(tag)

        assert result is not None
        r, g, b = _sample_center(result)
        assert r > 200, f"Multi-page content image darkened (r={r})"
