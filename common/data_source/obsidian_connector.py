"""Obsidian local vault data source connector.

Ingests markdown notes from a local Obsidian vault directory. Each ``.md``
file becomes one ``Document``. The connector parses YAML frontmatter and
``[[wikilink]]`` references so they surface as searchable metadata instead
of polluting the indexed body.

Obsidian vaults are plain folders on disk, so this connector only needs the
standard library plus an optional ``ruamel.yaml`` dependency for structured
frontmatter parsing (it falls back to raw text when unavailable).
"""

from __future__ import annotations

import hashlib
import os
import re
from collections.abc import Iterator
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from common.data_source.config import INDEX_BATCH_SIZE, DocumentSource
from common.data_source.interfaces import LoadConnector, PollConnector
from common.data_source.models import Document, GenerateDocumentsOutput, SecondsSinceUnixEpoch

# Directories that are part of Obsidian's own bookkeeping, not user notes.
_SKIPPED_DIR_PARTS = {".obsidian", ".trash", ".git"}
_MARKDOWN_SUFFIX = ".md"
_WIKILINK_RE = re.compile(r"\[\[([^\[\]]+)\]\]")
_FRONTMATTER_RE = re.compile(r"\A---\r?\n(.*?)\r?\n---\r?\n?(.*)\Z", re.DOTALL)


def _parse_yaml(block: str) -> dict[str, Any] | None:
    """Best-effort YAML parse; returns ``None`` when the dependency is missing
    or the block is malformed so the caller can fall back to raw text."""
    try:
        from ruamel.yaml import YAML
        from ruamel.yaml.error import YAMLError
    except ImportError:
        return None

    try:
        data = YAML(typ="safe").load(block)
    except YAMLError:
        return None

    return data if isinstance(data, dict) else None


def _split_frontmatter(text: str) -> tuple[str, dict[str, Any] | None, str | None]:
    """Split a markdown note into ``(body, frontmatter_dict, raw_frontmatter)``.

    ``frontmatter_dict`` is ``None`` when frontmatter is absent or unparseable;
    ``raw_frontmatter`` carries the verbatim YAML text so it is never lost.
    """
    match = _FRONTMATTER_RE.match(text)
    if not match:
        return text, None, None

    raw, body = match.group(1), match.group(2)
    return body, _parse_yaml(raw), raw


def _extract_wikilinks(body: str) -> tuple[str, list[str]]:
    """Replace ``[[wikilinks]]`` with their display text and return the targets."""
    targets: list[str] = []

    def _replace(match: re.Match[str]) -> str:
        inner = match.group(1)
        # ``[[target|alias]]`` shows ``alias`` but links to ``target``;
        # ``[[target]]`` uses ``target`` for both.
        if "|" in inner:
            target, _, alias = inner.partition("|")
        else:
            target, alias = inner, inner
        target = target.strip()
        if target:
            targets.append(target)
        return alias.strip()

    cleaned = _WIKILINK_RE.sub(_replace, body)
    return cleaned, targets


class ObsidianConnector(LoadConnector, PollConnector):
    """Connector for a local Obsidian vault directory."""

    def __init__(self, vault_path: str, batch_size: int = INDEX_BATCH_SIZE) -> None:
        self.vault_path = (vault_path or "").strip()
        self.batch_size = batch_size
        self.credentials: dict[str, Any] = {}

    def load_credentials(self, credentials: dict[str, Any]) -> None:
        self.credentials = credentials or {}

    def validate_connector_settings(self) -> None:
        if not self.vault_path:
            raise ValueError("vault_path is required")
        if not os.path.isabs(self.vault_path):
            raise ValueError("vault_path must be an absolute directory path")
        if not os.path.isdir(self.vault_path):
            raise ValueError(f"vault_path must be an existing directory: {self.vault_path}")
        if self.batch_size < 1:
            raise ValueError("batch_size must be greater than 0")

    def load_from_state(self) -> GenerateDocumentsOutput:
        yield from self._load_files()

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> GenerateDocumentsOutput:
        yield from self._load_files(start=start, end=end)

    def _iter_markdown(self) -> Iterator[Path]:
        vault = Path(self.vault_path)
        for path in vault.rglob(f"*{_MARKDOWN_SUFFIX}"):
            if _SKIPPED_DIR_PARTS & set(path.parts) or path.name.startswith("."):
                continue
            yield path

    def _load_files(
        self,
        start: float | None = None,
        end: float | None = None,
    ) -> GenerateDocumentsOutput:
        batch: list[Document] = []
        for path in self._iter_markdown():
            mtime = path.stat().st_mtime
            if start is not None and mtime <= start:
                continue
            if end is not None and mtime > end:
                continue

            text = path.read_text(encoding="utf-8", errors="ignore")
            batch.append(self._build_document(path, text, mtime))

            if len(batch) >= self.batch_size:
                yield batch
                batch = []

        if batch:
            yield batch

    def _build_document(self, path: Path, text: str, mtime: float) -> Document:
        body, frontmatter, raw_frontmatter = _split_frontmatter(text)
        body, links = _extract_wikilinks(body)

        rel = os.path.relpath(path, self.vault_path)
        blob = body.encode("utf-8")
        metadata: dict[str, Any] = {"path": rel}
        if frontmatter is not None:
            metadata["frontmatter"] = frontmatter
        elif raw_frontmatter is not None:
            metadata["frontmatter_raw"] = raw_frontmatter
        if links:
            metadata["links"] = links

        return Document(
            id=f"obsidian:{hashlib.md5(rel.encode('utf-8')).hexdigest()}",
            source=DocumentSource.OBSIDIAN,
            semantic_identifier=rel,
            extension=_MARKDOWN_SUFFIX,
            blob=blob,
            doc_updated_at=datetime.fromtimestamp(mtime, tz=UTC),
            size_bytes=len(blob),
            metadata=metadata,
        )
