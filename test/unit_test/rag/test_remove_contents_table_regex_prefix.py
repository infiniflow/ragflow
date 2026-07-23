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

import sys
import types
from contextlib import contextmanager
from unittest import mock

import pytest


@contextmanager
def _stubbed_heavy_imports():
    """Temporarily register lightweight stand-ins for the heavy modules that
    ``rag.nlp`` imports at module load time, so ``remove_contents_table`` can be
    imported in a stripped-down environment that lacks them.

    ``mock.patch.dict`` snapshots ``sys.modules`` on entry and fully restores it
    on exit, so none of these stubs -- nor ``rag``/``rag.nlp`` themselves -- leak
    into the rest of the pytest session. Later tests therefore still import the
    real modules regardless of collection order.
    """
    token_utils = types.ModuleType("common.token_utils")
    token_utils.num_tokens_from_string = lambda *a, **k: 0

    word2number = types.ModuleType("word2number")
    word2number.w2n = types.SimpleNamespace()

    cn2an = types.ModuleType("cn2an")
    cn2an.cn2an = lambda *a, **k: 0

    pil = types.ModuleType("PIL")
    pil_image = types.ModuleType("PIL.Image")
    pil.Image = pil_image

    stubs = {
        "common.token_utils": token_utils,
        "roman_numbers": types.ModuleType("roman_numbers"),
        "word2number": word2number,
        "cn2an": cn2an,
        "PIL": pil,
        "PIL.Image": pil_image,
        "chardet": types.ModuleType("chardet"),
    }
    with mock.patch.dict(sys.modules, stubs):
        yield


@pytest.fixture(scope="module")
def remove_contents_table():
    """Provide the real ``rag.nlp.remove_contents_table``.

    Import it directly when the environment already has ``rag.nlp``'s
    dependencies; otherwise import it once under :func:`_stubbed_heavy_imports`,
    which cleans up after itself so nothing leaks into ``sys.modules``.
    """
    try:
        from rag.nlp import remove_contents_table as fn
    except ImportError:
        with _stubbed_heavy_imports():
            from rag.nlp import remove_contents_table as fn
    return fn


@pytest.mark.p2
def test_metachar_prefix_does_not_crash_chinese(remove_contents_table):
    # Regression: the heading right after the table-of-contents marker starts
    # with an unbalanced "(", whose first 3 chars ("1.(") used to be fed to
    # re.match() as a *pattern* and raised re.error ("missing ), unterminated
    # subpattern"), aborting the whole book/laws parse.
    sections = ["目录", "1.(1) 引言", "some body text that follows the heading"]
    remove_contents_table(sections, eng=False)
    assert sections == ["some body text that follows the heading"]


@pytest.mark.p2
def test_metachar_prefix_does_not_crash_english(remove_contents_table):
    sections = ["contents", "[1] Overview", "body paragraph"]
    remove_contents_table(sections, eng=True)
    assert sections == ["body paragraph"]


@pytest.mark.p2
def test_literal_prefix_still_removes_toc_block(remove_contents_table):
    # The prefix "第(1" contains a regex metacharacter, yet the block between
    # the table-of-contents entries and the real chapter start must still be
    # removed by matching the prefix literally.
    sections = ["目录", "第(1)章 引言", "第(2)章 方法", "第(1)章 引言", "正文内容"]
    remove_contents_table(sections, eng=False)
    assert sections == ["第(1)章 引言", "正文内容"]


@pytest.mark.p2
def test_non_toc_sections_are_left_untouched(remove_contents_table):
    sections = ["Introduction", "First paragraph.", "Second paragraph."]
    remove_contents_table(sections, eng=True)
    assert sections == ["Introduction", "First paragraph.", "Second paragraph."]
