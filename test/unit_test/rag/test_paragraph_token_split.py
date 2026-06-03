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

"""Unit test for ``FulltextQueryer.paragraph`` token handling.

Regression: callers (e.g. ``Dealer.tag_content``) pass a *space-joined token
string* such as ``title_tks + " " + content_ltks``. ``paragraph`` previously
iterated that string character-by-character (``[c.strip() for c in
content_tks.strip()]``), so ``self.tw.weights`` received single characters
instead of whole tokens and the tag-feature query weights were meaningless.
"""

from unittest.mock import MagicMock

from rag.nlp.query import FulltextQueryer


def test_paragraph_splits_token_string_on_whitespace():
    # Bypass the heavy __init__ (term-weight / synonym dictionary loading).
    q = FulltextQueryer.__new__(FulltextQueryer)

    captured = {}

    def fake_weights(tks, preprocess=False):
        captured["tks"] = tks
        return []

    q.tw = MagicMock()
    q.tw.weights.side_effect = fake_weights
    q.syn = MagicMock()

    q.paragraph("foo bar baz")

    # Whole tokens, not individual characters.
    assert captured["tks"] == ["foo", "bar", "baz"]
