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
"""Unit tests for ``RAGFlowS3.put`` and ``RAGFlowS3.get`` retry semantics.

Covers issue #17065: the previous ``for _ in range(1):`` loops silently
swallowed transient failures. ``put`` should retry and re-raise on
exhaustion; ``get`` should retry and either re-raise (non-404) or return
``None`` (real 404).
"""

import sys
from io import BytesIO
from unittest.mock import MagicMock

import pytest
from botocore.exceptions import ClientError


def _install_no_op_singleton():
    """Replace ``@singleton`` with identity before importing the module under test.

    ``common.decorator.singleton`` has already wrapped the class with a
    factory function by the time we get here, so the live class is hidden.
    Replacing the decorator symbol in the module's namespace and
    re-importing ``rag.utils.s3_conn`` restores access to the underlying
    class for test instantiation.
    """
    import importlib

    # Load ``common.settings`` first; it does ``from rag.utils.s3_conn
    # import RAGFlowS3`` at module load, which forces the s3_conn module
    # to be initialized as a side effect. After that we can patch the
    # decorator and pull the real class.
    importlib.import_module("common.settings")

    decorator_module = importlib.import_module("common.decorator")
    decorator_module.singleton = lambda cls, *a, **kw: cls

    if "rag.utils.s3_conn" in sys.modules:
        del sys.modules["rag.utils.s3_conn"]
    return importlib.import_module("rag.utils.s3_conn")


_s3_module = _install_no_op_singleton()
RAGFlowS3 = _s3_module.RAGFlowS3
MAX_RETRIES = _s3_module.MAX_RETRIES


def _make_instance():
    """Build a bare ``RAGFlowS3`` instance without triggering ``__init__``/``__open__``."""
    instance = object.__new__(RAGFlowS3)
    instance.conn = [MagicMock()]
    instance.bucket = None
    instance.prefix_path = None
    return instance


def _client_error(code):
    """Construct a ``ClientError`` response with the given error code."""
    return ClientError({"Error": {"Code": code, "Message": code}}, "op")


@pytest.fixture
def patched(monkeypatch):
    """Apply module-level patches: re-route sleep, ``bucket_exists``, and ``__open__`` to no-ops."""
    sleeps = []

    def fake_sleep(_seconds):
        sleeps.append(_seconds)

    monkeypatch.setattr(_s3_module.time, "sleep", fake_sleep)

    opens = []

    def fake_open(self):
        opens.append(1)

    monkeypatch.setattr(RAGFlowS3, "__open__", fake_open)

    exists = {"bucket_exists": True}

    def fake_bucket_exists(self, bucket, *a, **kw):
        return exists["bucket_exists"]

    monkeypatch.setattr(RAGFlowS3, "bucket_exists", fake_bucket_exists)

    return {"sleeps": sleeps, "opens": opens, "exists": exists}


class TestPut:
    """Test the ``put`` retry loop."""

    def test_success_first_attempt(self, patched):
        instance = _make_instance()
        instance.conn[0].upload_fileobj.return_value = {"ok": True}

        result = instance.put("bkt", "key", b"data")

        assert result == {"ok": True}
        assert instance.conn[0].upload_fileobj.call_count == 1
        args, _ = instance.conn[0].upload_fileobj.call_args
        assert args[1] == "bkt"
        assert args[2] == "key"
        assert args[0].read() == b"data"
        assert patched["sleeps"] == []
        assert patched["opens"] == []

    def test_recovers_after_transient(self, patched):
        instance = _make_instance()
        instance.conn[0].upload_fileobj.side_effect = [
            OSError("transient"),
            {"ok": True},
        ]

        result = instance.put("bkt", "key", b"data")

        assert result == {"ok": True}
        assert instance.conn[0].upload_fileobj.call_count == 2
        assert len(patched["opens"]) == 1
        assert patched["sleeps"] == [1]

    def test_recovers_after_two_transient(self, patched):
        instance = _make_instance()
        instance.conn[0].upload_fileobj.side_effect = [
            OSError("transient 1"),
            OSError("transient 2"),
            {"ok": True},
        ]

        result = instance.put("bkt", "key", b"data")

        assert result == {"ok": True}
        assert instance.conn[0].upload_fileobj.call_count == 3
        assert len(patched["opens"]) == 2
        assert patched["sleeps"] == [1, 2]

    def test_raises_after_exhaustion(self, patched):
        instance = _make_instance()
        instance.conn[0].upload_fileobj.side_effect = OSError("persistent")

        with pytest.raises(OSError, match="persistent"):
            instance.put("bkt", "key", b"data")

        assert instance.conn[0].upload_fileobj.call_count == MAX_RETRIES
        assert len(patched["opens"]) == MAX_RETRIES
        assert patched["sleeps"] == [1, 2, 4]

    def test_raises_last_exception_not_first(self, patched):
        instance = _make_instance()
        instance.conn[0].upload_fileobj.side_effect = [
            OSError("first"),
            OSError("middle"),
            OSError("last"),
        ]

        with pytest.raises(OSError, match="last"):
            instance.put("bkt", "key", b"data")

    def test_exponential_backoff_called(self, patched):
        instance = _make_instance()
        instance.conn[0].upload_fileobj.side_effect = [
            OSError("a"),
            OSError("b"),
            {"ok": True},
        ]

        instance.put("bkt", "key", b"data")

        assert patched["sleeps"] == [1, 2]


class TestGet:
    """Test the ``get`` retry loop."""

    def test_success_first_attempt(self, patched):
        instance = _make_instance()
        instance.conn[0].get_object.return_value = {"Body": BytesIO(b"binary")}

        result = instance.get("bkt", "key")

        assert result == b"binary"
        assert instance.conn[0].get_object.call_count == 1

    def test_returns_none_on_real_404(self, patched):
        instance = _make_instance()
        instance.conn[0].get_object.side_effect = _client_error("404")

        result = instance.get("bkt", "key")

        assert result is None
        assert instance.conn[0].get_object.call_count == 1

    def test_recovers_after_transient(self, patched):
        instance = _make_instance()
        instance.conn[0].get_object.side_effect = [
            _client_error("InternalError"),
            {"Body": BytesIO(b"binary")},
        ]

        result = instance.get("bkt", "key")

        assert result == b"binary"
        assert instance.conn[0].get_object.call_count == 2
        assert len(patched["opens"]) == 1
        assert patched["sleeps"] == [1]

    def test_raises_on_non_404_after_exhaustion(self, patched):
        instance = _make_instance()
        err = _client_error("AccessDenied")
        instance.conn[0].get_object.side_effect = err

        with pytest.raises(ClientError) as excinfo:
            instance.get("bkt", "key")

        assert excinfo.value.response["Error"]["Code"] == "AccessDenied"
        assert instance.conn[0].get_object.call_count == MAX_RETRIES
        assert patched["sleeps"] == [1, 2, 4]

    def test_recovers_after_generic_exception(self, patched):
        instance = _make_instance()
        instance.conn[0].get_object.side_effect = [
            OSError("transient"),
            {"Body": BytesIO(b"binary")},
        ]

        result = instance.get("bkt", "key")

        assert result == b"binary"
        assert instance.conn[0].get_object.call_count == 2

    def test_raises_last_non_404(self, patched):
        instance = _make_instance()
        instance.conn[0].get_object.side_effect = [
            _client_error("InternalError"),
            _client_error("middle"),
            _client_error("AccessDenied"),
        ]

        with pytest.raises(ClientError) as excinfo:
            instance.get("bkt", "key")

        assert excinfo.value.response["Error"]["Code"] == "AccessDenied"
