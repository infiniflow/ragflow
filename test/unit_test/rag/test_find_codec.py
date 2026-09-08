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

import codecs
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


@pytest.mark.p2
def test_find_codec_utf8_emoji_roundtrips(find_codec):
    # Regression: the repo-pinned chardet 5.2.0 reports Windows-1254 at 0.56
    # confidence, so trusting it before checking UTF-8 mis-decodes this blob.
    text = "Sales are up 20% this quarter \U0001f4c8\U0001f680"
    blob = text.encode("utf-8")
    assert blob.decode(find_codec(blob)) == text


@pytest.mark.p2
def test_find_codec_rejects_utf8_when_only_the_sample_looks_valid(find_codec):
    # An incremental decoder buffers a trailing partial sequence instead of
    # rejecting it, so a sample-only check calls this blob UTF-8. The full blob
    # is not valid UTF-8, and callers decode with errors="ignore", which drops
    # bytes. The returned codec must decode the whole blob.
    blob = b"a" * 1023 + b"\xe9" + b" "
    codec = find_codec(blob)
    assert codec != "utf-8"
    assert isinstance(blob.decode(codec), str)


@pytest.mark.p2
@pytest.mark.parametrize(
    "text, encoding",
    [
        ("Ежедневный отчет о продажах за прошлую неделю показывает рост.", "cp1251"),
        ("Η ετήσια έκθεση πωλήσεων δείχνει σημαντική αύξηση στα έσοδα.", "cp1253"),
    ],
)
def test_find_codec_uses_low_confidence_detection(find_codec, monkeypatch, text, encoding):
    # The old code gated on confidence > 0.5, so a correct detection below the
    # gate was discarded and the brute-force loop returned utf_16 instead,
    # which mis-decodes. Both samples are verified not to decode as UTF-8, so
    # the UTF-8 check does not short-circuit.
    nlp = sys.modules["rag.nlp"]
    monkeypatch.setattr(
        nlp.chardet,
        "detect",
        lambda blob: {"encoding": encoding, "confidence": 0.3},
    )
    blob = text.encode(encoding)
    with pytest.raises(UnicodeDecodeError):
        blob.decode("utf-8")
    assert find_codec(blob) == encoding
    assert blob.decode(find_codec(blob)) == text


@pytest.mark.p2
def test_find_codec_invalid_detected_codec_falls_back(find_codec, monkeypatch):
    # This covers the LookupError fallback; it need not fail against old code.
    nlp = sys.modules["rag.nlp"]
    monkeypatch.setattr(
        nlp.chardet,
        "detect",
        lambda blob: {"encoding": "not-a-real-codec", "confidence": 0.99},
    )
    blob = "Ελληνικά".encode("cp1253")
    codec = find_codec(blob)
    codecs.lookup(codec)
    assert isinstance(blob.decode(codec), str)


@pytest.mark.p2
def test_find_codec_utf8_wins_over_confident_wrong_detection(find_codec, monkeypatch):
    # These UTF-8 bytes are valid cp1254, so old code returns the wrong guess.
    nlp = sys.modules["rag.nlp"]
    monkeypatch.setattr(
        nlp.chardet,
        "detect",
        lambda blob: {"encoding": "cp1254", "confidence": 0.99},
    )
    blob = "Ελληνικά".encode("utf-8")
    assert find_codec(blob) == "utf-8"


@pytest.fixture
def decode_text(find_codec):
    from rag.nlp import decode_text as _decode_text

    return _decode_text


@pytest.mark.p2
def test_decode_text_preserves_gbk_and_gb18030(decode_text):
    gbk = "项目名称,核心系统重构".encode("gbk")
    text, encoding = decode_text(gbk, document_type="CSV document")
    assert text == "项目名称,核心系统重构"
    assert encoding == "gb18030"

    gb18030_only = "𠀀".encode("gb18030")
    text, encoding = decode_text(gb18030_only, document_type="CSV document")
    assert text == "𠀀"
    assert encoding == "gb18030"


@pytest.mark.p2
def test_decode_text_rejects_undetectable_bytes_without_loss(decode_text):
    with pytest.raises(UnicodeError, match="CSV document"):
        decode_text(b"prefix\xc3suffix", document_type="CSV document")


@pytest.mark.p2
def test_decode_text_accepts_literal_replacement_character(decode_text):
    text, encoding = decode_text("literal � content".encode("utf-8"))
    assert text == "literal � content"
    assert encoding == "utf-8"
