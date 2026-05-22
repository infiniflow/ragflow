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
"""Unit tests for `_resolve_message_files` (api/apps/restful_apis/openai_api.py).

Covers issue #5637: the OpenAI-compatible chat API must accept inline file
attachments — `{"blob": "<base64>", "display_name": "a.txt"}` — inside a
message, persisting them so the existing chat pipeline can parse them.
"""

import base64
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _PassthroughManager:
    def route(self, *_args, **_kwargs):
        return lambda func: func


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


def _load_openai_api(monkeypatch):
    """Load api/apps/restful_apis/openai_api.py with minimal stubs.

    Returns `(module, put_blob_calls)` where `put_blob_calls` records every
    `FileService.put_blob(user_id, location, blob)` invocation.
    """
    put_blob_calls = []

    def _put_blob(user_id, location, blob):
        put_blob_calls.append((user_id, location, blob))

    uuid_counter = {"n": 0}

    def _get_uuid():
        uuid_counter["n"] += 1
        return f"file-uuid-{uuid_counter['n']}"

    _stub(monkeypatch, "api.apps", current_user=SimpleNamespace(id="tenant-1"), login_required=lambda func: func)
    _stub(monkeypatch, "api.db.services.dialog_service", DialogService=SimpleNamespace(), async_chat=lambda *_a, **_k: None)
    _stub(monkeypatch, "api.db.services.doc_metadata_service", DocMetadataService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.file_service", FileService=SimpleNamespace(put_blob=_put_blob))
    _stub(monkeypatch, "api.db.services.tenant_llm_service", TenantLLMService=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        get_error_data_result=lambda message="", code=None: {"code": code or 102, "message": message, "data": None},
        get_request_json=lambda: {},
        validate_request=lambda *_a, **_k: lambda func: func,
    )
    _stub(monkeypatch, "api.utils.reference_metadata_utils", enrich_chunks_with_document_metadata=lambda *_a, **_k: None)
    _stub(monkeypatch, "common.constants", RetCode=SimpleNamespace(ARGUMENT_ERROR=101), StatusEnum=SimpleNamespace(VALID=SimpleNamespace(value="1")))
    _stub(monkeypatch, "common.metadata_utils", convert_conditions=lambda *_a, **_k: None, meta_filter=lambda *_a, **_k: [])
    _stub(monkeypatch, "common.misc_utils", get_uuid=_get_uuid)
    _stub(monkeypatch, "common.token_utils", num_tokens_from_string=lambda _s: 0)
    _stub(monkeypatch, "rag.prompts.generator", chunks_format=lambda _r: [])

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "restful_apis" / "openai_api.py"
    spec = importlib.util.spec_from_file_location("test_openai_message_files_openai_api", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, "test_openai_message_files_openai_api", module)
    spec.loader.exec_module(module)
    return module, put_blob_calls


@pytest.mark.p2
class TestResolveMessageFiles:
    """Issue #5637: inline `files` entries on an OpenAI-compatible message."""

    def test_inline_blob_is_decoded_and_stored(self, monkeypatch):
        """A `{blob, display_name}` entry is decoded, persisted, and rewritten
        into the `{id, created_by, mime_type, name}` form the pipeline expects."""
        module, put_blob_calls = _load_openai_api(monkeypatch)
        content = b"hello world"
        files = [{"blob": base64.b64encode(content).decode(), "display_name": "a.txt"}]

        resolved = module._resolve_message_files(files)

        assert resolved == [
            {
                "id": "file-uuid-1",
                "created_by": "tenant-1",
                "name": "a.txt",
                "mime_type": "text/plain",
            }
        ]
        assert put_blob_calls == [("tenant-1", "file-uuid-1", content)]

    def test_data_uri_prefix_sets_mime(self, monkeypatch):
        """A `data:<mime>;base64,` prefix is stripped and supplies the mime type."""
        module, put_blob_calls = _load_openai_api(monkeypatch)
        content = b"\x89PNG fake-bytes"
        blob = "data:image/png;base64," + base64.b64encode(content).decode()

        resolved = module._resolve_message_files([{"blob": blob, "display_name": "pic"}])

        assert resolved[0]["mime_type"] == "image/png"
        assert put_blob_calls[0][2] == content

    def test_mime_falls_back_to_extension(self, monkeypatch):
        """Without a data-URI prefix the mime type is guessed from the name."""
        module, _ = _load_openai_api(monkeypatch)
        blob = base64.b64encode(b"data").decode()

        resolved = module._resolve_message_files([{"blob": blob, "display_name": "report.pdf"}])

        assert resolved[0]["mime_type"] == "application/pdf"

    def test_reference_entry_passes_through(self, monkeypatch):
        """An entry that already references a stored file is left untouched."""
        module, put_blob_calls = _load_openai_api(monkeypatch)
        ref = {"id": "doc-9", "created_by": "tenant-1", "mime_type": "text/plain", "name": "x.txt"}

        resolved = module._resolve_message_files([ref])

        assert resolved == [ref]
        assert put_blob_calls == []

    def test_missing_display_name_raises(self, monkeypatch):
        module, _ = _load_openai_api(monkeypatch)
        with pytest.raises(ValueError, match="display_name is required"):
            module._resolve_message_files([{"blob": base64.b64encode(b"x").decode()}])

    def test_invalid_base64_raises(self, monkeypatch):
        module, _ = _load_openai_api(monkeypatch)
        with pytest.raises(ValueError, match="not valid base64"):
            module._resolve_message_files([{"blob": "not!base64!", "display_name": "a.txt"}])

    def test_entry_without_blob_or_id_raises(self, monkeypatch):
        module, _ = _load_openai_api(monkeypatch)
        with pytest.raises(ValueError, match="either 'blob' or 'id'"):
            module._resolve_message_files([{"display_name": "a.txt"}])

    def test_non_dict_entry_raises(self, monkeypatch):
        module, _ = _load_openai_api(monkeypatch)
        with pytest.raises(ValueError, match="must be an object"):
            module._resolve_message_files(["just-a-string"])
