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

"""Root-confined filesystem primitives for ContextEngine state."""

from __future__ import annotations

import logging
import os
import shutil
from dataclasses import dataclass
from pathlib import Path, PurePosixPath
from typing import Iterator

logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class ContextEngineFile:
    """Metadata for a file or directory exposed by ContextEngineFS."""

    path: str
    is_dir: bool
    size: int


class ContextEngineFS:
    """Root-confined filesystem for ContextEngine state.

    Paths are POSIX-style and always resolved below ``root``. Absolute paths,
    parent traversal, and symlink escapes are rejected so callers can expose the
    abstraction to agent workflows without granting ambient filesystem access.
    """

    def __init__(self, root: str | os.PathLike[str]):
        """Create a filesystem rooted at ``root`` and ensure it exists."""
        self.root = Path(root).expanduser().resolve()
        self.root.mkdir(parents=True, exist_ok=True)

    @staticmethod
    def _clean_context_path(path: str | os.PathLike[str]) -> PurePosixPath:
        pure = PurePosixPath(str(path))
        if not pure.parts:
            logger.warning("ContextEngineFS rejected empty path")
            raise ValueError(f"ContextEngineFS path must be non-empty: {path}")
        if pure.is_absolute():
            logger.warning("ContextEngineFS rejected absolute path: %s", path)
            raise ValueError(f"ContextEngineFS path must be relative: {path}")
        if any(part in {"", ".", ".."} for part in pure.parts):
            logger.warning("ContextEngineFS rejected unsafe path segment: %s", path)
            raise ValueError(f"ContextEngineFS path contains an unsafe segment: {path}")
        return pure

    def _resolve(self, path: str | os.PathLike[str]) -> Path:
        pure = self._clean_context_path(path)
        resolved = (self.root / Path(*pure.parts)).resolve()
        if not resolved.is_relative_to(self.root):
            logger.warning("ContextEngineFS rejected path escaping root: %s", path)
            raise ValueError(f"ContextEngineFS path escapes root: {path}")
        return resolved

    @staticmethod
    def _to_context_path(root: Path, path: Path) -> str:
        rel = path.relative_to(root)
        return rel.as_posix()

    def _describe(self, path: Path, context_path: str | None = None) -> ContextEngineFile | None:
        try:
            resolved = path.resolve()
            if not resolved.is_relative_to(self.root):
                logger.debug("ContextEngineFS skipped path escaping root during describe: %s", path)
                return None
            st = path.stat()
            return ContextEngineFile(
                path=context_path or self._to_context_path(self.root, path),
                is_dir=path.is_dir(),
                size=0 if path.is_dir() else st.st_size,
            )
        except FileNotFoundError:
            logger.debug("ContextEngineFS skipped missing path during describe: %s", path)
            return None
        except ValueError:
            logger.debug("ContextEngineFS skipped invalid path during describe: %s", path)
            return None

    def exists(self, path: str | os.PathLike[str]) -> bool:
        """Return whether a validated context path exists."""
        return self._resolve(path).exists()

    def mkdir(self, path: str | os.PathLike[str]) -> None:
        """Create a validated context directory and any missing parents."""
        target = self._resolve(path)
        target.mkdir(parents=True, exist_ok=True)
        logger.debug("ContextEngineFS.mkdir created %s parents=True exist_ok=True", target)

    def write_bytes(self, path: str | os.PathLike[str], content: bytes) -> ContextEngineFile:
        """Write bytes to a validated context path and return its metadata."""
        requested_path = self._clean_context_path(path).as_posix()
        target = self._resolve(path)
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(content)
        logger.debug("ContextEngineFS.write_bytes wrote %d bytes to %s", len(content), target)
        return self.stat(requested_path)

    def read_bytes(self, path: str | os.PathLike[str]) -> bytes:
        """Read bytes from a validated context file."""
        target = self._resolve(path)
        if not target.is_file():
            raise FileNotFoundError(str(path))
        return target.read_bytes()

    def write_text(self, path: str | os.PathLike[str], content: str, encoding: str = "utf-8") -> ContextEngineFile:
        """Write text to a validated context path and return its metadata."""
        return self.write_bytes(path, content.encode(encoding))

    def read_text(self, path: str | os.PathLike[str], encoding: str = "utf-8") -> str:
        """Read text from a validated context file."""
        return self.read_bytes(path).decode(encoding)

    def stat(self, path: str | os.PathLike[str]) -> ContextEngineFile:
        """Return metadata for a validated context path."""
        requested_path = self._clean_context_path(path).as_posix()
        target = self._resolve(path)
        st = target.stat()
        return ContextEngineFile(
            path=requested_path,
            is_dir=target.is_dir(),
            size=0 if target.is_dir() else st.st_size,
        )

    def list(self, path: str | os.PathLike[str] = "") -> list[ContextEngineFile]:
        """List direct safe entries below a context directory."""
        base = self.root if str(path) == "" else self._resolve(path)
        if not base.is_dir():
            raise NotADirectoryError(str(path))
        entries = []
        for child in sorted(base.iterdir()):
            entry = self._describe(child)
            if entry is not None:
                entries.append(entry)
        return entries

    def walk(self, path: str | os.PathLike[str] = "") -> Iterator[ContextEngineFile]:
        """Yield safe entries recursively below a context directory."""
        base = self.root if str(path) == "" else self._resolve(path)
        if not base.is_dir():
            raise NotADirectoryError(str(path))
        for current, dirs, files in os.walk(base):
            dirs.sort()
            files.sort()
            current_path = Path(current)
            if current_path != base:
                current_entry = self._describe(current_path)
                if current_entry is not None:
                    yield current_entry
            for name in files:
                entry = self._describe(current_path / name)
                if entry is not None:
                    yield entry

    def remove(self, path: str | os.PathLike[str]) -> None:
        """Remove a validated context file, directory, or symlink."""
        pure = self._clean_context_path(path)
        parent = self.root if len(pure.parts) == 1 else self._resolve(PurePosixPath(*pure.parts[:-1]).as_posix())
        target = parent / pure.parts[-1]
        if not parent.is_relative_to(self.root):
            logger.warning("ContextEngineFS rejected remove path escaping root: %s", path)
            raise ValueError(f"ContextEngineFS path escapes root: {path}")
        if target.is_symlink():
            target.unlink()
        elif target.is_dir():
            shutil.rmtree(target)
        else:
            target.unlink()
        logger.debug("ContextEngineFS.remove removed %s", target)
