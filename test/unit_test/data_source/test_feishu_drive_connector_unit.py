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

import warnings
from types import SimpleNamespace
from unittest.mock import patch

import pytest

from common.data_source.config import DocumentSource
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    InsufficientPermissionsError,
)

# lark_oapi emits a pkg_resources UserWarning at import time; scope the
# suppression to just this import so the repo-wide ``filterwarnings = error``
# pytest setting does not fail collection (and no warnings are hidden elsewhere).
with warnings.catch_warnings():
    warnings.filterwarnings("ignore", message=r"pkg_resources is deprecated.*")
    import lark_oapi as lark  # noqa: E402
    from common.data_source.feishu_drive_connector import FeishuDriveConnector  # noqa: E402

pytestmark = [pytest.mark.p2]


# ---------------------------------------------------------------------------
# Fake Feishu/Lark Drive client
# ---------------------------------------------------------------------------

T_OLD = 1_700_000_000
T_NEW = 1_800_000_000


def _entry(token, name, typ, mtime, parent="root"):
    return SimpleNamespace(
        token=token, name=name, type=typ, modified_time=str(mtime), parent_token=parent, url=f"https://feishu/{token}"
    )


# A root folder with two files, one subfolder, and one native doc (must be skipped).
_ROOT = [
    _entry("f1", "spec.pdf", "file", T_NEW),
    _entry("f2", "budget.xlsx", "file", T_OLD),
    _entry("sub", "2024", "folder", T_NEW),
    _entry("d1", "plan.docx", "docx", T_NEW),
]
_SUB = [_entry("f3", "minutes.pdf", "file", T_NEW, parent="sub")]


class _FakeResp:
    def __init__(self, data=None, file_bytes=None, code=0, msg="ok"):
        self.data = data
        self._file = file_bytes
        self.code = code
        self.msg = msg

    def success(self):
        return self.code == 0

    def get_log_id(self):
        return "log123"

    @property
    def file(self):
        return SimpleNamespace(read=lambda: self._file)


class _FakeFileApi:
    def __init__(self, list_error_code=None):
        self._list_error_code = list_error_code

    def list(self, req):
        if self._list_error_code is not None:
            return _FakeResp(code=self._list_error_code, msg="forbidden")
        folder = getattr(req, "folder_token", "") or "root"
        files = _ROOT if folder == "root" else (_SUB if folder == "sub" else [])
        return _FakeResp(data=SimpleNamespace(files=files, has_more=False, next_page_token=None))

    def download(self, req):
        return _FakeResp(file_bytes=f"BYTES_OF_{req.file_token}".encode())


def _fake_client(list_error_code=None):
    return SimpleNamespace(drive=SimpleNamespace(v1=SimpleNamespace(file=_FakeFileApi(list_error_code))))


def _connector(folder_token="", list_error_code=None):
    c = FeishuDriveConnector(folder_token=folder_token)
    c.client = _fake_client(list_error_code)
    return c


# ---------------------------------------------------------------------------
# load_credentials
# ---------------------------------------------------------------------------


def test_load_credentials_requires_app_id_and_secret():
    c = FeishuDriveConnector()
    with pytest.raises(ConnectorMissingCredentialError):
        c.load_credentials({"feishu_app_id": "cli_x"})
    with pytest.raises(ConnectorMissingCredentialError):
        c.load_credentials({"feishu_app_secret": "secret"})


@patch("common.data_source.feishu_drive_connector.lark.Client")
def test_load_credentials_selects_feishu_domain_by_default(mock_client):
    builder = mock_client.builder.return_value
    builder.app_id.return_value = builder
    builder.app_secret.return_value = builder
    builder.domain.return_value = builder

    FeishuDriveConnector().load_credentials({"feishu_app_id": "cli_x", "feishu_app_secret": "s"})
    builder.domain.assert_called_once_with(lark.FEISHU_DOMAIN)


@patch("common.data_source.feishu_drive_connector.lark.Client")
def test_load_credentials_selects_lark_domain(mock_client):
    builder = mock_client.builder.return_value
    builder.app_id.return_value = builder
    builder.app_secret.return_value = builder
    builder.domain.return_value = builder

    c = FeishuDriveConnector()
    c.load_credentials({"feishu_app_id": "cli_x", "feishu_app_secret": "s", "feishu_domain": "lark"})
    builder.domain.assert_called_once_with(lark.LARK_DOMAIN)


@patch("common.data_source.feishu_drive_connector.lark.Client")
def test_load_credentials_picks_up_folder_token(mock_client):
    builder = mock_client.builder.return_value
    builder.app_id.return_value = builder
    builder.app_secret.return_value = builder
    builder.domain.return_value = builder

    c = FeishuDriveConnector()
    c.load_credentials({"feishu_app_id": "cli_x", "feishu_app_secret": "s", "feishu_folder_token": "FLDabc"})
    assert c.folder_token == "FLDabc"


# ---------------------------------------------------------------------------
# load_from_state — recursion, type filtering, download
# ---------------------------------------------------------------------------


def test_load_from_state_downloads_files_and_skips_native_docs():
    docs = [d for batch in _connector().load_from_state() for d in batch]
    ids = {d.id for d in docs}

    # f1, f2 in root + f3 in subfolder; native docx d1 skipped.
    assert ids == {"feishu_drive:f1", "feishu_drive:f2", "feishu_drive:f3"}
    assert "feishu_drive:d1" not in ids
    assert all(d.source == DocumentSource.FEISHU_DRIVE for d in docs)
    # blob bytes come from the download call
    assert all(d.blob == f"BYTES_OF_{d.id.split(':')[1]}".encode() for d in docs)
    # extension is derived from the filename
    by_id = {d.id: d for d in docs}
    assert by_id["feishu_drive:f1"].extension == ".pdf"
    assert by_id["feishu_drive:f2"].extension == ".xlsx"


def test_documents_carry_modified_time():
    docs = {d.id: d for batch in _connector().load_from_state() for d in batch}
    assert docs["feishu_drive:f1"].doc_updated_at.timestamp() == T_NEW
    assert docs["feishu_drive:f2"].doc_updated_at.timestamp() == T_OLD


# ---------------------------------------------------------------------------
# poll_source — incremental filtering
# ---------------------------------------------------------------------------


def test_poll_source_excludes_files_not_in_window():
    ids = {d.id for batch in _connector().poll_source(start=T_OLD, end=T_NEW + 1) for d in batch}
    # f2 (modified at T_OLD) is excluded because time <= start; f1/f3 (T_NEW) kept.
    assert ids == {"feishu_drive:f1", "feishu_drive:f3"}


# ---------------------------------------------------------------------------
# slim docs
# ---------------------------------------------------------------------------


def test_retrieve_all_slim_docs():
    ids = {s.id for batch in _connector().retrieve_all_slim_docs_perm_sync() for s in batch}
    assert ids == {"feishu_drive:f1", "feishu_drive:f2", "feishu_drive:f3"}


# ---------------------------------------------------------------------------
# error handling
# ---------------------------------------------------------------------------


def test_permission_error_is_mapped():
    c = _connector(list_error_code=1061004)
    with pytest.raises(InsufficientPermissionsError):
        c.validate_connector_settings()


def test_generic_list_error_raises_validation_error():
    c = _connector(list_error_code=1663)
    with pytest.raises(ConnectorValidationError):
        c.validate_connector_settings()


def test_operations_without_client_raise():
    c = FeishuDriveConnector()  # no client loaded
    with pytest.raises(ConnectorMissingCredentialError):
        list(c.load_from_state())
