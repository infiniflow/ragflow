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
import logging
import re
import shutil
import subprocess
import sys
import tempfile
import threading
from io import BytesIO

import pdfplumber
from PIL import Image

logger = logging.getLogger(__name__)

# Local imports
from api.constants import IMG_BASE64_PREFIX
from api.db import FileType

LOCK_KEY_pdfplumber = "global_shared_lock_pdfplumber"
if LOCK_KEY_pdfplumber not in sys.modules:
    sys.modules[LOCK_KEY_pdfplumber] = threading.Lock()


def filename_type(filename):
    filename = filename.lower()
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
    Generate a thumbnail image for the given file. Returns raw PNG bytes or None on failure.
    MySQL LongText max length is 65535; thumbnail is kept under that limit.

    Robustness and edge cases:
    - None or empty blob/filename returns None.
    - Corrupt or unreadable PDF/image files are caught and return None instead of raising.
    - PDF with zero pages is handled without raising IndexError.
    - PDF and image resources are always closed (try/finally) to avoid leaks.
    """
    if blob is None or (isinstance(blob, (bytes, bytearray)) and len(blob) == 0):
        logger.debug("thumbnail_img: skipping empty or None blob")
        return None
    if filename is None or not isinstance(filename, str) or not filename.strip():
        logger.debug("thumbnail_img: skipping invalid or empty filename")
        return None

    try:
        blob_bytes = bytes(blob) if not isinstance(blob, bytes) else blob
    except (TypeError, ValueError):
        logger.warning("thumbnail_img: blob is not bytes-like")
        return None

    filename_lower = filename.lower().strip()

    if re.match(r".*\.pdf$", filename_lower):
        pdf = None
        try:
            with sys.modules[LOCK_KEY_pdfplumber]:
                pdf = pdfplumber.open(BytesIO(blob_bytes))
                if not pdf.pages:
                    logger.warning("thumbnail_img: PDF has no pages")
                    return None
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
                return img
        except (IndexError, KeyError) as e:
            logger.warning("thumbnail_img: PDF structure error (e.g. no pages): %s", e)
            return None
        except Exception as e:
            logger.warning("thumbnail_img: PDF thumbnail failed: %s", e)
            return None
        finally:
            if pdf is not None:
                try:
                    pdf.close()
                except Exception as e:
                    logger.debug("thumbnail_img: error closing PDF: %s", e)

    if re.match(r".*\.(jpg|jpeg|png|tif|gif|icon|ico|webp)$", filename_lower):
        try:
            image = Image.open(BytesIO(blob_bytes))
            image.load()
        except OSError as e:
            logger.warning("thumbnail_img: corrupt or unsupported image: %s", e)
            return None
        except Exception as e:
            logger.warning("thumbnail_img: image open failed: %s", e)
            return None
        try:
            image.thumbnail((30, 30))
            buffered = BytesIO()
            image.save(buffered, format="png")
            return buffered.getvalue()
        except Exception as e:
            logger.warning("thumbnail_img: image thumbnail/save failed: %s", e)
            return None

    if re.match(r".*\.(ppt|pptx)$", filename_lower):
        try:
            import aspose.pydrawing as drawing
            import aspose.slides as slides

            with slides.Presentation(BytesIO(blob_bytes)) as presentation:
                if not presentation.slides or len(presentation.slides) == 0:
                    logger.warning("thumbnail_img: presentation has no slides")
                    return None
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
        except Exception as e:
            logger.debug("thumbnail_img: ppt/pptx thumbnail failed: %s", e)
    return None


def thumbnail(filename, blob):
    img = thumbnail_img(filename, blob)
    if img is not None:
        return IMG_BASE64_PREFIX + base64.b64encode(img).decode("utf-8")
    else:
        return ""


def repair_pdf_with_ghostscript(input_bytes):
    """
    Attempt to repair potentially broken PDF bytes using Ghostscript.

    Robustness and edge cases:
    - None or non-bytes input is returned unchanged; empty bytes return as-is.
    - If Ghostscript is not installed or subprocess fails, original input_bytes is returned.
    - Temp files are always cleaned up (context manager); no partial output on exception.
    """
    if input_bytes is None:
        return None
    if not isinstance(input_bytes, (bytes, bytearray)):
        logger.warning("repair_pdf_with_ghostscript: expected bytes, got %s", type(input_bytes).__name__)
        return input_bytes
    if len(input_bytes) == 0:
        return input_bytes

    if shutil.which("gs") is None:
        logger.debug("repair_pdf_with_ghostscript: gs not found, returning input unchanged")
        return input_bytes

    try:
        with tempfile.NamedTemporaryFile(suffix=".pdf", delete=True) as temp_in, tempfile.NamedTemporaryFile(suffix=".pdf", delete=True) as temp_out:
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
                proc = subprocess.run(cmd, capture_output=True, text=True, timeout=60)
            except subprocess.TimeoutExpired:
                logger.warning("repair_pdf_with_ghostscript: gs timed out")
                return input_bytes
            except Exception as e:
                logger.warning("repair_pdf_with_ghostscript: subprocess error: %s", e)
                return input_bytes

            if proc.returncode != 0:
                if proc.stderr:
                    logger.debug("repair_pdf_with_ghostscript: gs non-zero exit %s: %s", proc.returncode, proc.stderr[:200])
                return input_bytes

            temp_out.seek(0)
            repaired_bytes = temp_out.read()
            if not repaired_bytes:
                logger.warning("repair_pdf_with_ghostscript: gs produced empty output")
                return input_bytes
            return repaired_bytes
    except OSError as e:
        logger.warning("repair_pdf_with_ghostscript: temp file or read error: %s", e)
        return input_bytes
    except Exception as e:
        logger.warning("repair_pdf_with_ghostscript: unexpected error: %s", e)
        return input_bytes


def _pdf_try_open(blob):
    """Return True if blob can be opened as a PDF with at least one page."""
    if blob is None or (isinstance(blob, (bytes, bytearray)) and len(blob) == 0):
        return False
    try:
        buf = BytesIO(blob) if isinstance(blob, (bytes, bytearray)) else BytesIO(bytes(blob))
    except (TypeError, ValueError):
        return False
    try:
        with pdfplumber.open(buf) as pdf:
            return bool(pdf.pages)
    except Exception:
        return False


def read_potential_broken_pdf(blob):
    """
    Return a readable PDF byte sequence, attempting Ghostscript repair if initial open fails.

    Robustness and edge cases:
    - None blob returns None; empty bytes return empty bytes.
    - Non-bytes-like blob is normalized to bytes where possible; otherwise returned unchanged.
    - Repair is only attempted when blob is bytes-like and open fails; repair output is validated.
    """
    if blob is None:
        return None
    if isinstance(blob, (bytes, bytearray)) and len(blob) == 0:
        return blob

    try:
        blob_bytes = bytes(blob) if not isinstance(blob, bytes) else blob
    except (TypeError, ValueError):
        logger.warning("read_potential_broken_pdf: blob is not bytes-like, returning as-is")
        return blob

    if _pdf_try_open(blob_bytes):
        return blob_bytes

    repaired = repair_pdf_with_ghostscript(blob_bytes)
    if repaired is not None and repaired is not blob_bytes and _pdf_try_open(repaired):
        return repaired

    return blob_bytes


def sanitize_path(raw_path: str | None) -> str:
    """Normalize and sanitize a user-provided path segment.

    - Converts backslashes to forward slashes
    - Strips leading/trailing slashes
    - Removes '.' and '..' segments
    - Restricts characters to A-Za-z0-9, underscore, dash, and '/'
    """
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
