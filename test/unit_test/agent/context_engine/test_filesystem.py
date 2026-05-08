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

import pytest

from agent.context_engine import ContextEngineFS


@pytest.mark.p2
def test_context_engine_filesystem_read_write_list_and_remove(tmp_path):
    fs = ContextEngineFS(tmp_path)

    info = fs.write_text("sessions/s1/context.md", "hello")

    assert info.path == "sessions/s1/context.md"
    assert info.size == 5
    assert fs.read_text("sessions/s1/context.md") == "hello"
    assert [item.path for item in fs.list("sessions/s1")] == ["sessions/s1/context.md"]
    assert [item.path for item in fs.walk()] == [
        "sessions",
        "sessions/s1",
        "sessions/s1/context.md",
    ]

    fs.remove("sessions")
    assert not fs.exists("sessions/s1/context.md")


@pytest.mark.p2
@pytest.mark.parametrize(
    "unsafe_path",
    ["", "/tmp/context.md", "../context.md", "safe/../context.md"],
)
def test_context_engine_filesystem_rejects_unsafe_paths(tmp_path, unsafe_path):
    fs = ContextEngineFS(tmp_path)

    with pytest.raises(ValueError):
        fs.write_text(unsafe_path, "blocked")


@pytest.mark.p2
def test_context_engine_filesystem_rejects_symlink_escape(tmp_path):
    fs = ContextEngineFS(tmp_path / "root")
    outside = tmp_path / "outside"
    outside.mkdir()
    (fs.root / "link").symlink_to(outside, target_is_directory=True)

    with pytest.raises(ValueError):
        fs.write_text("link/context.md", "blocked")


@pytest.mark.p2
@pytest.mark.parametrize(
    "operation",
    ["exists", "mkdir", "read_text", "write_text", "stat", "remove"],
)
def test_context_engine_filesystem_rejects_empty_paths(tmp_path, operation):
    fs = ContextEngineFS(tmp_path)

    with pytest.raises(ValueError):
        if operation == "write_text":
            fs.write_text("", "blocked")
        else:
            getattr(fs, operation)("")


@pytest.mark.p2
def test_context_engine_filesystem_list_and_walk_skip_symlink_escape(tmp_path):
    fs = ContextEngineFS(tmp_path / "root")
    fs.write_text("safe/context.md", "hello")
    outside = tmp_path / "outside"
    outside.mkdir()
    (fs.root / "escape").symlink_to(outside, target_is_directory=True)

    assert [item.path for item in fs.list()] == ["safe"]
    assert [item.path for item in fs.walk()] == ["safe", "safe/context.md"]


@pytest.mark.p2
def test_context_engine_filesystem_remove_unlinks_symlink_without_deleting_target(tmp_path):
    fs = ContextEngineFS(tmp_path / "root")
    outside = tmp_path / "outside"
    outside.mkdir()
    target_file = outside / "keep.md"
    target_file.write_text("keep", encoding="utf-8")
    link = fs.root / "escape"
    link.symlink_to(outside, target_is_directory=True)

    fs.remove("escape")

    assert not link.exists()
    assert target_file.read_text(encoding="utf-8") == "keep"


@pytest.mark.p2
def test_context_engine_filesystem_preserves_requested_path_for_in_root_symlink(tmp_path):
    fs = ContextEngineFS(tmp_path / "root")
    fs.mkdir("actual")
    (fs.root / "alias").symlink_to(fs.root / "actual", target_is_directory=True)

    info = fs.write_text("alias/context.md", "hello")

    assert info.path == "alias/context.md"
    assert fs.stat("alias/context.md").path == "alias/context.md"
    assert fs.read_text("actual/context.md") == "hello"
