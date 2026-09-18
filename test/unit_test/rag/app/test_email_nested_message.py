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

"""Forwarding a mail as an attachment produces a `message/rfc822` part. That part
is a container — `is_multipart()` is True and `iter_parts()` yields the forwarded
message — but `message/rfc822` does not contain the substring "multipart", so a
body walk that tests the content-type string never descends into it and the whole
forwarded message is lost.

`common/data_source/imap_connector.py:662` already branches on `is_multipart()`
for exactly this reason, with a comment spelling out that a container part is not
content. The Go port asserts the same user-visible outcome in
`TestEmailParser_NestedEMLAttachmentRechunk`
(`internal/parser/parser/email_parser_test.go:997`), though it reaches it through
its attachment re-parse path rather than through the body walk.
"""

from __future__ import annotations

import importlib
import sys
from email.message import EmailMessage

import pytest

NAME = "forwarded.eml"
OUTER_BODY = "See the thread below."
INNER_BODY = "Q3 revenue was up 12 percent."


def _deepdoc_and_chunker_modules():
    """The `sys.modules` keys this module swaps out, as a fresh list."""
    return [name for name in list(sys.modules) if name in ("deepdoc", "rag.app") or name.startswith(("deepdoc.", "rag.app."))]


@pytest.fixture(scope="module", autouse=True)
def _real_pdf_parser():
    """Swap the `deepdoc.parser.pdf_parser` stub for the real package, then put it back.

    `test/unit_test/rag/conftest.py` installs that stub at conftest import time so
    the tests that only need `rag.nlp` do not pull in deepdoc. `rag.app.email`
    imports `deepdoc.parser` for real, which fails against the stub. The `rag.app`
    modules go with it, and both halves are restored on the way out — including the
    `app` attribute on the `rag` package, which `sys.modules` alone does not cover —
    so the session is left exactly as this module found it.
    """
    saved_modules = {name: sys.modules[name] for name in _deepdoc_and_chunker_modules()}
    rag_package = sys.modules.get("rag")
    unset = object()
    saved_app_attr = getattr(rag_package, "app", unset) if rag_package is not None else unset

    for name in _deepdoc_and_chunker_modules():
        del sys.modules[name]
    importlib.invalidate_caches()
    importlib.import_module("deepdoc.parser.pdf_parser")

    yield

    for name in _deepdoc_and_chunker_modules():
        del sys.modules[name]
    sys.modules.update(saved_modules)
    if rag_package is not None:
        if saved_app_attr is unset:
            rag_package.__dict__.pop("app", None)
        else:
            rag_package.app = saved_app_attr
    importlib.invalidate_caches()


def _noop_callback(*args, **kwargs):
    return None


def _forwarded(*, inner_html=None):
    """Build the shape every mail client produces for "forward as attachment"."""
    inner = EmailMessage()
    inner["From"] = "analyst@example.com"
    inner["To"] = "sender@example.com"
    inner["Subject"] = "Q3 revenue"
    inner.set_content(INNER_BODY)
    if inner_html is not None:
        inner.add_alternative(inner_html, subtype="html")

    outer = EmailMessage()
    outer["From"] = "sender@example.com"
    outer["To"] = "receiver@example.com"
    outer["Subject"] = "Fwd: Q3 revenue"
    outer.set_content(OUTER_BODY)
    outer.add_attachment(inner)
    return outer.as_bytes()


def _indexed_text(chunks):
    return " ".join(chunk.get("content_with_weight", "") for chunk in chunks)


@pytest.mark.p2
def test_forwarded_message_body_reaches_the_index():
    """The forwarded message is the content the user wanted indexed; losing it is
    total and silent."""
    email_module = importlib.import_module("rag.app.email")

    indexed = _indexed_text(email_module.chunk(NAME, binary=_forwarded(), callback=_noop_callback))

    assert OUTER_BODY in indexed
    assert INNER_BODY in indexed
    # Descend once, not twice: a duplicated walk would index every body again.
    assert indexed.count(OUTER_BODY) == 1
    assert indexed.count(INNER_BODY) == 1


@pytest.mark.p2
def test_forwarded_message_html_body_reaches_the_index():
    """Recovering the forwarded HTML takes two descents: through the
    `message/rfc822` part and then through the `multipart/alternative` inside it."""
    email_module = importlib.import_module("rag.app.email")

    indexed = _indexed_text(email_module.chunk(NAME, binary=_forwarded(inner_html="<p>Q3 revenue chart</p>"), callback=_noop_callback))

    assert indexed.count("Q3 revenue chart") == 1


@pytest.mark.p2
def test_plain_multipart_mail_is_unaffected():
    """The guard must not cost the shapes that already worked."""
    email_module = importlib.import_module("rag.app.email")

    msg = EmailMessage()
    msg["From"] = "sender@example.com"
    msg["To"] = "receiver@example.com"
    msg["Subject"] = "quarterly report"
    msg.set_content("plain body")
    msg.add_alternative("<p>html body</p>", subtype="html")
    msg.add_attachment(b"\x89PNG\r\n", maintype="image", subtype="png", filename="chart.png")

    indexed = _indexed_text(email_module.chunk(NAME, binary=msg.as_bytes(), callback=_noop_callback))

    assert indexed.count("plain body") == 1
    assert indexed.count("html body") == 1
    # The HTML alternative still goes through HtmlParser, and a binary leaf stays
    # out of the body walk entirely.
    assert "<p>" not in indexed
    assert "PNG" not in indexed
