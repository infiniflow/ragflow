#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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


# Standard library imports
import base64
import hashlib
import io
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import threading
import zipfile
import requests
import PyPDF2
from docx import Document
from io import BytesIO
from requests.exceptions import Timeout, RequestException

# Typing
from typing import List, Union, Tuple, Optional, Dict

# Third-party imports
import olefile
import pdfplumber
from cachetools import LRUCache, cached
from PIL import Image
from ruamel.yaml import YAML

# Local imports
from api.constants import IMG_BASE64_PREFIX
from api.db import FileType
from common.file_utils import get_project_base_directory

LOCK_KEY_pdfplumber = "global_shared_lock_pdfplumber"
if LOCK_KEY_pdfplumber not in sys.modules:
    sys.modules[LOCK_KEY_pdfplumber] = threading.Lock()


@cached(cache=LRUCache(maxsize=10))
def load_json_conf(conf_path):
    if os.path.isabs(conf_path):
        json_conf_path = conf_path
    else:
        json_conf_path = os.path.join(get_project_base_directory(), conf_path)
    try:
        with open(json_conf_path) as f:
            return json.load(f)
    except BaseException:
        raise EnvironmentError("loading json file config from '{}' failed!".format(json_conf_path))


def dump_json_conf(config_data, conf_path):
    if os.path.isabs(conf_path):
        json_conf_path = conf_path
    else:
        json_conf_path = os.path.join(get_project_base_directory(), conf_path)
    try:
        with open(json_conf_path, "w") as f:
            json.dump(config_data, f, indent=4)
    except BaseException:
        raise EnvironmentError("loading json file config from '{}' failed!".format(json_conf_path))


def load_json_conf_real_time(conf_path):
    if os.path.isabs(conf_path):
        json_conf_path = conf_path
    else:
        json_conf_path = os.path.join(get_project_base_directory(), conf_path)
    try:
        with open(json_conf_path) as f:
            return json.load(f)
    except BaseException:
        raise EnvironmentError("loading json file config from '{}' failed!".format(json_conf_path))


def load_yaml_conf(conf_path):
    if not os.path.isabs(conf_path):
        conf_path = os.path.join(get_project_base_directory(), conf_path)
    try:
        with open(conf_path) as f:
            yaml = YAML(typ="safe", pure=True)
            return yaml.load(f)
    except Exception as e:
        raise EnvironmentError("loading yaml file config from {} failed:".format(conf_path), e)


def rewrite_yaml_conf(conf_path, config):
    if not os.path.isabs(conf_path):
        conf_path = os.path.join(get_project_base_directory(), conf_path)
    try:
        with open(conf_path, "w") as f:
            yaml = YAML(typ="safe")
            yaml.dump(config, f)
    except Exception as e:
        raise EnvironmentError("rewrite yaml file config {} failed:".format(conf_path), e)


def rewrite_json_file(filepath, json_data):
    with open(filepath, "w", encoding="utf-8") as f:
        json.dump(json_data, f, indent=4, separators=(",", ": "))
    f.close()


def filename_type(filename):
    filename = filename.lower()
    if re.match(r".*\.pdf$", filename):
        return FileType.PDF.value

    if re.match(r".*\.(msg|eml|doc|docx|ppt|pptx|yml|xml|htm|json|jsonl|ldjson|csv|txt|ini|xls|xlsx|wps|rtf|hlp|pages|numbers|key|md|py|js|java|c|cpp|h|php|go|ts|sh|cs|kt|html|sql)$", filename):
        return FileType.DOC.value

    if re.match(r".*\.(wav|flac|ape|alac|wavpack|wv|mp3|aac|ogg|vorbis|opus)$", filename):
        return FileType.AURAL.value

    if re.match(r".*\.(jpg|jpeg|png|tif|gif|pcx|tga|exif|fpx|svg|psd|cdr|pcd|dxf|ufo|eps|ai|raw|WMF|webp|avif|apng|icon|ico|mpg|mpeg|avi|rm|rmvb|mov|wmv|asf|dat|asx|wvx|mpe|mpa|mp4|avi|mkv)$", filename):
        return FileType.VISUAL.value

    return FileType.OTHER.value


def thumbnail_img(filename, blob):
    """
    MySQL LongText max length is 65535
    """
    filename = filename.lower()
    if re.match(r".*\.pdf$", filename):
        with sys.modules[LOCK_KEY_pdfplumber]:
            pdf = pdfplumber.open(BytesIO(blob))

            buffered = BytesIO()
            resolution = 32
            img = None
            for _ in range(10):
                # https://github.com/jsvine/pdfplumber?tab=readme-ov-file#creating-a-pageimage-with-to_image
                pdf.pages[0].to_image(resolution=resolution).annotated.save(buffered, format="png")
                img = buffered.getvalue()
                if len(img) >= 64000 and resolution >= 2:
                    resolution = resolution / 2
                    buffered = BytesIO()
                else:
                    break
        pdf.close()
        return img

    elif re.match(r".*\.(jpg|jpeg|png|tif|gif|icon|ico|webp)$", filename):
        image = Image.open(BytesIO(blob))
        image.thumbnail((30, 30))
        buffered = BytesIO()
        image.save(buffered, format="png")
        return buffered.getvalue()

    elif re.match(r".*\.(ppt|pptx)$", filename):
        import aspose.pydrawing as drawing
        import aspose.slides as slides

        try:
            with slides.Presentation(BytesIO(blob)) as presentation:
                buffered = BytesIO()
                scale = 0.03
                img = None
                for _ in range(10):
                    # https://reference.aspose.com/slides/python-net/aspose.slides/slide/get_thumbnail/#float-float
                    presentation.slides[0].get_thumbnail(scale, scale).save(buffered, drawing.imaging.ImageFormat.png)
                    img = buffered.getvalue()
                    if len(img) >= 64000:
                        scale = scale / 2.0
                        buffered = BytesIO()
                    else:
                        break
                return img
        except Exception:
            pass
    return None


def thumbnail(filename, blob):
    img = thumbnail_img(filename, blob)
    if img is not None:
        return IMG_BASE64_PREFIX + base64.b64encode(img).decode("utf-8")
    else:
        return ""


def traversal_files(base):
    for root, ds, fs in os.walk(base):
        for f in fs:
            fullname = os.path.join(root, f)
            yield fullname


def repair_pdf_with_ghostscript(input_bytes):
    if shutil.which("gs") is None:
        return input_bytes

    with tempfile.NamedTemporaryFile(suffix=".pdf") as temp_in, tempfile.NamedTemporaryFile(suffix=".pdf") as temp_out:
        temp_in.write(input_bytes)
        temp_in.flush()

        cmd = [
            "gs",
            "-o",
            temp_out.name,
            "-sDEVICE=pdfwrite",
            "-dPDFSETTINGS=/prepress",
            temp_in.name,
        ]
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True)
            if proc.returncode != 0:
                return input_bytes
        except Exception:
            return input_bytes

        temp_out.seek(0)
        repaired_bytes = temp_out.read()

    return repaired_bytes


def read_potential_broken_pdf(blob):
    def try_open(blob):
        try:
            with pdfplumber.open(BytesIO(blob)) as pdf:
                if pdf.pages:
                    return True
        except Exception:
            return False
        return False

    if try_open(blob):
        return blob

    repaired = repair_pdf_with_ghostscript(blob)
    if try_open(repaired):
        return repaired

    return blob



def _is_zip(h: bytes) -> bool:
    return h.startswith(b"PK\x03\x04") or h.startswith(b"PK\x05\x06") or h.startswith(b"PK\x07\x08")

def _is_pdf(h: bytes) -> bool:
    return h.startswith(b"%PDF-")

def _is_ole(h: bytes) -> bool:
    return h.startswith(b"\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1")

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
        _ = int.from_bytes(data[pos:pos+4], "little")
        pos += 4
        # filename/src/tmp (NUL-terminated ANSI)
        for _ in range(3):
            z = data.index(b"\x00", pos)
            pos = z + 1
        # skip unknown 4 bytes
        pos += 4
        if pos + 4 > len(data):
            return data
        size = int.from_bytes(data[pos:pos+4], "little")
        pos += 4
        if pos + size <= len(data):
            return data[pos:pos+size]
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
                embed_dirs = (
                    "word/embeddings/", "word/objects/", "word/activex/",
                    "xl/embeddings/", "ppt/embeddings/"
                )
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
            if rel.reltype == (
                "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink"
            ):
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
        _GLOBAL_SESSION.headers.update({
            "User-Agent": (
                "Mozilla/5.0 (X11; Linux x86_64) "
                "AppleWebKit/537.36 (KHTML, like Gecko) "
                "Chrome/121.0 Safari/537.36"
            )
        })
    if headers:
        _GLOBAL_SESSION.headers.update(headers)
    return _GLOBAL_SESSION


def extract_html(
    url: str,
    timeout: float = 60.0,
    headers: Optional[Dict[str, str]] = None,
    max_retries: int = 2,
) -> Tuple[Optional[bytes], Dict[str, str]]:
    """
    Extract the full HTML page as raw bytes from a given URL.
    Automatically reuses a persistent HTTP session and applies robust timeout & retry logic.

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
    sess = _get_session(headers=headers)
    metadata = {"final_url": url, "status_code": "", "content_type": "", "error": ""}

    for attempt in range(1, max_retries + 1):
        try:
            resp = sess.get(url, timeout=timeout)
            resp.raise_for_status()

            html_bytes = resp.content
            metadata.update({
                "final_url": resp.url,
                "status_code": str(resp.status_code),
                "content_type": resp.headers.get("Content-Type", ""),
            })
            return html_bytes, metadata

        except Timeout:
            metadata["error"] = f"Timeout after {timeout}s (attempt {attempt}/{max_retries})"
            if attempt >= max_retries:
                continue
        except RequestException as e:
            metadata["error"] = f"Request failed: {e}"
            continue

    return None, metadata