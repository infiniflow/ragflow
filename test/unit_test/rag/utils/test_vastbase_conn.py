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
from rag.utils.vastbase_conn import field_keyword, quote_ident


class TestVastbaseHelpers:
    def test_quote_ident_escapes_quotes(self):
        assert quote_ident('foo"bar') == '"foo""bar"'

    def test_field_keyword_rules(self):
        assert field_keyword("source_id") is True
        assert field_keyword("tag_kwd") is True
        assert field_keyword("docnm_kwd") is False
        assert field_keyword("content_ltks") is False
