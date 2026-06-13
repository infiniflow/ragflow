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

import sys
import types

import pytest


@pytest.fixture
def find_codec(monkeypatch):
    """Import ``rag.nlp.find_codec`` with heavy module-level deps stubbed.

    The stubs are installed through ``monkeypatch`` so they are restored after
    each test instead of leaking fakes into ``sys.modules`` for unrelated tests.
    ``chardet`` is intentionally left real, since ``find_codec`` relies on real
    encoding detection.
    """

    def _stub(name, **attrs):
        module = types.ModuleType(name)
        for key, value in attrs.items():
            setattr(module, key, value)
        monkeypatch.setitem(sys.modules, name, module)
        return module

    _stub("common.token_utils", num_tokens_from_string=lambda *a, **k: 0)
    _stub("roman_numbers")
    _stub("word2number", w2n=types.SimpleNamespace())
    _stub("cn2an", cn2an=lambda *a, **k: 0)
    pil = _stub("PIL")
    pil.Image = _stub("PIL.Image")

    # Another test module stubs chardet at import time; drop any such stub so the
    # fresh rag.nlp import below binds the real chardet (find_codec needs it).
    monkeypatch.delitem(sys.modules, "chardet", raising=False)

    # Force a fresh import so the stubs above are picked up; monkeypatch restores
    # the module table (including rag.nlp) afterwards.
    monkeypatch.delitem(sys.modules, "rag.nlp", raising=False)
    from rag.nlp import find_codec as _find_codec

    return _find_codec


@pytest.mark.p2
@pytest.mark.parametrize(
    "text, encoding",
    [
        ("こんにちは世界。これは日本語のサンプルテキストです。" * 14, "shift_jis"),
        ("안녕하세요 세계. 이것은 한국어 샘플 텍스트입니다. " * 12, "euc_kr"),
    ],
)
def test_find_codec_honors_confident_detection(find_codec, text, encoding):
    # Regression: find_codec computed a confident chardet detection but only
    # used it for the ascii special case; for any other encoding it fell through
    # to the brute-force all_codecs loop, which returns the first codec that
    # does not raise. Legacy single-byte codecs like cp037 (and gbk/gb2312 for
    # these bytes) decode arbitrary input without error, so a non-UTF-8 upload
    # came back mis-decoded as garbage. The returned codec must reproduce the
    # original text.
    blob = text.encode(encoding)
    codec = find_codec(blob)
    assert blob.decode(codec) == text


@pytest.mark.p2
def test_find_codec_ascii_maps_to_utf8(find_codec):
    assert find_codec(b"plain ascii content, nothing special here") == "utf-8"


@pytest.mark.p2
def test_find_codec_utf8_roundtrips(find_codec):
    text = "Lorem ipsum UTF-8 sample 日本語 and Ελληνικά mixed together."
    blob = text.encode("utf-8")
    assert blob.decode(find_codec(blob)) == text
