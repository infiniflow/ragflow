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

"""`tika.parser.from_buffer` answers with `content=None` for a `.doc` it cannot
extract any text from: an empty or unreadable body, an encrypted document, or a
Tika server that answers with nothing.

#3564 added the guard for that to `naive.py`, closing #3229
("'NoneType' object has no attribute 'split'"). #11807 carried it into
`laws.py`. #12073 carried it into `one.py` and `book.py` but kept only its
`if content is not None:` half, so those two have been falling through ever
since.

Every `rag/app` chunker whose `.doc` branch goes through tika is parametrized
here; `manual.py` has a `.doc` branch too but rejects the format outright.
`laws` and `naive` are the control group: they already behave correctly, so they
must stay green both before and after the fix.
"""

from __future__ import annotations

import importlib
import sys

import pytest

# A display name, not a path on disk, plus the OLE2 magic a real .doc starts with.
DOC_NAME = "legacy.doc"
DOC_BINARY = b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"
EMPTY_CONTENT_MESSAGE = f"tika.parser got empty content from {DOC_NAME}."

DOC_MODULES = ["book", "laws", "naive", "one"]


class _Callback:
    """Records what the chunker reports, the way `task_executor` does."""

    def __init__(self):
        self.messages: list[str] = []

    def __call__(self, prog=None, msg=""):
        if msg:
            self.messages.append(msg)


def _load(module_name: str):
    """Import a `rag.app` module lazily, inside the test that needs it."""
    return importlib.import_module(f"rag.app.{module_name}")


@pytest.fixture(scope="module", autouse=True)
def _real_pdf_parser():
    """Swap the `deepdoc.parser.pdf_parser` stub for the real package, once.

    `test/unit_test/rag/conftest.py` installs that stub at conftest import time
    so the tests that only need `rag.nlp` do not pull in deepdoc. The chunkers
    here import `deepdoc.parser` for real, which fails against the stub, so drop
    it and let the import machinery find the package again.

    The `rag.app` modules go with it, and neither half is put back. A `rag.app`
    copy built while the stub was installed subclasses the stub's
    `RAGFlowPdfParser`, so a later test patching `deepdoc.parser.PdfParser`
    would patch a class that copy no longer inherits from. Dropping both, and
    restoring neither, leaves the session with exactly one copy of deepdoc.
    """
    for module_name in [name for name in list(sys.modules) if name == "deepdoc" or name.startswith("deepdoc.")]:
        del sys.modules[module_name]
    for module_name in [name for name in list(sys.modules) if name == "rag.app" or name.startswith("rag.app.")]:
        del sys.modules[module_name]
    importlib.invalidate_caches()
    importlib.import_module("deepdoc.parser.pdf_parser")


@pytest.fixture
def tika_content(monkeypatch):
    """Drive `from_buffer`'s return value without a Tika server or a JVM."""
    import tika.parser

    def _set(content):
        monkeypatch.setattr(tika.parser, "from_buffer", lambda *args, **kwargs: {"content": content, "metadata": {}, "status": 200})

    return _set


@pytest.mark.parametrize("module_name", DOC_MODULES)
def test_doc_without_tika_content_reports_and_returns_nothing(module_name, tika_content, caplog):
    """No crash, no chunks, and the user is told why."""
    tika_content(None)
    callback = _Callback()

    with caplog.at_level("WARNING"):
        chunks = _load(module_name).chunk(DOC_NAME, binary=DOC_BINARY, callback=callback)

    assert chunks == []
    assert EMPTY_CONTENT_MESSAGE in callback.messages
    # Reporting success and then failing is what made this hard to diagnose.
    assert "Finish parsing." not in callback.messages
    assert EMPTY_CONTENT_MESSAGE in caplog.text


@pytest.mark.parametrize("module_name", DOC_MODULES)
def test_doc_with_tika_content_is_unaffected(module_name, tika_content):
    """The guard must not cost the path that does have content."""
    tika_content("First paragraph.\nSecond paragraph.\nThird paragraph.\n")
    callback = _Callback()

    chunks = _load(module_name).chunk(DOC_NAME, binary=DOC_BINARY, callback=callback)

    assert chunks
    assert "Finish parsing." in callback.messages
