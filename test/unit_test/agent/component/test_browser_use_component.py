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

import re
import sys
import types
from pathlib import Path
from types import SimpleNamespace
from urllib.parse import urlparse

from api.db import FileType


def _install_cv2_stub_if_unavailable():
    try:
        import cv2  # noqa: F401

        return
    except Exception:
        pass
    stub = types.ModuleType("cv2")
    stub.INTER_LINEAR = 1
    stub.INTER_CUBIC = 2
    stub.BORDER_CONSTANT = 0
    stub.BORDER_REPLICATE = 1

    def _module_getattr(name):
        if name.isupper():
            return 0
        raise RuntimeError("cv2 runtime call is unavailable in this test environment")

    stub.__getattr__ = _module_getattr
    sys.modules["cv2"] = stub


_install_cv2_stub_if_unavailable()


def _install_xgboost_stub_if_unavailable():
    # deepdoc/parser/pdf_parser.py imports xgboost unconditionally. Its macOS
    # wheels need libomp, which slim test environments may not have; none of
    # these tests touch the layout model, so a stub keeps them collectable.
    try:
        import xgboost  # noqa: F401

        return
    except Exception:
        pass
    stub = types.ModuleType("xgboost")

    def _module_getattr(name):
        raise RuntimeError(f"xgboost runtime call ({name}) is unavailable in this test environment")

    stub.__getattr__ = _module_getattr
    sys.modules["xgboost"] = stub


_install_xgboost_stub_if_unavailable()


from agent.component import browser as browser_use_module  # noqa: E402


class _FakeCanvas:
    def __init__(self, refs=None):
        self._refs = refs or {}

    def is_reff(self, token):
        key = token.strip("{} ")
        return key in self._refs or token in self._refs

    def get_variable_value(self, token):
        key = token.strip("{} ")
        if key in self._refs:
            return self._refs[key]
        return self._refs[token]

    def get_tenant_id(self):
        return "tenant-1"


def _build_component():
    component = browser_use_module.Browser.__new__(browser_use_module.Browser)
    component._canvas = _FakeCanvas()
    component._param = SimpleNamespace(upload_sources="")
    return component


def test_prepare_input_values_records_variable_inputs():
    component = browser_use_module.Browser.__new__(browser_use_module.Browser)
    component._canvas = _FakeCanvas(refs={"sys.query": "open example.com"})
    component._param = browser_use_module.BrowserParam()
    component._param.prompts = "{sys.query}"
    component._param.inputs = {}

    component._prepare_input_values()

    assert component.get_input_value("sys.query") == "open example.com"
    assert component.get_input_values()["sys.query"] == "open example.com"


def test_extract_ids_supports_mixed_literals_and_variables():
    component = browser_use_module.Browser.__new__(browser_use_module.Browser)
    component._canvas = _FakeCanvas(
        refs={
            "{begin@file_ids}": ["f2", "f3,f4"],
            "{begin@extra_file}": "f5",
        }
    )

    file_ids = component._extract_ids("f1,{begin@file_ids},{begin@extra_file},f1")

    assert file_ids == ["f1", "f2", "f3", "f4", "f5"]


def test_extract_ids_supports_json_array_and_csv_format():
    component = browser_use_module.Browser.__new__(browser_use_module.Browser)
    component._canvas = _FakeCanvas()

    json_ids = component._extract_ids('["1","2"]')
    csv_ids = component._extract_ids("1,2")

    assert json_ids == ["1", "2"]
    assert csv_ids == ["1", "2"]


def test_extract_ids_supports_variable_values_from_input_or_globals():
    component = browser_use_module.Browser.__new__(browser_use_module.Browser)
    component._canvas = _FakeCanvas(
        refs={
            "{begin@upload_ids}": '["10","20"]',
            "{sys@upload_id}": 30,
            "{begin@file_obj}": {"id": "40", "name": "demo.pdf"},
        }
    )

    file_ids = component._extract_ids("{begin@upload_ids},{sys@upload_id},{begin@file_obj}")

    assert file_ids == ["10", "20", "30", "40"]


def test_extract_ids_supports_url_key_in_variable_object():
    component = browser_use_module.Browser.__new__(browser_use_module.Browser)
    component._canvas = _FakeCanvas(
        refs={
            "{begin@upload_url_obj}": {"url": "https://example.com/demo.pdf"},
        }
    )

    refs = component._extract_ids("{begin@upload_url_obj}")

    assert refs == ["https://example.com/demo.pdf"]


def test_extract_ids_does_not_split_http_url_by_comma():
    component = browser_use_module.Browser.__new__(browser_use_module.Browser)
    component._canvas = _FakeCanvas()

    refs = component._extract_ids("https://example.com/download?name=a,b.txt")

    assert refs == ["https://example.com/download?name=a,b.txt"]


class _FakeRequestsResponse:
    def __init__(self, status_code=200, headers=None, data=b""):
        self.status_code = status_code
        self.headers = dict(headers or {})
        self._data = data
        self.closed = False

    def iter_content(self, chunk_size=1024 * 1024):
        for i in range(0, len(self._data), max(chunk_size, 1)):
            yield self._data[i : i + chunk_size]

    def raise_for_status(self):
        if self.status_code >= 400:
            raise AssertionError(f"unexpected HTTP status: {self.status_code}")

    def close(self):
        self.closed = True


class _FakeRequestsSession:
    """Stands in for requests.Session: records calls and lifecycle state."""

    def __init__(self, handler):
        # Production must set trust_env = False right after construction.
        self.trust_env = True
        self.closed = False
        self.calls = []
        self._handler = handler

    def get(self, url, **kwargs):
        self.calls.append(url)
        return self._handler(url, **kwargs)

    def close(self):
        self.closed = True


def _patch_requests_session(monkeypatch, handler):
    """Redirect requests.Session() to a recording fake; returns created sessions."""
    import requests

    created = []

    def _factory():
        session = _FakeRequestsSession(handler)
        created.append(session)
        return session

    monkeypatch.setattr(requests, "Session", _factory)
    return created


def _record_dns_pins(monkeypatch):
    """Record pin_dns(host, ip) activations; exposes the active stack."""
    import common.ssrf_guard as ssrf

    active = []
    recorded = []

    class _pin:
        def __init__(self, host, ip):
            self._host, self._ip = host, ip

        def __enter__(self):
            active.append((self._host, self._ip))
            recorded.append((self._host, self._ip))

        def __exit__(self, *_exc):
            active.pop()

    monkeypatch.setattr(ssrf, "pin_dns", _pin)
    return recorded, active


def _allow_public_hosts(monkeypatch, allowed_substrings=("example.com", "example.net")):
    import common.ssrf_guard as ssrf

    def _fake_assert(url):
        host = urlparse(url).hostname or ""
        for marker in allowed_substrings:
            if marker in host:
                return (host, "93.184.216.34")
        raise ValueError(f"blocked in test: {url}")

    monkeypatch.setattr(ssrf, "assert_url_is_safe", _fake_assert)


def test_prepare_upload_files_supports_http_url(monkeypatch, tmp_path):
    component = _build_component()
    component._param.upload_sources = "https://example.com/files/demo.txt"

    _allow_public_hosts(monkeypatch)
    _patch_requests_session(
        monkeypatch,
        lambda _url, **_kwargs: _FakeRequestsResponse(
            headers={"Content-Disposition": 'attachment; filename="remote_demo.txt"'},
            data=b"hello from url",
        ),
    )

    prepared = component._prepare_upload_files(str(tmp_path))

    assert len(prepared) == 1
    assert prepared[0]["file_id"] == ""
    assert prepared[0]["name"] == "remote_demo.txt"
    assert prepared[0]["source_url"] == "https://example.com/files/demo.txt"
    assert Path(prepared[0]["local_path"]).exists()
    assert Path(prepared[0]["local_path"]).read_bytes() == b"hello from url"


def test_extract_url_filename_decodes_percent_escaped_traversal():
    # Regression (CWE-22): "%2e%2e%2f" is not a separator for os.path.basename()
    # before decoding, so basename-then-unquote left "../" sequences in the
    # name and the joined path escaped the upload directory.
    name = browser_use_module.Browser._extract_url_filename(
        "https://example.com/download/%2e%2e%2f..%2f..%2fetc%2fpasswd",
        headers={},
    )

    assert name == "passwd"


def test_extract_url_filename_sanitizes_content_disposition_traversal():
    headers = {"Content-Disposition": "attachment; filename*=UTF-8''..%2f..%2fowned.txt"}

    name = browser_use_module.Browser._extract_url_filename("https://example.com/dl", headers=headers)

    assert name == "owned.txt"


def test_extract_url_filename_falls_back_to_uuid_when_no_safe_name():
    # The whole path decodes to a traversal segment, so no safe name remains.
    name = browser_use_module.Browser._extract_url_filename("https://example.com/%2e%2e%2f", headers={})

    assert re.fullmatch(r"url_file_[0-9a-f]{8}\.bin", name)


def test_prepare_upload_url_file_keeps_percent_traversal_inside_upload_dir(monkeypatch, tmp_path):
    component = _build_component()

    _allow_public_hosts(monkeypatch)
    _patch_requests_session(monkeypatch, lambda _url, **_kwargs: _FakeRequestsResponse(data=b"payload"))

    prepared = component._prepare_upload_url_file("https://example.com/files/%2e%2e%2f..%2fvictim.txt", str(tmp_path))

    assert prepared is not None
    local_path = Path(prepared["local_path"])
    assert local_path.parent == tmp_path
    assert local_path.name == "victim.txt"
    assert local_path.read_bytes() == b"payload"


def test_prepare_upload_url_file_rejects_private_and_metadata_urls(monkeypatch, tmp_path):
    # Uses the real assert_url_is_safe (no monkeypatch): loopback, link-local
    # metadata and RFC1918 hosts must be refused before any connection.
    component = _build_component()
    monkeypatch.delenv("ALLOW_ANY_HOST", raising=False)

    connections = []
    sessions = _patch_requests_session(monkeypatch, lambda url, **_kwargs: connections.append(url))

    for url in (
        "http://127.0.0.1:8080/admin",
        "http://169.254.169.254/latest/meta-data/",
        "http://10.1.2.3/internal.txt",
        "http://[::1]/x.txt",
    ):
        assert component._prepare_upload_url_file(url, str(tmp_path)) is None

    assert connections == []
    assert list(tmp_path.iterdir()) == []
    # The isolated session is still constructed and released on each attempt.
    assert all(session.closed for session in sessions)


def test_prepare_upload_url_file_rejects_redirect_to_intranet(monkeypatch, tmp_path):
    # First hop is a public URL answering 302 -> cloud metadata: the redirect
    # target must be re-validated and rejected before connecting to it.
    component = _build_component()

    _allow_public_hosts(monkeypatch)
    connections = []

    def _redirect_to_metadata(url, **_kwargs):
        connections.append(url)
        return _FakeRequestsResponse(
            status_code=302,
            headers={"Location": "http://169.254.169.254/latest/meta-data/iam/security-credentials/"},
        )

    sessions = _patch_requests_session(monkeypatch, _redirect_to_metadata)

    prepared = component._prepare_upload_url_file("https://example.com/file.bin", str(tmp_path))

    assert prepared is None
    assert connections == ["https://example.com/file.bin"]
    assert list(tmp_path.iterdir()) == []
    # The rejected fetch still releases its isolated session.
    assert sessions[0].closed is True


def test_prepare_upload_url_file_follows_safe_redirects(monkeypatch, tmp_path):
    component = _build_component()

    _allow_public_hosts(monkeypatch)
    responses = []

    def _get(url, **_kwargs):
        if url == "https://example.com/download":
            response = _FakeRequestsResponse(status_code=301, headers={"Location": "https://cdn.example.net/final/report.pdf"})
        elif url == "https://cdn.example.net/final/report.pdf":
            response = _FakeRequestsResponse(data=b"PDF")
        else:
            raise AssertionError(f"unexpected url: {url}")
        responses.append(response)
        return response

    sessions = _patch_requests_session(monkeypatch, _get)

    prepared = component._prepare_upload_url_file("https://example.com/download", str(tmp_path))

    assert prepared is not None
    assert prepared["name"] == "report.pdf"
    assert prepared["source_url"] == "https://example.com/download"
    assert Path(prepared["local_path"]).read_bytes() == b"PDF"
    # The isolated session ignores ambient proxy/netrc config and is closed,
    # and every hop's response (including intermediate redirects) is closed.
    assert sessions[0].trust_env is False
    assert sessions[0].closed is True
    assert [r.closed for r in responses] == [True, True]


def test_prepare_upload_url_file_redirect_limit_boundary(monkeypatch, tmp_path):
    # MAX_UPLOAD_URL_REDIRECTS is 5: five hops to the final 200 are accepted,
    # a chain that needs a sixth redirect is refused.
    component = _build_component()
    _allow_public_hosts(monkeypatch)

    def _chain(hop_count):
        def _get(url, **_kwargs):
            hop = int(url.rsplit("/", 1)[-1])
            if hop < hop_count:
                return _FakeRequestsResponse(status_code=302, headers={"Location": f"https://example.com/hop/{hop + 1}"})
            return _FakeRequestsResponse(data=b"ok")

        return _get

    _patch_requests_session(monkeypatch, _chain(5))
    prepared = component._prepare_upload_url_file("https://example.com/hop/0", str(tmp_path))
    assert prepared is not None
    assert Path(prepared["local_path"]).read_bytes() == b"ok"

    _patch_requests_session(monkeypatch, _chain(6))
    prepared = component._prepare_upload_url_file("https://example.com/hop/0", str(tmp_path / "sub"))
    assert prepared is None


def test_prepare_upload_url_file_cleans_partial_file_on_size_limit(monkeypatch, tmp_path):
    component = _build_component()
    monkeypatch.setattr(browser_use_module.Browser, "_resolve_upload_url_max_bytes", lambda _self: 4)
    _allow_public_hosts(monkeypatch)
    _patch_requests_session(monkeypatch, lambda _url, **_kwargs: _FakeRequestsResponse(data=b"0123456789"))

    prepared = component._prepare_upload_url_file("https://example.com/big.bin", str(tmp_path))

    assert prepared is None
    assert list(tmp_path.iterdir()) == []


def test_prepare_upload_url_file_cleans_partial_file_when_stream_fails(monkeypatch, tmp_path):
    import requests

    component = _build_component()
    _allow_public_hosts(monkeypatch)

    class _ExplodingResponse(_FakeRequestsResponse):
        def iter_content(self, chunk_size=1024 * 1024):
            yield b"partial-"
            raise requests.RequestException("connection reset mid-stream")

    _patch_requests_session(monkeypatch, lambda _url, **_kwargs: _ExplodingResponse(data=b"partial-"))

    prepared = component._prepare_upload_url_file("https://example.com/dies.bin", str(tmp_path))

    assert prepared is None
    assert list(tmp_path.iterdir()) == []


def test_prepare_upload_url_file_pins_dns_for_every_hop(monkeypatch, tmp_path):
    component = _build_component()

    import common.ssrf_guard as ssrf

    def _assert(url):
        host = urlparse(url).hostname or ""
        return (host, {"example.com": "93.184.216.34", "cdn.example.net": "198.51.100.7"}[host])

    monkeypatch.setattr(ssrf, "assert_url_is_safe", _assert)
    recorded, active = _record_dns_pins(monkeypatch)

    def _get(url, **_kwargs):
        if url == "https://example.com/download":
            assert active[-1] == ("example.com", "93.184.216.34"), active
            return _FakeRequestsResponse(status_code=302, headers={"Location": "https://cdn.example.net/report.pdf"})
        assert active[-1] == ("cdn.example.net", "198.51.100.7"), active
        return _FakeRequestsResponse(data=b"ok")

    _patch_requests_session(monkeypatch, _get)

    prepared = component._prepare_upload_url_file("https://example.com/download", str(tmp_path))

    assert prepared is not None
    assert recorded == [
        ("example.com", "93.184.216.34"),
        ("cdn.example.net", "198.51.100.7"),
    ]
    # No pin leaks past the fetch: the stack unwinds to empty.
    assert active == []


def test_extract_url_filename_sanitizes_all_content_disposition_forms():
    cases = [
        ("attachment; filename*=UTF-8''..%2f..%2fstar.txt", "star.txt"),
        ('attachment; filename="..\\..\\quoted.txt"', "quoted.txt"),
        ("attachment; filename=../../unquoted.txt", "unquoted.txt"),
        ('attachment; filename="..%2F..%2Fmixed.txt"', "mixed.txt"),
    ]
    for header, expected in cases:
        name = browser_use_module.Browser._extract_url_filename("https://example.com/dl", headers={"Content-Disposition": header})
        assert name == expected, (header, name)


def test_save_downloads_persists_file_records(monkeypatch, tmp_path):
    component = _build_component()
    component._canvas = _FakeCanvas()

    download_file = tmp_path / "report.txt"
    download_file.write_text("ok", encoding="utf-8")

    monkeypatch.setattr(
        browser_use_module.FileService,
        "get_by_id",
        lambda _folder_id: (True, SimpleNamespace(type=FileType.FOLDER.value)),
    )
    monkeypatch.setattr(browser_use_module, "duplicate_name", lambda *_args, **_kwargs: "report.txt")

    stored = {}

    def _put(parent_id, location, blob):
        stored["parent_id"] = parent_id
        stored["location"] = location
        stored["blob"] = blob

    monkeypatch.setattr(browser_use_module.settings, "STORAGE_IMPL", SimpleNamespace(put=_put))
    monkeypatch.setattr(
        browser_use_module.FileService,
        "insert",
        lambda data: SimpleNamespace(
            id="file-1",
            name=data["name"],
            size=data["size"],
            parent_id=data["parent_id"],
        ),
    )

    result = component._save_downloads(str(tmp_path), "dir-1")

    assert len(result) == 1
    assert result[0]["file_id"] == "file-1"
    assert result[0]["parent_id"] == "dir-1"
    assert stored["parent_id"] == "dir-1"
    assert stored["location"] == "report.txt"
    assert stored["blob"] == b"ok"
    assert Path(download_file).exists()
