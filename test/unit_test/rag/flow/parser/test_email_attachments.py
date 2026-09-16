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

"""`Parser._email` has two paths a single-part message never reaches: the recursive
body walk, and the attachment loop. Any `multipart/*` message exercises the first —
with or without an attachment — and an attachment exercises the second. Both are
covered here.
"""

import importlib
import sys
from email.message import EmailMessage
from types import SimpleNamespace
from unittest.mock import Mock

import pytest

EMAIL_FIELDS = ["from", "to", "cc", "bcc", "date", "subject", "body", "attachments", "metadata"]


def _load_parser_module(monkeypatch):
    for module_name in ("rag.flow.parser.parser", "deepdoc.parser.pdf_parser", "deepdoc.parser", "deepdoc"):
        monkeypatch.delitem(sys.modules, module_name, raising=False)
    importlib.invalidate_caches()
    importlib.import_module("deepdoc.parser.pdf_parser")
    return importlib.import_module("rag.flow.parser.parser")


class _FakeProcess:
    def __init__(self):
        self._param = SimpleNamespace(
            setups={
                "email": {
                    "suffix": ["eml", "msg"],
                    "fields": EMAIL_FIELDS,
                    "output_format": "json",
                }
            }
        )
        self._canvas = SimpleNamespace(_tenant_id="tenant", _language="English")
        self.callback = Mock()
        self.outputs = {}

    def set_output(self, name, value):
        self.outputs[name] = value


def _eml(*, body="Please find the report attached.", attach=None):
    """Build a real .eml. Adding an attachment makes the message multipart/mixed."""
    msg = EmailMessage()
    msg["From"] = "sender@example.com"
    msg["To"] = "receiver@example.com"
    msg["Subject"] = "quarterly report"
    msg.set_content(body)
    if attach is not None:
        msg.add_attachment(attach["data"], maintype=attach["maintype"], subtype=attach["subtype"], filename=attach["filename"], **attach.get("kwargs", {}))
    return msg.as_bytes()


def _parse(parser_module, raw, name="message.eml"):
    process = _FakeProcess()
    parser_module.Parser._email(process, name, raw)
    return process.outputs["json"][0]


@pytest.mark.p2
def test_multipart_body_comes_from_the_part_not_the_whole_message(monkeypatch):
    """`get_payload(decode=True)` on a multipart container returns None. The body
    walk must read the part it was handed, not the top-level message."""
    parser_module = _load_parser_module(monkeypatch)
    raw = _eml(body="body survives", attach={"data": b"%PDF-1.4\n\xff\xd8bin", "maintype": "application", "subtype": "pdf", "filename": "report.pdf"})

    content = _parse(parser_module, raw)

    assert content["text"].strip() == "body survives"


@pytest.mark.p2
def test_single_part_body_still_parses(monkeypatch):
    parser_module = _load_parser_module(monkeypatch)

    content = _parse(parser_module, _eml(body="lone body"))

    assert content["text"].strip() == "lone body"
    assert content.get("attachments", []) == []


@pytest.mark.p2
@pytest.mark.parametrize(
    "maintype,subtype,data,filename",
    [
        ("application", "pdf", b"%PDF-1.4\n\xff\xd8bin", "report.pdf"),
        ("image", "png", b"\x89PNG\r\n\x1a\n\x00\x00\x00", "chart.png"),
    ],
)
def test_binary_attachment_keeps_its_name_and_contributes_no_text(monkeypatch, maintype, subtype, data, filename):
    """A binary attachment carries no charset, so decoding its bytes as text would
    only push mojibake into the indexed content. Keep the name, drop the payload."""
    parser_module = _load_parser_module(monkeypatch)
    raw = _eml(attach={"data": data, "maintype": maintype, "subtype": subtype, "filename": filename})

    content = _parse(parser_module, raw)

    assert [a["filename"] for a in content["attachments"]] == [filename]
    assert content["attachments"][0]["payload"] == ""


@pytest.mark.p2
def test_utf8_text_attachment_is_decoded(monkeypatch):
    parser_module = _load_parser_module(monkeypatch)
    raw = _eml(attach={"data": b"line one\nline two\n", "maintype": "text", "subtype": "plain", "filename": "notes.txt"})

    content = _parse(parser_module, raw)

    assert content["attachments"][0]["filename"] == "notes.txt"
    assert "line one" in content["attachments"][0]["payload"]


@pytest.mark.p2
def test_gbk_text_attachment_goes_through_the_fallback_chain(monkeypatch):
    """A charset the payload does not actually honour must fall back rather than raise."""
    parser_module = _load_parser_module(monkeypatch)
    raw = _eml(
        attach={
            "data": "中文内容".encode("gbk"),
            "maintype": "text",
            "subtype": "plain",
            "filename": "chinese.txt",
            "kwargs": {"params": {"charset": "utf-8"}},
        }
    )

    content = _parse(parser_module, raw)

    assert content["attachments"][0]["filename"] == "chinese.txt"
    assert content["attachments"][0]["payload"] == "中文内容"


@pytest.mark.p2
def test_msg_binary_attachment_keeps_its_name_and_contributes_no_text(monkeypatch):
    """The .msg branch hardcodes utf-8, which raises on the same inputs."""
    parser_module = _load_parser_module(monkeypatch)
    fake_extract_msg = SimpleNamespace(
        Message=Mock(
            return_value=SimpleNamespace(
                sender="sender@example.com",
                to="receiver@example.com",
                cc=None,
                bcc=None,
                date="Mon, 8 Sep 2026 10:00:00 +0800",
                subject="quarterly report",
                body="body survives",
                htmlBody=None,
                messageId="<id@example.com>",
                inReplyTo=None,
                attachments=[SimpleNamespace(name="report.pdf", data=b"%PDF-1.4\n\xff\xd8bin")],
            )
        )
    )
    monkeypatch.setitem(sys.modules, "extract_msg", fake_extract_msg)

    content = _parse(parser_module, b"ole2-bytes", name="message.msg")

    assert content["text"] == "body survives"
    assert [a["filename"] for a in content["attachments"]] == ["report.pdf"]
    assert content["attachments"][0]["payload"] == ""


def _multipart_alternative(*, plain="plain body", html="<p>html body</p>", attach=None):
    """A plain+HTML message. This is `multipart/alternative` on its own, and
    `multipart/mixed` wrapping an alternative once an attachment is added."""
    msg = EmailMessage()
    msg["From"] = "sender@example.com"
    msg["To"] = "receiver@example.com"
    msg["Subject"] = "newsletter"
    msg.set_content(plain)
    msg.add_alternative(html, subtype="html")
    if attach is not None:
        msg.add_attachment(attach["data"], maintype=attach["maintype"], subtype=attach["subtype"], filename=attach["filename"])
    return msg.as_bytes()


@pytest.mark.p2
def test_multipart_alternative_without_attachment_recovers_both_bodies(monkeypatch):
    """The wrong-object read fired on any multipart container, so a plain+HTML mail
    with no attachment at all was already failing."""
    parser_module = _load_parser_module(monkeypatch)

    content = _parse(parser_module, _multipart_alternative())

    assert content["text"].strip() == "plain body"
    assert "html body" in content["text_html"]


@pytest.mark.p2
def test_nested_multipart_walks_into_the_alternative_and_the_attachment(monkeypatch):
    """`multipart/mixed` wrapping a `multipart/alternative` plus a binary attachment:
    the walk has to recurse twice, and both leaf branches have to read their own part."""
    parser_module = _load_parser_module(monkeypatch)
    raw = _multipart_alternative(attach={"data": b"%PDF-1.4\n\xff\xd8bin", "maintype": "application", "subtype": "pdf", "filename": "report.pdf"})

    content = _parse(parser_module, raw)

    assert content["text"].strip() == "plain body"
    assert "html body" in content["text_html"]
    assert [a["filename"] for a in content["attachments"]] == ["report.pdf"]
    assert content["attachments"][0]["payload"] == ""
