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
import os
import re
import shutil
import subprocess
import sys
import tempfile
import threading
from io import BytesIO

import pdfplumber
from PIL import Image

# Local imports
from api.constants import FILE_NAME_LEN_LIMIT, IMG_BASE64_PREFIX
from api.db import FileType

# Robustness and resource limits: reject oversized inputs to avoid DoS and OOM.
MAX_BLOB_SIZE_THUMBNAIL = 50 * 1024 * 1024  # 50 MiB for thumbnail generation
MAX_BLOB_SIZE_PDF = 100 * 1024 * 1024       # 100 MiB for PDF repair / read
GHOSTSCRIPT_TIMEOUT_SEC = 120                # Timeout for Ghostscript subprocess

LOCK_KEY_pdfplumber = "global_shared_lock_pdfplumber"
if LOCK_KEY_pdfplumber not in sys.modules:
    sys.modules[LOCK_KEY_pdfplumber] = threading.Lock()


def _normalize_filename_for_type(filename):
    """Extract a safe basename for type detection. Returns (normalized_str, True) or ("", False)."""
    if filename is None:
        return "", False
    if not isinstance(filename, str):
        return "", False
    base = os.path.basename(filename).strip()
    if not base or len(base) > FILE_NAME_LEN_LIMIT:
        return "", False
    return base.lower(), True


def filename_type(filename):
    """Return file type from extension. Handles None, empty, path-only, and oversized names."""
    normalized, ok = _normalize_filename_for_type(filename)
    if not ok:
        return FileType.OTHER.value
    filename = normalized
    if re.match(r".*\.pdf$", filename):
        return FileType.PDF.value

    if re.match(r".*\.(msg|eml|doc|docx|ppt|pptx|yml|xml|htm|json|jsonl|ldjson|csv|txt|ini|xls|xlsx|wps|rtf|hlp|pages|numbers|key|md|mdx|py|js|java|c|cpp|h|php|go|ts|sh|cs|kt|html|sql)$", filename):
        return FileType.DOC.value

    if re.match(r".*\.(wav|flac|ape|alac|wavpack|wv|mp3|aac|ogg|vorbis|opus)$", filename):
        return FileType.AURAL.value

    if re.match(r".*\.(jpg|jpeg|png|tif|gif|pcx|tga|exif|fpx|svg|psd|cdr|pcd|dxf|ufo|eps|ai|raw|WMF|webp|avif|apng|icon|ico|mpg|mpeg|avi|rm|rmvb|mov|wmv|asf|dat|asx|wvx|mpe|mpa|mp4|avi|mkv)$", filename):
        return FileType.VISUAL.value

    return FileType.OTHER.value


def thumbnail_img(filename, blob):
    """
    Generate thumbnail image bytes for PDF, image, or PPT. MySQL LongText max length is 65535.

    Robustness and edge cases:
    - Rejects None, empty, or oversized blob to avoid DoS/OOM.
    - Uses basename for type detection (handles paths like "a/b/c.pdf").
    - Catches corrupt or malformed files and returns None instead of raising.
    - Normalizes PIL image mode (e.g. RGBA -> RGB) for safe PNG export.
    """
    if blob is None:
        return None
    try:
        blob_len = len(blob)
    except TypeError:
        return None
    if blob_len == 0 or blob_len > MAX_BLOB_SIZE_THUMBNAIL:
        return None

    normalized, ok = _normalize_filename_for_type(filename)
    if not ok:
        return None
    filename = normalized

    if re.match(r".*\.pdf$", filename):
        try:
            with sys.modules[LOCK_KEY_pdfplumber]:
                pdf = pdfplumber.open(BytesIO(blob))
                if not pdf.pages:
                    pdf.close()
                    return None
                buffered = BytesIO()
                resolution = 32
                img = None
                for _ in range(10):
                    pdf.pages[0].to_image(resolution=resolution).annotated.save(buffered, format="png")
                    img = buffered.getvalue()
                    if len(img) >= 64000 and resolution >= 2:
                        resolution = resolution / 2
                        buffered = BytesIO()
                    else:
                        break
                pdf.close()
                return img
        except Exception:
            return None

    if re.match(r".*\.(jpg|jpeg|png|tif|gif|icon|ico|webp)$", filename):
        try:
            image = Image.open(BytesIO(blob))
            image.load()
            if image.mode in ("RGBA", "P", "LA"):
                image = image.convert("RGB")
            image.thumbnail((30, 30))
            buffered = BytesIO()
            image.save(buffered, format="png")
            return buffered.getvalue()
        except Exception:
            return None

    if re.match(r".*\.(ppt|pptx)$", filename):
        try:
            import aspose.pydrawing as drawing
            import aspose.slides as slides
        except ImportError:
            return None

        try:
            with slides.Presentation(BytesIO(blob)) as presentation:
                if not presentation.slides or len(presentation.slides) == 0:
                    return None
                buffered = BytesIO()
                scale = 0.03
                img = None
                for _ in range(10):
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


def repair_pdf_with_ghostscript(input_bytes):
    """Attempt to repair corrupt PDF bytes via Ghostscript. Returns original bytes on failure or timeout."""
    if input_bytes is None or len(input_bytes) == 0:
        return input_bytes if input_bytes is not None else b""
    if len(input_bytes) > MAX_BLOB_SIZE_PDF:
        return input_bytes

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
            proc = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=GHOSTSCRIPT_TIMEOUT_SEC,
            )
            if proc.returncode != 0:
                return input_bytes
            temp_out.seek(0)
            repaired_bytes = temp_out.read()
            if not repaired_bytes:
                return input_bytes
            return repaired_bytes
        except subprocess.TimeoutExpired:
            return input_bytes
        except Exception:
            return input_bytes


def read_potential_broken_pdf(blob):
    """
    Return PDF bytes, optionally repaired via Ghostscript if initially unreadable.

    Edge cases and robustness:
    - None blob returns b"" to avoid callers receiving None.
    - Empty blob returned as-is.
    - Oversized blob (> MAX_BLOB_SIZE_PDF) returned as-is without repair to avoid DoS.
    """
    if blob is None:
        return b""
    try:
        blob_len = len(blob)
    except TypeError:
        return b""
    if blob_len == 0:
        return blob

    def try_open(data):
        try:
            with pdfplumber.open(BytesIO(data)) as pdf:
                if pdf.pages:
                    return True
        except Exception:
            return False
        return False

    if try_open(blob):
        return blob

    if blob_len > MAX_BLOB_SIZE_PDF:
        return blob

    repaired = repair_pdf_with_ghostscript(blob)
    if try_open(repaired):
        return repaired

    return blob


def sanitize_path(raw_path: str | None) -> str:
    """Normalize and sanitize a user-provided path segment.

    - Converts backslashes to forward slashes
    - Strips leading/trailing slashes
    - Removes '.' and '..' segments
    - Restricts characters to A-Za-z0-9, underscore, dash, and '/'
    - Returns "" for None, empty, or non-string input (robustness).
    """
    if raw_path is None or not isinstance(raw_path, str):
        return ""
    raw_path = raw_path.strip()
    if not raw_path:
        return ""
    backslash_re = re.compile(r"[\\]+")
    unsafe_re = re.compile(r"[^A-Za-z0-9_\-/]")
    normalized = backslash_re.sub("/", raw_path)
    normalized = normalized.strip("/")
    parts = [seg for seg in normalized.split("/") if seg and seg not in (".", "..")]
    sanitized = "/".join(parts)
    sanitized = unsafe_re.sub("", sanitized)
    return sanitized
