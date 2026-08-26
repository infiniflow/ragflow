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
import importlib
import sys


def _load_utils_module(monkeypatch):
    # The deepdoc package tree can be left half-initialized in sys.modules by
    # earlier collection-time imports (circular import through
    # deepdoc/parser/__init__); purge and re-import so utils resolves cleanly.
    for module_name in ("rag.flow.parser.utils", "deepdoc.parser.pdf_parser", "deepdoc.parser", "deepdoc"):
        monkeypatch.delitem(sys.modules, module_name, raising=False)
    importlib.invalidate_caches()
    importlib.import_module("deepdoc.parser.pdf_parser")
    return importlib.import_module("rag.flow.parser.utils")


def test_remove_toc_drops_headingless_leader_entries(monkeypatch):
    utils = _load_utils_module(monkeypatch)
    items = [
        {"text": "《道德经》全文及翻译"},
        {"text": "前言：……………………………………………………………………1"},
        {"text": "第1章 “道”…………………………………………………………3"},
        {"text": "第13章 以身为天下，可寄/托天下……………………………18 I"},
        {"text": "第17章 太上，不知有之；功成事遂，百姓皆谓“我自然”..25"},
        {"text": "Introduction to the Dao ....... 12"},
        {"text": "II"},
        {"text": "前言：本文是《道德经》的白话翻译，供读者参考。"},
        {"text": "道可道，非常道。名可名，非常名。"},
    ]
    kept, kept_indices = utils.remove_toc(items)
    assert [item["text"] for item in kept] == [
        "《道德经》全文及翻译",
        "II",
        "前言：本文是《道德经》的白话翻译，供读者参考。",
        "道可道，非常道。名可名，非常名。",
    ]
    assert kept_indices == [0, 6, 7, 8]


def test_remove_toc_keeps_prose_ending_with_numbers(monkeypatch):
    utils = _load_utils_module(monkeypatch)
    items = [
        {"text": "全书共 81 章，约五千言。"},
        {"text": "公元前 571 年出生。"},
        {"text": "详见下一页。"},
    ]
    kept, kept_indices = utils.remove_toc(items)
    assert kept == items
    assert kept_indices == [0, 1, 2]
