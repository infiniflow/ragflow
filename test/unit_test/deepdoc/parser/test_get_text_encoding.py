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

import importlib.util
import os
import sys
import types
from unittest import mock

import pytest

# get_text lives in deepdoc/parser/utils.py. We load it by file path (see the
# get_text fixture) so the test does not pull in deepdoc/parser/__init__.py's
# heavy transitive imports (the pypdf-backed parsers, etc.). Its one real
# dependency is the production rag.nlp.find_codec, which we exercise directly:
# if rag.nlp cannot be imported in this lane we skip rather than substitute a
# divergent stand-in that would test different codec coverage than production.
find_codec = pytest.importorskip("rag.nlp").find_codec


def _find_project_root(marker="pyproject.toml"):
    d = os.path.dirname(os.path.abspath(__file__))
    while d != os.path.dirname(d):
        if os.path.exists(os.path.join(d, marker)):
            return d
        d = os.path.dirname(d)
    raise RuntimeError(f"could not locate project root (missing {marker})")


@pytest.fixture(scope="module")
def get_text():
    """Load get_text from deepdoc/parser/utils.py by file path.

    utils.py imports pypdf (used only by the unrelated extract_pdf_outlines), so
    a lightweight stand-in is registered when pypdf is unavailable. Every
    sys.modules mutation -- the optional pypdf stand-in and the freshly loaded
    ``deepdoc.parser.utils`` entry -- is scoped by ``mock.patch.dict`` and
    reverted on teardown, so later tests import the application's real modules.
    """
    utils_path = os.path.join(_find_project_root(), "deepdoc", "parser", "utils.py")

    stubs = {}
    if "pypdf" not in sys.modules:
        pypdf_stub = types.ModuleType("pypdf")
        pypdf_stub.PdfReader = object
        stubs["pypdf"] = pypdf_stub

    with mock.patch.dict(sys.modules, stubs):
        spec = importlib.util.spec_from_file_location("deepdoc.parser.utils", utils_path)
        module = importlib.util.module_from_spec(spec)
        # Registered inside the patched dict so patch.dict removes it on exit.
        sys.modules["deepdoc.parser.utils"] = module
        spec.loader.exec_module(module)
        yield module.get_text


def _write(tmp_path, name, data: bytes) -> str:
    p = tmp_path / name
    p.write_bytes(data)
    return str(p)


@pytest.mark.p2
def test_reads_gbk_encoded_file_from_disk(get_text, tmp_path):
    # A GBK-encoded text file (common for Chinese documents) is not valid UTF-8.
    # Reading it via the file path used to open() in the platform-default text
    # mode and raise UnicodeDecodeError; it must now be decoded through
    # find_codec and round-trip to the original text.
    text = "你好，世界"
    fnm = _write(tmp_path, "gbk.txt", text.encode("gbk"))
    assert get_text(fnm) == text


@pytest.mark.p2
def test_reads_utf8_encoded_file_from_disk(get_text, tmp_path):
    text = "你好，世界"
    fnm = _write(tmp_path, "utf8.txt", text.encode("utf-8"))
    assert get_text(fnm) == text


@pytest.mark.p2
def test_reads_plain_ascii_file_from_disk(get_text, tmp_path):
    fnm = _write(tmp_path, "ascii.txt", b"hello world")
    assert get_text(fnm) == "hello world"


@pytest.mark.p2
def test_non_utf8_file_is_decoded_via_find_codec(get_text, tmp_path):
    # The core regression the fix targets: a non-UTF-8 byte stream must be read
    # without raising and decoded through the production find_codec, instead of
    # crashing in the platform-default text mode. Using a Latin-1 payload as a
    # non-UTF-8, non-CJK example, we assert get_text reproduces exactly what the
    # real find_codec dictates for the same bytes (its full contract).
    data = "café résumé".encode("latin-1")
    fnm = _write(tmp_path, "latin1.txt", data)
    result = get_text(fnm)
    assert isinstance(result, str)
    assert result == data.decode(find_codec(data), errors="ignore")


@pytest.mark.p2
def test_binary_argument_still_decoded_via_find_codec(get_text):
    # The binary branch is unchanged: passing bytes decodes them directly and the
    # file name is ignored.
    assert get_text("ignored-name.txt", "数据".encode("utf-8")) == "数据"
