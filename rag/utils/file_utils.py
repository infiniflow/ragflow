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

import io
import hashlib
import zipfile
import requests
from requests.exceptions import Timeout, RequestException
from urllib.parse import urljoin
from io import BytesIO
from typing import List, Union, Tuple, Optional, Dict
import pypdf as PyPDF2
from docx import Document
import olefile


def _is_zip(h: bytes) -> bool:
    return h.startswith(b"PK\x03\x04") or h.startswith(b"PK\x05\x06") or h.startswith(b"PK\x07\x08")


def _is_pdf(h: bytes) -> bool:
    return h.startswith(b"%PDF-")


def _is_ole(h: bytes) -> bool:
    return h.startswith(b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1")


def _sha10(b: bytes) -> str:
    return hashlib.sha256(b).hexdigest()[:10]


def _guess_ext(b: bytes) -> str:
    h = b[:8]
    if _is_zip(h):
        try:
            with zipfile.ZipFile(io.BytesIO(b), "r") as z:
                names = [n.lower() for n in z.namelist()]
                if any(n.startswith("word/") for n in names):
                    return ".docx"
                if any(n.startswith("ppt/") for n in names):
                    return ".pptx"
                if any(n.startswith("xl/") for n in names):
                    return ".xlsx"
        except Exception:
            pass
        return ".zip"
    if _is_pdf(h):
        return ".pdf"
    if _is_ole(h):
        return ".doc"
    return ".bin"


# Try to extract the real embedded payload from OLE's Ole10Native
def _extract_ole10native_payload(data: bytes) -> bytes:
    try:
        pos = 0
        if len(data) < 4:
            return data
        _ = int.from_bytes(data[pos : pos + 4], "little")
        pos += 4
        # filename/src/tmp (NUL-terminated ANSI)
        for _ in range(3):
            z = data.index(b"\x00", pos)
            pos = z + 1
        # skip unknown 4 bytes
        pos += 4
        if pos + 4 > len(data):
            return data
        size = int.from_bytes(data[pos : pos + 4], "little")
        pos += 4
        if pos + size <= len(data):
            return data[pos : pos + size]
    except Exception:
        pass
    return data


def extract_embed_file(target: Union[bytes, bytearray]) -> List[Tuple[str, bytes]]:
    """
    Only extract the 'first layer' of embedding, returning raw (filename, bytes).
    """
    top = bytes(target)
    head = top[:8]
    out: List[Tuple[str, bytes]] = []
    seen = set()

    def push(b: bytes, name_hint: str = ""):
        h10 = _sha10(b)
        if h10 in seen:
            return
        seen.add(h10)
        ext = _guess_ext(b)
        # If name_hint has an extension use its basename; else fallback to guessed ext
        if "." in name_hint:
            fname = name_hint.split("/")[-1]
        else:
            fname = f"{h10}{ext}"
        out.append((fname, b))

    # OOXML/ZIP container (docx/xlsx/pptx)
    if _is_zip(head):
        try:
            with zipfile.ZipFile(io.BytesIO(top), "r") as z:
                embed_dirs = ("word/embeddings/", "word/objects/", "word/activex/", "xl/embeddings/", "ppt/embeddings/")
                for name in z.namelist():
                    low = name.lower()
                    if any(low.startswith(d) for d in embed_dirs):
                        try:
                            b = z.read(name)
                            push(b, name)
                        except Exception:
                            pass
        except Exception:
            pass
        return out

    # OLE container (doc/ppt/xls)
    if _is_ole(head):
        try:
            with olefile.OleFileIO(io.BytesIO(top)) as ole:
                for entry in ole.listdir():
                    p = "/".join(entry)
                    try:
                        data = ole.openstream(entry).read()
                    except Exception:
                        continue
                    if not data:
                        continue
                    if "Ole10Native" in p or "ole10native" in p.lower():
                        data = _extract_ole10native_payload(data)
                    push(data, p)
        except Exception:
            pass
        return out

    return out


def extract_links_from_docx(docx_bytes: bytes):
    """
    Extract all hyperlinks from a Word (.docx) document binary stream.

    Args:
        docx_bytes (bytes): Raw bytes of a .docx file.

    Returns:
        set[str]: A set of unique hyperlink URLs.
    """
    links = set()
    with BytesIO(docx_bytes) as bio:
        document = Document(bio)

        # Each relationship may represent a hyperlink, image, footer, etc.
        for rel in document.part.rels.values():
            if rel.reltype == ("http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink"):
                links.add(rel.target_ref)

    return links


def extract_links_from_pdf(pdf_bytes: bytes):
    """
    Extract all clickable hyperlinks from a PDF binary stream.

    Args:
        pdf_bytes (bytes): Raw bytes of a PDF file.

    Returns:
        set[str]: A set of unique hyperlink URLs (unordered).
    """
    links = set()
    with BytesIO(pdf_bytes) as bio:
        pdf = PyPDF2.PdfReader(bio)

        for page in pdf.pages:
            annots = page.get("/Annots")
            if not annots or isinstance(annots, PyPDF2.generic.IndirectObject):
                continue
            for annot in annots:
                obj = annot.get_object()
                a = obj.get("/A")
                if a and a.get("/URI"):
                    links.add(a["/URI"])

    return links


_GLOBAL_SESSION: Optional[requests.Session] = None


def _get_session(headers: Optional[Dict[str, str]] = None) -> requests.Session:
    """Get or create a global reusable session."""
    global _GLOBAL_SESSION
    if _GLOBAL_SESSION is None:
        _GLOBAL_SESSION = requests.Session()
        _GLOBAL_SESSION.headers.update({"User-Agent": ("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0 Safari/537.36")})
    if headers:
        _GLOBAL_SESSION.headers.update(headers)
    return _GLOBAL_SESSION


# Upper bound on redirect hops followed by extract_html.  Each hop is
# re-validated and DNS-pinned, so this also caps the work an attacker can
# force via a redirect chain.
_MAX_HTML_REDIRECTS = 5


def extract_html(
    url: str,
    timeout: float = 60.0,
    headers: Optional[Dict[str, str]] = None,
    max_retries: int = 2,
) -> Tuple[Optional[bytes], Dict[str, str]]:
    """
    Extract the full HTML page as raw bytes from a given URL.
    Automatically reuses a persistent HTTP session and applies robust timeout & retry logic.

    The target URL is attacker-controlled: it is extracted from hyperlinks
    embedded in uploaded documents (see ``extract_links_from_docx`` /
    ``extract_links_from_pdf``, consumed by ``rag/app/naive.py`` when
    ``analyze_hyperlink`` is enabled).  To prevent SSRF, the URL is validated
    against :func:`common.ssrf_guard.assert_url_is_safe` before any request,
    and redirects are followed manually so that **every** hop is re-validated
    and DNS-pinned before the connection is made — closing the TOCTOU /
    DNS-rebinding window that ``allow_redirects=True`` with post-hoc checks
    would leave open.  This mirrors the hardening already applied to the RSS
    connector and the markdown image fetcher.

    Args:
        url (str): Target webpage URL.
        timeout (float): Request timeout in seconds (applies to connect + read).
        headers (dict, optional): Extra HTTP headers.
        max_retries (int): Number of retries on timeout or transient errors.

    Returns:
        tuple(bytes|None, dict):
            - html_bytes: Raw HTML content (or None if failed)
            - metadata: HTTP info (status_code, content_type, final_url, error if any)
    """
    from common.ssrf_guard import assert_url_is_safe, pin_dns

    session = _get_session(headers=headers)
    metadata = {"final_url": url, "status_code": "", "content_type": "", "error": ""}

    # Validate the initial URL once, up front, to obtain the pinned IP for the
    # first request and to reject obviously unsafe targets without retrying.
    try:
        initial_hostname, initial_ip = assert_url_is_safe(url)
    except ValueError as e:
        metadata["error"] = f"Blocked unsafe URL: {e}"
        return None, metadata

    for attempt in range(1, max_retries + 1):
        try:
            resp = None
            hostname, ip, target = initial_hostname, initial_ip, url
            for _ in range(_MAX_HTML_REDIRECTS + 1):
                with pin_dns(hostname, ip):
                    resp = session.get(target, timeout=timeout, allow_redirects=False)

                if resp.status_code not in (301, 302, 303, 307, 308):
                    break

                location = resp.headers.get("Location")
                if not location:
                    break  # broken redirect; let raise_for_status() handle it

                target = urljoin(target, location)
                # Re-validate (and re-pin) the redirect target before following it.
                hostname, ip = assert_url_is_safe(target)
            else:
                metadata["error"] = f"Exceeded {_MAX_HTML_REDIRECTS} redirects"
                return None, metadata

            resp.raise_for_status()

            html_bytes = resp.content
            metadata.update(
                {
                    "final_url": resp.url,
                    "status_code": str(resp.status_code),
                    "content_type": resp.headers.get("Content-Type", ""),
                }
            )
            return html_bytes, metadata

        except ValueError as e:
            # A redirect hop resolved to an unsafe address — do not retry.
            metadata["error"] = f"Blocked unsafe redirect: {e}"
            return None, metadata
        except Timeout:
            metadata["error"] = f"Timeout after {timeout}s (attempt {attempt}/{max_retries})"
            if attempt >= max_retries:
                continue
        except RequestException as e:
            metadata["error"] = f"Request failed: {e}"
            continue

    return None, metadata
