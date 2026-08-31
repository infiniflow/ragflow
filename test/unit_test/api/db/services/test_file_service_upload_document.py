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
import socket
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
    try:
        importlib.import_module("cv2")
        return
    except Exception:
        pass

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
    if importlib.util.find_spec("xgboost") is not None:
        return
    sys.modules["xgboost"] = types.ModuleType("xgboost")


_install_cv2_stub_if_unavailable()
_install_xgboost_stub_if_unavailable()

from api.db.services import file_service as file_service_module  # noqa: E402
from api.db.services.file_service import FileService  # noqa: E402


class _DummyUploadFile:
    def __init__(self, filename, doc_id, blob=None):
        self.filename = filename
        self.id = doc_id
        self._blob = blob

    def read(self):
        if self._blob is None:
            raise AssertionError("read() should not be called for cross-KB collision path")
        return self._blob


class _DummyStorage:
    """Storage double recording writes without touching MinIO."""

    def __init__(self):
        self.written = {}
        self.removed = []

    def obj_exist(self, _bucket, _location):
        return False

    def put(self, bucket, location, blob, *_args):
        self.written[(bucket, location)] = blob

    def rm(self, bucket, location):
        self.removed.append((bucket, location))


def _unwrapped_upload_document():
    return FileService.upload_document.__func__.__wrapped__


def _target_kb():
    return SimpleNamespace(
        id="kb-target",
        tenant_id="tenant-1",
        name="Target KB",
        parser_id="default",
        pipeline_id=None,
        parser_config={},
    )


def _stub_folder_lookups(monkeypatch):
    monkeypatch.setattr(FileService, "get_root_folder", classmethod(lambda cls, _uid: {"id": "root"}))
    monkeypatch.setattr(FileService, "init_knowledgebase_docs", classmethod(lambda cls, _pf_id, _uid: None))
    monkeypatch.setattr(FileService, "get_kb_folder", classmethod(lambda cls, _uid: {"id": "kb-root"}))
    monkeypatch.setattr(
        FileService,
        "new_a_file_from_kb",
        classmethod(lambda cls, _tenant_id, _name, _parent_id: {"id": "kb-folder"}),
    )


@pytest.mark.p2
def test_upload_document_skips_cross_kb_document_id_collision(monkeypatch):
    kb = _target_kb()
    existing_doc = SimpleNamespace(
        id="doc-1",
        kb_id="kb-other",
        location="old-location.txt",
        content_hash="old-hash",
        to_dict=lambda: {"id": "doc-1"},
    )

    _stub_folder_lookups(monkeypatch)
    monkeypatch.setattr(file_service_module.DocumentService, "get_by_id", lambda _doc_id: (True, existing_doc))
    monkeypatch.setattr(
        file_service_module.KnowledgebaseService,
        "get_or_none",
        classmethod(lambda cls, **_kwargs: SimpleNamespace(id="kb-other")),
    )

    err, files = _unwrapped_upload_document()(
        FileService,
        kb,
        [_DummyUploadFile(filename="collision.txt", doc_id="doc-1")],
        "user-1",
    )

    assert files == []
    assert len(err) == 1
    assert err[0].startswith("collision.txt: ")
    # The owning knowledge base is named so the conflict can actually be found.
    assert "Existing document id collision with knowledge base 'kb-other'; skipping update." in err[0]


@pytest.mark.p2
def test_upload_document_reclaims_document_stranded_by_deleted_kb(monkeypatch):
    """A row whose knowledge base is gone must not block ingestion forever.

    Regression test for #18116: the stranded row is unreachable through the UI
    and the API, so treating it as a live conflict permanently blocked the
    document from syncing anywhere.
    """
    kb = _target_kb()
    stranded_doc = SimpleNamespace(
        id="doc-1",
        kb_id="kb-deleted",
        location="old-location.txt",
        content_hash="old-hash",
        to_dict=lambda: {"id": "doc-1"},
    )
    storage = _DummyStorage()
    discarded = []
    inserted = []
    tasks_deleted = []
    files_deleted = []
    mappings_deleted = []

    _stub_folder_lookups(monkeypatch)
    monkeypatch.setattr(file_service_module.DocumentService, "get_by_id", lambda _doc_id: (True, stranded_doc))
    monkeypatch.setattr(
        file_service_module.KnowledgebaseService,
        "get_or_none",
        classmethod(lambda cls, **kwargs: None if kwargs.get("id") == "kb-deleted" else SimpleNamespace(id=kwargs.get("id"))),
    )
    monkeypatch.setattr(
        file_service_module.DocumentService,
        "delete_by_id",
        classmethod(lambda cls, doc_id: discarded.append(doc_id)),
    )
    monkeypatch.setattr(
        file_service_module.File2DocumentService,
        "delete_by_document_id",
        classmethod(lambda cls, doc_id: mappings_deleted.append(doc_id)),
    )
    monkeypatch.setattr(
        file_service_module.File2DocumentService,
        "get_storage_address",
        classmethod(lambda cls, **_kwargs: ("kb-deleted", "old-location.txt")),
    )
    monkeypatch.setattr(
        file_service_module.File2DocumentService,
        "get_by_document_id",
        classmethod(lambda cls, _doc_id: [SimpleNamespace(file_id="file-1")]),
    )
    monkeypatch.setattr(
        file_service_module.TaskService,
        "filter_delete",
        classmethod(lambda cls, _filters: tasks_deleted.append("doc-1")),
    )
    monkeypatch.setattr(
        FileService,
        "filter_delete",
        classmethod(lambda cls, _filters: files_deleted.append("file-1") or 1),
    )
    monkeypatch.setattr(file_service_module.DocumentService, "check_doc_health", classmethod(lambda cls, _tid, _name: None))
    monkeypatch.setattr(file_service_module.DocumentService, "query", classmethod(lambda cls, **_kwargs: []))
    monkeypatch.setattr(file_service_module.DocumentService, "insert", classmethod(lambda cls, doc: inserted.append(doc)))
    monkeypatch.setattr(FileService, "add_file_from_kb", classmethod(lambda cls, _doc, _folder_id, _tenant_id: None))
    monkeypatch.setattr(file_service_module, "thumbnail_img", lambda _name, _blob: None)
    monkeypatch.setattr(file_service_module.settings, "STORAGE_IMPL", storage)

    err, files = _unwrapped_upload_document()(
        FileService,
        kb,
        [_DummyUploadFile(filename="collision.txt", doc_id="doc-1", blob=b"payload")],
        "user-1",
    )

    assert err == []
    assert discarded == ["doc-1"]
    # The stranded row's debris goes with it, matching what delete_docs would
    # have removed had the dataset deletion reached this document.
    assert tasks_deleted == ["doc-1"]
    assert files_deleted == ["file-1"]
    assert mappings_deleted == ["doc-1"]
    assert storage.removed == [("kb-deleted", "old-location.txt")]
    assert len(inserted) == 1
    assert inserted[0]["id"] == "doc-1"
    assert inserted[0]["kb_id"] == "kb-target"
    assert len(files) == 1
    assert files[0][1] == b"payload"
    assert storage.written[("kb-target", "collision.txt")] == b"payload"


# ---------------------------------------------------------------------------
# Helpers shared by TestValidateUrlForCrawl
# ---------------------------------------------------------------------------


def _addrinfo(ip_str: str) -> list:
    """Build a minimal getaddrinfo-style result for a single address string."""
    family = socket.AF_INET6 if ":" in ip_str else socket.AF_INET
    return [(family, socket.SOCK_STREAM, 6, "", (ip_str, 0))]


# ---------------------------------------------------------------------------
# _validate_url_for_crawl SSRF-guard tests
# ---------------------------------------------------------------------------


@pytest.mark.p2
class TestValidateUrlForCrawl:
    """Focused regression suite for the SSRF guard on the URL-crawl path.

    All DNS lookups are monkeypatched so the tests are deterministic and
    require no network access.
    """

    # -- scheme checks -------------------------------------------------------

    def test_rejects_ftp_scheme(self):
        with pytest.raises(ValueError, match="scheme"):
            FileService._validate_url_for_crawl("ftp://example.com/file.txt")

    def test_rejects_file_scheme(self):
        with pytest.raises(ValueError, match="scheme"):
            FileService._validate_url_for_crawl("file:///etc/passwd")

    def test_rejects_javascript_scheme(self):
        with pytest.raises(ValueError, match="scheme"):
            FileService._validate_url_for_crawl("javascript:alert(1)")

    # -- host checks ---------------------------------------------------------

    def test_rejects_missing_host(self):
        with pytest.raises(ValueError, match="host"):
            FileService._validate_url_for_crawl("http:///path")

    def test_rejects_dns_resolution_failure(self, monkeypatch):
        def _raise(h, p):
            raise socket.gaierror("NXDOMAIN")

        monkeypatch.setattr(socket, "getaddrinfo", _raise)
        with pytest.raises(ValueError, match="Could not resolve"):
            FileService._validate_url_for_crawl("http://nxdomain.invalid/")

    # -- blocked address families --------------------------------------------

    def test_rejects_loopback_ipv4(self, monkeypatch):
        monkeypatch.setattr(socket, "getaddrinfo", lambda h, p: _addrinfo("127.0.0.1"))
        with pytest.raises(ValueError, match="non-public"):
            FileService._validate_url_for_crawl("http://localhost/")

    def test_rejects_private_class_a(self, monkeypatch):
        monkeypatch.setattr(socket, "getaddrinfo", lambda h, p: _addrinfo("10.0.0.1"))
        with pytest.raises(ValueError, match="non-public"):
            FileService._validate_url_for_crawl("http://internal.example/")

    def test_rejects_private_class_b(self, monkeypatch):
        monkeypatch.setattr(socket, "getaddrinfo", lambda h, p: _addrinfo("172.16.0.1"))
        with pytest.raises(ValueError, match="non-public"):
            FileService._validate_url_for_crawl("http://internal.example/")

    def test_rejects_private_class_c(self, monkeypatch):
        monkeypatch.setattr(socket, "getaddrinfo", lambda h, p: _addrinfo("192.168.1.100"))
        with pytest.raises(ValueError, match="non-public"):
            FileService._validate_url_for_crawl("http://internal.example/")

    def test_rejects_link_local_ipv4(self, monkeypatch):
        monkeypatch.setattr(socket, "getaddrinfo", lambda h, p: _addrinfo("169.254.0.1"))
        with pytest.raises(ValueError, match="non-public"):
            FileService._validate_url_for_crawl("http://link-local.example/")

    def test_rejects_reserved_ipv4(self, monkeypatch):
        # 240.0.0.0/4 is IANA reserved — not globally routable
        monkeypatch.setattr(socket, "getaddrinfo", lambda h, p: _addrinfo("240.0.0.1"))
        with pytest.raises(ValueError, match="non-public"):
            FileService._validate_url_for_crawl("http://reserved.example/")

    def test_rejects_ipv4_mapped_loopback(self, monkeypatch):
        """::ffff:127.0.0.1 must not bypass the loopback check."""
        monkeypatch.setattr(socket, "getaddrinfo", lambda h, p: _addrinfo("::ffff:127.0.0.1"))
        with pytest.raises(ValueError, match="non-public"):
            FileService._validate_url_for_crawl("http://mapped-loopback.example/")

    def test_rejects_ipv4_mapped_private(self, monkeypatch):
        """::ffff:192.168.1.1 must not bypass the private-range check."""
        monkeypatch.setattr(socket, "getaddrinfo", lambda h, p: _addrinfo("::ffff:192.168.1.1"))
        with pytest.raises(ValueError, match="non-public"):
            FileService._validate_url_for_crawl("http://mapped-private.example/")

    def test_rejects_when_any_record_is_private(self, monkeypatch):
        """All DNS records must pass; one private record is enough to block."""
        monkeypatch.setattr(
            socket,
            "getaddrinfo",
            lambda h, p: _addrinfo("93.184.216.34") + _addrinfo("10.0.0.1"),
        )
        with pytest.raises(ValueError, match="non-public"):
            FileService._validate_url_for_crawl("http://mixed.example/")

    # -- allowed cases -------------------------------------------------------

    def test_allows_public_ipv4(self, monkeypatch):
        monkeypatch.setattr(socket, "getaddrinfo", lambda h, p: _addrinfo("93.184.216.34"))
        hostname, resolved_ip = FileService._validate_url_for_crawl("https://example.com/doc.pdf")
        assert hostname == "example.com"
        assert resolved_ip == "93.184.216.34"

    def test_allows_public_ipv6(self, monkeypatch):
        monkeypatch.setattr(
            socket,
            "getaddrinfo",
            lambda h, p: _addrinfo("2606:2800:220:1:248:1893:25c8:1946"),
        )
        hostname, resolved_ip = FileService._validate_url_for_crawl("https://example.com/")
        assert hostname == "example.com"
        assert resolved_ip == "2606:2800:220:1:248:1893:25c8:1946"

    def test_allows_http_scheme(self, monkeypatch):
        monkeypatch.setattr(socket, "getaddrinfo", lambda h, p: _addrinfo("1.2.3.4"))
        hostname, _ = FileService._validate_url_for_crawl("http://example.com/")
        assert hostname == "example.com"

    # -- multi-record behaviour ----------------------------------------------

    def test_returns_first_ip_for_multi_record_host(self, monkeypatch):
        """The first public IP is returned as the DNS pin value."""
        monkeypatch.setattr(
            socket,
            "getaddrinfo",
            lambda h, p: _addrinfo("1.2.3.4") + _addrinfo("5.6.7.8"),
        )
        _, resolved_ip = FileService._validate_url_for_crawl("http://multi.example/")
        assert resolved_ip == "1.2.3.4"

    def test_allows_dual_stack_host(self, monkeypatch):
        """A host with both public IPv4 and public IPv6 records is allowed."""
        monkeypatch.setattr(
            socket,
            "getaddrinfo",
            lambda h, p: _addrinfo("93.184.216.34") + _addrinfo("2606:2800:220:1:248:1893:25c8:1946"),
        )
        hostname, resolved_ip = FileService._validate_url_for_crawl("https://example.com/")
        assert hostname == "example.com"
        assert resolved_ip == "93.184.216.34"
