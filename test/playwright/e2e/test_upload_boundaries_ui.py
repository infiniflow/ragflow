"""Shared upload dialog behavior, with API failures injected at the HTTP boundary.

These checks prove browser handling, not server-side file/parser validation.
"""

import re
from email import policy
from email.parser import BytesParser
from urllib.parse import urlparse

import pytest
from playwright.sync_api import expect

from test.playwright.e2e.test_auth_boundaries_ui import _AuthApi, _session


class _UploadApi(_AuthApi):
    def __init__(self, *, failure=0):
        super().__init__()
        self.failure = failure
        self.uploads = []
        self.list_reads = 0

    def __call__(self, route):
        if urlparse(route.request.url).path == "/api/v1/files":
            if route.request.method == "POST":
                content_type = route.request.headers["content-type"]
                multipart = BytesParser(policy=policy.default).parsebytes(f"Content-Type: {content_type}\r\n\r\n".encode() + route.request.post_data_buffer)
                self.uploads.append([(part.get_filename(), part.get_payload(decode=True)) for part in multipart.iter_parts() if part.get_filename()])
                status = self.failure if self.failure else 200
                route.fulfill(
                    status=status,
                    json={
                        "code": self.failure,
                        "data": [],
                        "message": "Upload rejected by server" if self.failure else "",
                    },
                )
                return
            self.list_reads += 1
        super().__call__(route)


def _open_upload(page, base_url, api):
    _session(page)
    # WebKit's request inspection omits multipart file bytes. Observe the actual
    # FormData passed by the application; preserve the native fetch and response.
    page.add_init_script("""
      window.__qaUploadBodies = [];
      const nativeFetch = window.fetch;
      window.fetch = function(input, options) {
        const url = new URL(typeof input === 'string' ? input : input.url, location.href);
        if (url.pathname === '/api/v1/files' && options?.body instanceof FormData) {
          const files = [...options.body.values()].filter(value => value instanceof File);
          window.__qaUploadBodies.push(Promise.all(files.map(async file =>
            [file.name, [...new Uint8Array(await file.arrayBuffer())]])));
        }
        return nativeFetch.apply(this, arguments);
      };
    """)
    page.route("**/api/**", api)
    page.goto(f"{base_url}/files")
    expect(page.get_by_test_id("files-list")).to_be_visible()
    page.get_by_role("button", name=re.compile("add file", re.I)).click()
    page.get_by_role("menuitem", name=re.compile("upload file", re.I)).click()
    modal = page.get_by_test_id("dataset-upload-modal")
    expect(modal).to_be_visible()
    return modal


def _select(modal, *, name="qa.txt", content=b"QA upload boundary"):
    modal.locator("input[type=file]:not([webkitdirectory])").set_input_files({"name": name, "mimeType": "application/octet-stream", "buffer": content})
    expect(modal.get_by_text(name, exact=True)).to_be_visible()


def test_empty_selection_and_removal_prevent_upload(page, base_url):
    api = _UploadApi()
    modal = _open_upload(page, base_url, api)
    save = modal.get_by_role("button", name="Save", exact=True)
    save.click()
    expect(modal.get_by_text(re.compile("upload at least one file", re.I))).to_be_visible()
    _select(modal)
    modal.get_by_role("button", name="Remove file").click()
    expect(modal.get_by_text("qa.txt", exact=True)).to_have_count(0)
    save.click()
    expect(modal.get_by_text(re.compile("upload at least one file", re.I))).to_be_visible()
    assert api.uploads == []


def test_cancel_discards_selection_without_upload(page, base_url):
    api = _UploadApi()
    modal = _open_upload(page, base_url, api)
    _select(modal)
    modal.get_by_role("button", name="Close", exact=True).click()
    expect(modal).to_have_count(0)
    page.get_by_role("button", name=re.compile("add file", re.I)).click()
    page.get_by_role("menuitem", name=re.compile("upload file", re.I)).click()
    expect(page.get_by_test_id("dataset-upload-modal")).to_be_visible()
    expect(page.get_by_text("qa.txt", exact=True)).to_have_count(0)
    assert api.uploads == []


@pytest.mark.parametrize(
    "name,content,status",
    [
        ("empty.txt", b"", 400),
        ("corrupt.pdf", b"%PDF-1.7\ninvalid-pdf", 400),
        ("large.txt", b"QA payload", 413),
    ],
    ids=["zero-byte-server-rejection", "corrupt-server-rejection", "http-413"],
)
def test_rejected_upload_preserves_selection_and_can_retry(page, base_url, name, content, status):
    api = _UploadApi(failure=status)
    modal = _open_upload(page, base_url, api)
    _select(modal, name=name, content=content)
    save = modal.get_by_role("button", name="Save", exact=True)
    with page.expect_response(lambda response: response.request.method == "POST" and urlparse(response.url).path == "/api/v1/files"):
        save.click()
    expect(save).to_be_enabled()
    expect(modal).to_be_visible()
    expect(modal.get_by_text(name, exact=True)).to_be_visible()
    expect(page.get_by_text(re.compile(rf"Request error {status}:"))).to_be_visible()
    assert len(api.uploads) == 1
    assert [filename for filename, _ in api.uploads[0]] == [name]
    assert page.evaluate("Promise.all(window.__qaUploadBodies)") == [[[name, list(content)]]]
    reads_before_retry = api.list_reads
    api.failure = 0
    save.click()
    expect(modal).to_have_count(0)
    assert len(api.uploads) == 2
    assert [filename for filename, _ in api.uploads[1]] == [name]
    assert page.evaluate("Promise.all(window.__qaUploadBodies)") == [[[name, list(content)]]] * 2
    expect(page.get_by_test_id("files-list")).to_be_visible()
    assert api.list_reads > reads_before_retry
