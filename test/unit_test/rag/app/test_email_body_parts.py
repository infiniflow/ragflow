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
"""Each part of an email reaches the chunks once: the body in one rendering,
an attachment as an attachment."""

from __future__ import annotations

from email.message import EmailMessage

import pytest

from rag.app import email


@pytest.fixture(autouse=True)
def _stub_rag_tokenizer(monkeypatch):
    def fake_tokenize(text):
        return str(text)

    monkeypatch.setattr("rag.nlp.rag_tokenizer.tokenize", fake_tokenize)
    monkeypatch.setattr("rag.nlp.rag_tokenizer.fine_grained_tokenize", fake_tokenize)


def _message(plain="Plain rendering of the body."):
    msg = EmailMessage()
    msg["From"] = "sender@example.com"
    msg["To"] = "receiver@example.com"
    msg["Subject"] = "quarterly numbers"
    msg.set_content(plain)
    return msg


def _chunk_text(msg):
    chunks = email.chunk("message.eml", binary=msg.as_bytes(), callback=lambda *a, **k: None)
    return "\n".join(chunk["content_with_weight"] for chunk in chunks)


@pytest.mark.p2
def test_a_plain_and_html_body_is_read_once():
    """The last alternative is the preferred one, and only one is read."""
    msg = _message()
    msg.add_alternative("<html><body><p>HTML rendering of the body.</p></body></html>", subtype="html")

    text = _chunk_text(msg)

    assert text.count("HTML rendering of the body.") == 1
    assert "Plain rendering of the body." not in text


@pytest.mark.p2
def test_an_alternative_the_parser_cannot_read_is_passed_over():
    """A meeting invitation ends in text/calendar; the HTML before it is read."""
    msg = _message()
    msg.add_alternative("<html><body><p>HTML rendering of the body.</p></body></html>", subtype="html")
    msg.add_alternative("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n", subtype="calendar")

    text = _chunk_text(msg)

    assert text.count("HTML rendering of the body.") == 1
    assert "Plain rendering of the body." not in text
    assert "VCALENDAR" not in text


@pytest.mark.p2
def test_an_html_body_with_inline_images_is_read_once():
    """With inline images the HTML alternative is itself multipart/related."""
    msg = _message()
    msg.add_alternative('<html><body><p>HTML rendering of the body.</p><img src="cid:chart"></body></html>', subtype="html")
    msg.get_payload()[1].add_related(b"\x89PNG\r\n\x1a\n", maintype="image", subtype="png", cid="<chart>")

    text = _chunk_text(msg)

    assert text.count("HTML rendering of the body.") == 1
    assert "Plain rendering of the body." not in text


@pytest.mark.p2
def test_a_text_attachment_is_read_as_an_attachment_only():
    msg = _message(plain="The notes are attached.")
    msg.add_attachment(b"Attachment line about budgets.", maintype="text", subtype="plain", filename="notes.txt")

    text = _chunk_text(msg)

    assert text.count("The notes are attached.") == 1
    assert text.count("Attachment line about budgets.") == 1
