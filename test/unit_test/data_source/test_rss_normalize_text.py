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
from common.data_source.rss_connector import RSSConnector


def test_inline_markup_stays_in_its_sentence():
    # get_text("\n") put every text node on a line of its own.
    html = '<p>The new <a href="https://example.com">release</a> adds <code>fast</code> mode.</p><ul><li>One</li><li>Two <i>items</i></li></ul>'
    assert RSSConnector._normalize_text(html) == "The new release adds fast mode.\n- One\n- Two items"


def test_plain_text_keeps_its_line_breaks():
    assert RSSConnector._normalize_text("  line one\nline two & more  ") == "line one\nline two & more"


def test_non_string_is_empty():
    assert RSSConnector._normalize_text(None) == ""
