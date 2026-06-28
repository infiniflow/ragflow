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

"""
PipelineChunker Component

Run RAGFlow Pipeline-style chunkers (rag.app.*) against uploaded files inside an
Agent workflow. Emits plain text chunks for downstream Agent nodes — no
embedding, no persistence. Wraps existing chunker functions; does not
re-implement chunking logic.
"""

import importlib
import logging
import os
from abc import ABC

from agent.component.base import ComponentBase, ComponentParamBase
from api.db.services.file_service import FileService
from common.connection_utils import timeout


# Parser id -> dotted module path under rag.app. Imported lazily so we don't
# pull deepdoc/OCR/VLM machinery at component-discovery time.
_PARSER_MODULES: dict[str, str] = {
    "general": "rag.app.naive",
    "naive": "rag.app.naive",
    "paper": "rag.app.paper",
    "book": "rag.app.book",
    "presentation": "rag.app.presentation",
    "manual": "rag.app.manual",
    "laws": "rag.app.laws",
    "qa": "rag.app.qa",
    "table": "rag.app.table",
    "resume": "rag.app.resume",
    "picture": "rag.app.picture",
    "one": "rag.app.one",
    "audio": "rag.app.audio",
    "email": "rag.app.email",
    "tag": "rag.app.tag",
}


def _load_chunker(parser_id: str):
    """Resolve a parser id to the underlying ``rag.app.<module>.chunk`` callable."""
    module_path = _PARSER_MODULES[parser_id.lower()]
    return importlib.import_module(module_path).chunk


class PipelineChunkerParam(ComponentParamBase):
    """
    Define the PipelineChunker component parameters.
    """

    def __init__(self):
        """Initialise PipelineChunker defaults and declare component outputs."""
        super().__init__()
        self.inputs = []  # variable references to uploaded files
        self.parser_id = "naive"
        self.lang = "English"
        self.from_page = 0
        self.to_page = 100000000
        self.parser_config = {}

        self.outputs = {
            "chunks": {"type": "list", "value": []},
            "chunks_full": {"type": "list", "value": []},
            "summary": {"type": "str", "value": ""},
        }

    def check(self):
        """Validate parser id, page range, and parser_config shape."""
        self.check_valid_value(
            self.parser_id.lower(),
            "[PipelineChunker] parser_id",
            list(_PARSER_MODULES.keys()),
        )
        self.check_nonnegative_number(self.from_page, "[PipelineChunker] from_page")
        self.check_nonnegative_number(self.to_page, "[PipelineChunker] to_page")
        if isinstance(self.from_page, (int, float)) and isinstance(self.to_page, (int, float)) and self.from_page > self.to_page:
            raise ValueError("[PipelineChunker] from_page must be <= to_page")
        if not isinstance(self.parser_config, dict):
            raise ValueError("[PipelineChunker] parser_config must be a dict.")
        return True


class PipelineChunker(ComponentBase, ABC):
    """
    Run a Pipeline-style chunker (naive, paper, qa, manual, book, ...) against
    one or more uploaded files and surface the resulting chunks to downstream
    Agent nodes.
    """

    component_name = "PipelineChunker"

    def get_input_form(self) -> dict[str, dict]:
        """Expose each referenced file input as a file-typed form element."""
        res = {}
        for ref in self._param.inputs or []:
            for k, o in self.get_input_elements_from_text(ref).items():
                res[k] = {"name": o.get("name", ""), "type": "file"}
        return res

    def _get_file_content(self, file_ref: str) -> tuple[bytes | None, str | None]:
        """Resolve a canvas variable reference to ``(content_bytes, filename)``."""
        value = self._canvas.get_variable_value(file_ref)
        if value is None:
            return None, None

        if isinstance(value, list) and value:
            value = value[0]

        if isinstance(value, dict):
            file_id = value.get("id") or value.get("file_id")
            created_by = value.get("created_by") or self._canvas.get_tenant_id()
            filename = value.get("name") or value.get("filename") or "uploaded"
            if file_id:
                try:
                    return FileService.get_blob(created_by, file_id), filename
                except Exception as e:
                    logging.exception(
                        f"[PipelineChunker] FileService.get_blob failed for "
                        f"file_id={file_id} created_by={created_by} filename={filename}: {e}"
                    )
                    return None, None
        return None, None

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10 * 60)))
    def _invoke(self, **kwargs):
        """Run the configured chunker over every referenced file and publish outputs."""
        if self.check_if_canceled("PipelineChunker processing"):
            return

        chunker = _load_chunker(self._param.parser_id)
        tenant_id = self._canvas.get_tenant_id()
        chunk_kwargs = dict(
            lang=self._param.lang,
            tenant_id=tenant_id,
            from_page=self._param.from_page,
            to_page=self._param.to_page,
            parser_config=self._param.parser_config or {},
            callback=lambda prog=0, msg="": logging.info(f"[PipelineChunker] {prog}: {msg}"),
        )

        all_chunks: list[dict] = []
        per_file_counts: list[str] = []

        for file_ref in self._param.inputs or []:
            if self.check_if_canceled("PipelineChunker processing"):
                return

            content, filename = self._get_file_content(file_ref)
            self.set_input_value(file_ref, filename or "")
            if content is None:
                logging.warning(f"[PipelineChunker] could not resolve file ref: {file_ref}")
                per_file_counts.append(f"{filename or file_ref}: error (could not resolve file)")
                continue

            try:
                file_chunks = chunker(filename, binary=content, **chunk_kwargs) or []
            except Exception as e:
                logging.exception(e)
                per_file_counts.append(f"{filename}: error (chunking failed)")
                continue

            all_chunks.extend(file_chunks)
            per_file_counts.append(f"{filename}: {len(file_chunks)} chunks")

        text_only = [(c.get("content_with_weight") or c.get("text") or "") for c in all_chunks if isinstance(c, dict)]
        text_only = [t for t in text_only if t]

        self.set_output("chunks", text_only)
        self.set_output("chunks_full", all_chunks)
        self.set_output(
            "summary",
            f"Parser: {self._param.parser_id} | Files: {len(self._param.inputs or [])} | Chunks: {len(text_only)}" + (" | " + "; ".join(per_file_counts) if per_file_counts else ""),
        )

    def thoughts(self) -> str:
        """Return a short status line for UI display."""
        return f"Chunking with `{self._param.parser_id}` strategy..."
