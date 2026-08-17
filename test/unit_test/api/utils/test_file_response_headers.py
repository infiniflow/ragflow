#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#

from urllib.parse import urlencode

import pytest

from api.utils import file_response as module


class _DummyHeaders(dict):
    def set(self, key, value):
        self[key] = value


class _DummyResponse:
    def __init__(self):
        self.headers = _DummyHeaders()


@pytest.mark.p2
def test_apply_preview_sets_inline_for_pdf():
    response = _DummyResponse()
    module.apply_preview_file_response_headers(response, "application/pdf", "pdf", "report.pdf")
    assert response.headers["Content-Type"] == "application/pdf"
    assert response.headers["Content-Disposition"] == 'inline; filename="report.pdf"'


@pytest.mark.p2
def test_apply_preview_forces_attachment_for_html():
    response = _DummyResponse()
    module.apply_preview_file_response_headers(response, "text/html", "html", "page.html")
    assert response.headers["Content-Disposition"] == "attachment"
    assert response.headers["X-Content-Type-Options"] == "nosniff"


@pytest.mark.p2
def test_apply_download_sets_attachment_for_pdf():
    response = _DummyResponse()
    module.apply_download_file_response_headers(response, "application/pdf", "pdf", "report.pdf")
    assert response.headers["Content-Disposition"] == 'attachment; filename="report.pdf"'


@pytest.mark.p2
def test_resolve_attachment_content_type_prefers_mime_type():
    content_type, ext = module.resolve_attachment_content_type(ext="md", mime_type="application/pdf")
    assert content_type == "application/pdf"
    assert ext == "pdf"


@pytest.mark.p2
def test_agent_attachment_preview_path_includes_query():
    path = module.agent_attachment_preview_path("doc-1", ext="pdf", mime_type="application/pdf")
    query = urlencode({"ext": "pdf", "mime_type": "application/pdf"})
    assert path == f"/api/v1/agents/attachments/doc-1/preview?{query}"


@pytest.mark.p2
def test_agent_attachment_preview_path_encodes_svg_mime_type():
    path = module.agent_attachment_preview_path("doc-1", ext="svg", mime_type="image/svg+xml")
    query = urlencode({"ext": "svg", "mime_type": "image/svg+xml"})
    assert path == f"/api/v1/agents/attachments/doc-1/preview?{query}"
    assert "%2B" in path or "svg%2Bxml" in path
