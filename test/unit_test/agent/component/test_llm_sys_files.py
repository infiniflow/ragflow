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

from types import SimpleNamespace

import pytest

from agent.component.llm import LLM

pytestmark = pytest.mark.p1


class _FakeCanvas:
    def __init__(self, sys_files=None):
        self.globals = {"sys.files": list(sys_files) if sys_files is not None else []}


def _build_component(sys_files=None, sys_prompt="", prompts=None):
    component = LLM.__new__(LLM)
    component._canvas = _FakeCanvas(sys_files=sys_files)
    component._param = SimpleNamespace(
        sys_prompt=sys_prompt,
        prompts=prompts if prompts is not None else [{"role": "user", "content": "{sys.query}"}],
    )
    return component


def test_collect_sys_files_empty_returns_empty():
    component = _build_component(sys_files=[])
    assert component._collect_sys_files() == ([], [])


def test_collect_sys_files_missing_key_returns_empty():
    component = _build_component()
    component._canvas.globals.pop("sys.files", None)
    assert component._collect_sys_files() == ([], [])


def test_collect_sys_files_text_only():
    files = ["File: a.pdf\ncontent A", "File: b.txt\ncontent B"]
    component = _build_component(sys_files=files)
    text_parts, image_data_uris = component._collect_sys_files()
    assert text_parts == files
    assert image_data_uris == []


def test_collect_sys_files_images_only():
    files = ["data:image/png;base64,AAAA", "data:image/jpeg;base64,BBBB"]
    component = _build_component(sys_files=files)
    text_parts, image_data_uris = component._collect_sys_files()
    assert text_parts == []
    assert image_data_uris == files


def test_collect_sys_files_mixed():
    files = [
        "File: a.pdf\ncontent A",
        "data:image/png;base64,AAAA",
        "File: b.txt\ncontent B",
    ]
    component = _build_component(sys_files=files)
    text_parts, image_data_uris = component._collect_sys_files()
    assert text_parts == ["File: a.pdf\ncontent A", "File: b.txt\ncontent B"]
    assert image_data_uris == ["data:image/png;base64,AAAA"]


def test_collect_sys_files_skips_non_str_entries():
    files = ["File: a.pdf\ncontent A", None, 123, {"name": "x"}, "data:image/png;base64,AAAA"]
    component = _build_component(sys_files=files)
    text_parts, image_data_uris = component._collect_sys_files()
    assert text_parts == ["File: a.pdf\ncontent A"]
    assert image_data_uris == ["data:image/png;base64,AAAA"]


def test_collect_sys_files_explicit_in_sys_prompt_skips_injection():
    files = ["File: a.pdf\ncontent A", "data:image/png;base64,AAAA"]
    component = _build_component(
        sys_files=files,
        sys_prompt="Answer using {sys.files} as context.",
    )
    assert component._collect_sys_files() == ([], [])


def test_collect_sys_files_explicit_in_prompts_entry_skips_injection():
    files = ["File: a.pdf\ncontent A"]
    component = _build_component(
        sys_files=files,
        prompts=[{"role": "user", "content": "{sys.query}\n\n{sys.files}"}],
    )
    assert component._collect_sys_files() == ([], [])


def test_collect_sys_files_string_prompts_does_not_crash():
    # _param.prompts can be a string before normalization elsewhere; the explicit
    # check must not raise in that case, it just falls through to splitting.
    files = ["File: a.pdf\ncontent A"]
    component = _build_component(sys_files=files, prompts="some raw template")
    text_parts, image_data_uris = component._collect_sys_files()
    assert text_parts == files
    assert image_data_uris == []
