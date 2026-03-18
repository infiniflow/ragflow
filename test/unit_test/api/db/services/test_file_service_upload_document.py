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
import importlib.util
import sys
import types
import warnings
from types import SimpleNamespace

import pytest

warnings.filterwarnings(
    "ignore",
    message="pkg_resources is deprecated as an API.*",
    category=UserWarning,
)


def _install_cv2_stub_if_unavailable():
    if importlib.util.find_spec("cv2") is not None:
        return

    stub = types.ModuleType("cv2")
    stub.INTER_LINEAR = 1
    stub.INTER_CUBIC = 2
    stub.BORDER_CONSTANT = 0
    stub.BORDER_REPLICATE = 1

    def _missing(*_args, **_kwargs):
        raise RuntimeError("cv2 runtime call is unavailable in this test environment")

    def _module_getattr(name):
        if name.isupper():
            return 0
        return _missing

    stub.__getattr__ = _module_getattr
    sys.modules["cv2"] = stub


def _install_xgboost_stub_if_unavailable():
    if "xgboost" in sys.modules:
        return
    sys.modules["xgboost"] = types.ModuleType("xgboost")


_install_cv2_stub_if_unavailable()
_install_xgboost_stub_if_unavailable()

from api.db.services import file_service as file_service_module  # noqa: E402
from api.db.services.file_service import FileService  # noqa: E402


class _DummyUploadFile:
    def __init__(self, filename, doc_id):
        self.filename = filename
        self.id = doc_id

    def read(self):
        raise AssertionError("read() should not be called for cross-KB collision path")


def _unwrapped_upload_document():
    return FileService.upload_document.__func__.__wrapped__


@pytest.mark.p2
def test_upload_document_skips_cross_kb_document_id_collision(monkeypatch):
    kb = SimpleNamespace(
        id="kb-target",
        tenant_id="tenant-1",
        name="Target KB",
        parser_id="default",
        pipeline_id=None,
        parser_config={},
    )
    existing_doc = SimpleNamespace(
        id="doc-1",
        kb_id="kb-other",
        location="old-location.txt",
        content_hash="old-hash",
        to_dict=lambda: {"id": "doc-1"},
    )

    monkeypatch.setattr(FileService, "get_root_folder", classmethod(lambda cls, _uid: {"id": "root"}))
    monkeypatch.setattr(FileService, "init_knowledgebase_docs", classmethod(lambda cls, _pf_id, _uid: None))
    monkeypatch.setattr(FileService, "get_kb_folder", classmethod(lambda cls, _uid: {"id": "kb-root"}))
    monkeypatch.setattr(
        FileService,
        "new_a_file_from_kb",
        classmethod(lambda cls, _tenant_id, _name, _parent_id: {"id": "kb-folder"}),
    )
    monkeypatch.setattr(file_service_module.DocumentService, "get_by_id", lambda _doc_id: (True, existing_doc))

    err, files = _unwrapped_upload_document()(
        FileService,
        kb,
        [_DummyUploadFile(filename="collision.txt", doc_id="doc-1")],
        "user-1",
    )

    assert files == []
    assert len(err) == 1
    assert err[0].startswith("collision.txt: ")
    assert "Existing document id collision with another knowledge base; skipping update." in err[0]
