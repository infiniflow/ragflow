import io
import json
import zipfile

from deepdoc.parser.monkeyocrv2_parser import MonkeyOCRv2Parser


def make_zip(name, text):
    out = io.BytesIO()
    with zipfile.ZipFile(out, "w") as z:
        z.writestr(f"{name}/{name}.md", text)
        z.writestr(f"{name}/jsons/{name}.json", json.dumps({"layouts": [{"label": "Text", "content": text, "bbox": [1, 2, 30, 40], "page_num": 2}]}))
    return out.getvalue()


def test_converts_native_layout_to_sections(monkeypatch):
    class Response:
        headers = {"content-type": "application/zip"}
        content = make_zip("doc", "hello")
        def raise_for_status(self): pass
    monkeypatch.setattr("deepdoc.parser.monkeyocrv2_parser.requests.post", lambda *a, **k: Response())
    sections, tables = MonkeyOCRv2Parser("http://parser").parse_pdf("doc.pdf", binary=b"pdf", page_from=1, page_to=3)
    assert sections == [("hello", "@@2\t1\t30\t2\t40##")]
    assert tables == []


def test_sends_page_range_and_accepts_binary(monkeypatch):
    seen = {}
    class Response:
        headers = {"content-type": "application/zip"}; content = make_zip("doc", "ok")
        def raise_for_status(self): pass
    def post(url, **kwargs):
        seen.update(kwargs); return Response()
    monkeypatch.setattr("deepdoc.parser.monkeyocrv2_parser.requests.post", post)
    MonkeyOCRv2Parser("http://parser").parse_pdf("doc.pdf", binary=b"pdf", page_from=4, page_to=9)
    assert seen["data"] == {"start_page_id": 4, "end_page_id": 9}
