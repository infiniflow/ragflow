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

"""The API default delimiters must be an actual newline, not the raw 2-char
escape ``r"\\n"``.

A default stored as ``r"\\n"`` reaches ``rag.nlp.delim.parse_delimiter_field`` as
a backslash followed by the letter ``n``. The canonical parser honours bare
characters, so it then splits documents on every ``n`` (``"information"`` ->
``"i"``, ``"formatio"``) and on the stray backslash. An actual newline parses to
a single newline delimiter. The frontend already sends a real newline and the
``naive`` default already uses one; these guard the ``knowledge_graph`` and
``ParserConfig`` / ``ParentChildConfig`` defaults that did not.
"""

from api.utils.api_utils import get_parser_config
from api.utils.validation_utils import ParentChildConfig, ParserConfig
from rag.nlp.delim import parse_delimiter_field


def _assert_real_newline(delimiter):
    assert delimiter == "\n", f"default delimiter should be a real newline, got {delimiter!r}"
    # A real newline parses to a single newline delimiter. The raw escape would
    # instead parse to ["\\", "n"] and shred the document on the letter n.
    assert parse_delimiter_field(delimiter) == ["\n"]


def test_parser_config_delimiter_default_is_real_newline():
    _assert_real_newline(ParserConfig().delimiter)


def test_parent_child_children_delimiter_default_is_real_newline():
    _assert_real_newline(ParentChildConfig().children_delimiter)


def test_knowledge_graph_delimiter_default_is_real_newline():
    _assert_real_newline(get_parser_config("knowledge_graph", None)["delimiter"])
