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
"""Unit tests for the Obsidian local vault connector.

The ``common.data_source`` package namespace is stubbed so that importing
``obsidian_connector`` (and its lightweight sub-module deps) does **not**
trigger ``common/data_source/__init__.py``, which would pull in every
connector and their heavy transitive dependencies. This mirrors the RSS
connector unit test.
"""

import importlib
import os
import sys
import time
from datetime import UTC, datetime
from pathlib import Path
from types import ModuleType

import pytest

import common  # lightweight top-level package

repo_root = Path(__file__).resolve().parents[3]
data_source_pkg = ModuleType("common.data_source")
data_source_pkg.__path__ = [str(repo_root / "common" / "data_source")]
sys.modules["common.data_source"] = data_source_pkg
common.data_source = data_source_pkg

DocumentSource = importlib.import_module("common.data_source.config").DocumentSource
obsidian_module = importlib.import_module("common.data_source.obsidian_connector")
ObsidianConnector = obsidian_module.ObsidianConnector


def _write(path: Path, content: str) -> Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")
    return path


def _docs(connector):
    return [doc for batch in connector.load_from_state() for doc in batch]


# --- validation -------------------------------------------------------------


@pytest.mark.p2
def test_validate_rejects_empty_vault_path():
    with pytest.raises(ValueError, match="vault_path is required"):
        ObsidianConnector(vault_path="").validate_connector_settings()


@pytest.mark.p2
def test_validate_rejects_relative_path():
    with pytest.raises(ValueError, match="absolute directory path"):
        ObsidianConnector(vault_path="relative/vault").validate_connector_settings()


@pytest.mark.p2
def test_validate_rejects_nonexistent_path(tmp_path):
    missing = tmp_path / "does-not-exist"
    with pytest.raises(ValueError, match="existing directory"):
        ObsidianConnector(vault_path=str(missing)).validate_connector_settings()


@pytest.mark.p2
def test_validate_rejects_zero_batch_size(tmp_path):
    with pytest.raises(ValueError, match="greater than 0"):
        ObsidianConnector(vault_path=str(tmp_path), batch_size=0).validate_connector_settings()


# --- document generation ----------------------------------------------------


@pytest.mark.p1
def test_load_from_state_builds_basic_document(tmp_path):
    _write(tmp_path / "note.md", "# Title\n\nSome body text.")

    connector = ObsidianConnector(vault_path=str(tmp_path))
    docs = _docs(connector)

    assert len(docs) == 1
    doc = docs[0]
    assert doc.source == DocumentSource.OBSIDIAN
    assert doc.semantic_identifier == "note.md"
    assert doc.extension == ".md"
    assert b"# Title" in doc.blob
    assert doc.size_bytes == len(doc.blob)
    assert doc.metadata["path"] == "note.md"
    # id is a stable hex digest keyed on the relative path.
    assert doc.id.startswith("obsidian:")


@pytest.mark.p1
def test_load_from_state_strips_and_preserves_frontmatter(tmp_path):
    _write(
        tmp_path / "note.md",
        "---\ntags: [idea, draft]\ntitle: My Note\n---\n\nBody after frontmatter.",
    )

    connector = ObsidianConnector(vault_path=str(tmp_path))
    doc = _docs(connector)[0]

    # Frontmatter must not leak into the indexed body.
    assert b"---" not in doc.blob
    assert b"Body after frontmatter" in doc.blob
    # Parsed frontmatter surfaces as structured metadata.
    assert doc.metadata["frontmatter"] == {"tags": ["idea", "draft"], "title": "My Note"}


@pytest.mark.p1
def test_load_from_state_parses_wikilinks(tmp_path):
    _write(
        tmp_path / "note.md",
        "See [[Daily Notes]] and [[projects/alpha|the alpha project]].",
    )

    connector = ObsidianConnector(vault_path=str(tmp_path))
    doc = _docs(connector)[0]

    body = doc.blob.decode("utf-8")
    # Display text replaces the bracket syntax.
    assert "[[" not in body
    assert "Daily Notes" in body
    assert "the alpha project" in body
    # Link targets are collected in metadata.
    assert doc.metadata["links"] == ["Daily Notes", "projects/alpha"]


@pytest.mark.p1
def test_load_from_state_skips_obsidian_and_trash_dirs(tmp_path):
    _write(tmp_path / "real.md", "keep me")
    # .obsidian holds app config, .trash holds soft-deleted notes.
    _write(tmp_path / ".obsidian" / "app.json", "{}")
    _write(tmp_path / ".obsidian" / "config.md", "should skip")
    _write(tmp_path / ".trash" / "deleted.md", "should skip")
    _write(tmp_path / ".hidden.md", "should skip")

    connector = ObsidianConnector(vault_path=str(tmp_path))
    docs = _docs(connector)

    assert [d.semantic_identifier for d in docs] == ["real.md"]


@pytest.mark.p1
def test_load_from_state_indexes_nested_notes(tmp_path):
    _write(tmp_path / "folder" / "sub" / "deep.md", "deep note")

    connector = ObsidianConnector(vault_path=str(tmp_path))
    doc = _docs(connector)[0]

    expected = os.path.join("folder", "sub", "deep.md")
    assert doc.semantic_identifier == expected
    assert doc.metadata["path"] == expected


@pytest.mark.p2
def test_batching_yields_in_configured_batch_sizes(tmp_path):
    for i in range(3):
        _write(tmp_path / f"n{i}.md", f"note {i}")

    connector = ObsidianConnector(vault_path=str(tmp_path), batch_size=2)
    batches = list(connector.load_from_state())

    assert len(batches) == 2
    assert len(batches[0]) == 2
    assert len(batches[1]) == 1


@pytest.mark.p2
def test_document_id_is_stable_across_content_edits(tmp_path):
    path = _write(tmp_path / "note.md", "original")
    connector = ObsidianConnector(vault_path=str(tmp_path))
    first_id = _docs(connector)[0].id

    # Change content but keep the same path; the id must not change.
    path.write_text("edited content", encoding="utf-8")
    second_id = _docs(connector)[0].id

    assert first_id == second_id


@pytest.mark.p2
def test_size_bytes_is_byte_length_not_char_count(tmp_path):
    # Non-ASCII chars occupy more than one byte in UTF-8.
    _write(tmp_path / "note.md", "中文")

    connector = ObsidianConnector(vault_path=str(tmp_path))
    doc = _docs(connector)[0]

    assert doc.size_bytes == len("中文".encode())


# --- polling ----------------------------------------------------------------


@pytest.mark.p1
def test_poll_source_filters_by_mtime_window(tmp_path):
    _write(tmp_path / "old.md", "old note")
    old_path = tmp_path / "old.md"
    # Backdate "old.md" so it falls outside the poll window.
    old_ts = time.time() - 10_000
    os.utime(old_path, (old_ts, old_ts))
    _write(tmp_path / "new.md", "new note")

    connector = ObsidianConnector(vault_path=str(tmp_path))
    start = datetime.now(UTC).timestamp() - 5_000
    end = datetime.now(UTC).timestamp()

    docs = [doc for batch in connector.poll_source(start, end) for doc in batch]

    assert [d.semantic_identifier for d in docs] == ["new.md"]


@pytest.mark.p2
def test_load_credentials_is_noop_for_credentials():
    connector = ObsidianConnector(vault_path="/tmp")
    # Obsidian reads from the local filesystem; credentials are not required.
    connector.load_credentials({"token": "anything"})
    assert connector.credentials == {"token": "anything"}


# --- frontmatter edge cases -------------------------------------------------


@pytest.mark.p2
def test_frontmatter_falls_back_to_raw_when_yaml_invalid(tmp_path):
    _write(
        tmp_path / "note.md",
        "---\nthis: is: not: valid: yaml: [[[\n---\n\nBody text.",
    )

    connector = ObsidianConnector(vault_path=str(tmp_path))
    doc = _docs(connector)[0]

    # Body is still extracted cleanly.
    assert b"Body text" in doc.blob
    # Unparseable frontmatter is preserved verbatim instead of being dropped.
    assert "frontmatter_raw" in doc.metadata
    assert "frontmatter" not in doc.metadata


@pytest.mark.p2
def test_note_without_frontmatter_is_indexed_unchanged(tmp_path):
    _write(tmp_path / "note.md", "Just a plain note with [[a link]].")

    connector = ObsidianConnector(vault_path=str(tmp_path))
    doc = _docs(connector)[0]

    assert "frontmatter" not in doc.metadata
    assert "frontmatter_raw" not in doc.metadata
    assert doc.metadata["links"] == ["a link"]
