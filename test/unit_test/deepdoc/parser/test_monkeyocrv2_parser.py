import io
import json
import zipfile

import pytest

from deepdoc.parser.monkeyocrv2_parser import MonkeyOCRv2Parser


class Response:
    def __init__(self, content):
        self.headers = {"content-type": "application/zip"}
        self.content = content

    def raise_for_status(self):
        pass


def make_zip(name, text, *, label="Text", include_markdown=True):
    out = io.BytesIO()
    with zipfile.ZipFile(out, "w") as z:
        if include_markdown:
            z.writestr(f"{name}/{name}.md", text)
        z.writestr(f"{name}/jsons/{name}.json", json.dumps({"layouts": [{"label": label, "content": text, "bbox": [1, 2, 30, 40], "page_num": 2}]}))
    return out.getvalue()


def test_converts_native_layout_to_sections(monkeypatch):
    monkeypatch.setattr("deepdoc.parser.monkeyocrv2_parser.requests.post", lambda *a, **k: Response(make_zip("doc", "hello")))
    sections, tables = MonkeyOCRv2Parser("http://parser").parse_pdf("doc.pdf", binary=b"pdf", page_from=1, page_to=3)
    assert sections == [("hello", "@@2\t1\t30\t2\t40##")]
    assert tables == []


def test_converts_json_only_archive_without_markdown(monkeypatch):
    monkeypatch.setattr("deepdoc.parser.monkeyocrv2_parser.requests.post", lambda *a, **k: Response(make_zip("doc", "json only", include_markdown=False)))
    sections, tables = MonkeyOCRv2Parser("http://parser").parse_pdf("doc.pdf", binary=b"pdf")
    assert sections == [("json only", "@@2\t1\t30\t2\t40##")]
    assert tables == []


def test_converts_canonical_root_json_archive(monkeypatch):
    out = io.BytesIO()
    with zipfile.ZipFile(out, "w") as archive:
        archive.writestr(
            "doc/doc.json",
            json.dumps({"layouts": [{"label": "Text", "content": "canonical", "bbox": [1, 2, 30, 40], "page_num": 1}]}),
        )

    monkeypatch.setattr("deepdoc.parser.monkeyocrv2_parser.requests.post", lambda *a, **k: Response(out.getvalue()))
    sections, tables = MonkeyOCRv2Parser("http://parser").parse_pdf("doc.pdf", binary=b"pdf")
    assert sections == [("canonical", "@@1\t1\t30\t2\t40##")]
    assert tables == []


def test_converts_all_results_only_archive(monkeypatch):
    out = io.BytesIO()
    with zipfile.ZipFile(out, "w") as archive:
        archive.writestr(
            "doc/all_results.json",
            json.dumps({"layouts": [{"label": "Text", "content": "summary", "bbox": [1, 2, 30, 40], "page_num": 2}]}),
        )

    monkeypatch.setattr("deepdoc.parser.monkeyocrv2_parser.requests.post", lambda *a, **k: Response(out.getvalue()))
    sections, tables = MonkeyOCRv2Parser("http://parser").parse_pdf("doc.pdf", binary=b"pdf")
    assert sections == [("summary", "@@2\t1\t30\t2\t40##")]
    assert tables == []


def test_converts_table_layout_with_position(monkeypatch):
    monkeypatch.setattr("deepdoc.parser.monkeyocrv2_parser.requests.post", lambda *a, **k: Response(make_zip("doc", "a | b", label="Table")))
    sections, tables = MonkeyOCRv2Parser("http://parser").parse_pdf("doc.pdf", binary=b"pdf")
    assert sections == []
    assert tables == [((None, "a | b"), [(1, 1, 30, 2, 40)])]


def test_skips_malformed_layout_records(monkeypatch):
    out = io.BytesIO()
    with zipfile.ZipFile(out, "w") as archive:
        archive.writestr("doc/jsons/doc.json", json.dumps({"layouts": [None, {"content": "bad", "page_num": None, "bbox": [1]}, {"content": "ok", "page_num": 1, "bbox": [1, 2, 3, 4]}]}))

    monkeypatch.setattr("deepdoc.parser.monkeyocrv2_parser.requests.post", lambda *a, **k: Response(out.getvalue()))
    sections, tables = MonkeyOCRv2Parser("http://parser").parse_pdf("doc.pdf", binary=b"pdf")
    assert sections == [("ok", "@@1\t1\t3\t2\t4##")]
    assert tables == []


def test_emits_slice_relative_page_numbers(monkeypatch):
    monkeypatch.setattr("deepdoc.parser.monkeyocrv2_parser.requests.post", lambda *a, **k: Response(make_zip("doc", "page", include_markdown=False)))
    sections, _ = MonkeyOCRv2Parser("http://parser").parse_pdf("doc.pdf", binary=b"pdf", page_from=4)
    assert sections == [("page", "@@2\t1\t30\t2\t40##")]


def test_wraps_malformed_zip_response(monkeypatch):
    monkeypatch.setattr("deepdoc.parser.monkeyocrv2_parser.requests.post", lambda *a, **k: Response(b"not a zip archive"))
    with pytest.raises(RuntimeError, match="Invalid MonkeyOCRv2 parse response"):
        MonkeyOCRv2Parser("http://parser").parse_pdf("doc.pdf", binary=b"pdf")


def test_sends_page_range_and_accepts_binary(monkeypatch):
    seen = {}

    def post(url, **kwargs):
        seen.update(kwargs)
        return Response(make_zip("doc", "ok"))

    monkeypatch.setattr("deepdoc.parser.monkeyocrv2_parser.requests.post", post)
    MonkeyOCRv2Parser("http://parser").parse_pdf("doc.pdf", binary=b"pdf", page_from=4, page_to=9)
    assert seen["data"] == {"start_page_id": 4, "end_page_id": 9}
