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
"""HTML to Markdown conversion for the ingestion paths."""

from typing import Any

from markdownify import MarkdownConverter

# Inline tags whose markdownify conversion runs the text through chomp(), which lifts the
# surrounding whitespace out of the text and then returns an empty string once nothing is
# left. A whitespace-only element therefore disappears together with its whitespace and the
# words on either side of it run together. A word processor keeps a differently formatted
# space as a run of its own, so `First<strong> </strong>Last` is what a .docx with a bolded
# space converts to, and `FirstLast` is what gets chunked, embedded and retrieved.
_WHITESPACE_ONLY_PRESERVING_TAGS = frozenset(
    {
        "a",
        "b",
        "code",
        "del",
        "em",
        "i",
        "kbd",
        "s",
        "samp",
        "strike",
        "strong",
        "sub",
        "sup",
        "u",
    }
)


class _WhitespacePreservingConverter(MarkdownConverter):
    """Same as markdownify's converter, but a whitespace-only inline element keeps its text."""

    def get_conv_fn(self, tag_name: str) -> Any:
        convert_fn = super().get_conv_fn(tag_name)
        if convert_fn is None or tag_name.lower() not in _WHITESPACE_ONLY_PRESERVING_TAGS:
            return convert_fn

        def _keep_whitespace_only(el: Any, text: str, *args: Any, **kwargs: Any) -> str:
            if not text.strip():
                return text

            return convert_fn(el, text, *args, **kwargs)

        return _keep_whitespace_only


def html_to_markdown(html: str, **options: Any) -> str:
    """Convert `html` to Markdown without losing the word boundaries it carries.

    `options` are markdownify's own, so this is a drop-in for `markdownify(html, ...)`.
    """
    return _WhitespacePreservingConverter(**options).convert(html)
