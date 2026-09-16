#!/usr/bin/env python3
"""Generate minimal stub packages for the OSS DeepDoc Docker image.

The deepdoc vision modules (ocr.py, recognizer.py, etc.) import from
``common``, ``rag``, and ``deepdoc`` at module level.  In the full
RAGFlow environment these packages pull in heavy dependencies (torch,
pdfplumber, database connectors, beartype) that are not needed by the
ONNX-only inference server.

This script writes lightweight replacement modules under /app so the
import chain succeeds without pulling in the full dependency tree.

Why stubs instead of conditionally lazy imports in the vision code?
The vision modules are shared between the full Python backend and the
Docker server.  Keeping the stubs here avoids adding Docker-specific
guards to the shared code.
"""

import os

TARGET = os.environ.get("STUB_TARGET", "/app")


def write(path: str, content: str) -> None:
    full = os.path.join(TARGET, path)
    os.makedirs(os.path.dirname(full), exist_ok=True)
    with open(full, "w") as f:
        f.write(content.lstrip("\n"))


# ── deepdoc ────────────────────────────────────────────────────────────
# Real deepdoc/__init__.py calls beartype_this_package() which requires
# the beartype library.

write(
    "deepdoc/__init__.py",
    """
# Minimal deepdoc __init__ for Docker — avoids beartype dependency.
""",
)

# Real deepdoc/vision/__init__.py imports pdfplumber and
# AscendLayoutRecognizer (requires ais_bench).  The Docker server only
# needs the four ONNX-based classes below.

write(
    "deepdoc/vision/__init__.py",
    """
# Minimal deepdoc.vision __init__ for Docker — avoids pdfplumber and Ascend imports.
from .ocr import OCR
from .recognizer import Recognizer
from .layout_recognizer import LayoutRecognizer4YOLOv10 as LayoutRecognizer
from .table_structure_recognizer import TableStructureRecognizer

__all__ = ["OCR", "Recognizer", "LayoutRecognizer", "TableStructureRecognizer"]
""",
)

# ── common ─────────────────────────────────────────────────────────────
# Real common.settings imports rag.utils.es_conn and other database/storage
# connectors.  The server only needs PARALLEL_DEVICES for OCR.

write(
    "common/__init__.py",
    """
# Stub common.__init__ for Docker deepdoc service.
import os


class _Settings:
    PARALLEL_DEVICES = int(os.environ.get("PARALLEL_DEVICES", "0"))


settings = _Settings()
""",
)

# Real common.file_utils derives the project base from __file__.  In
# Docker the project root is always /app.

write(
    "common/file_utils.py",
    """
# Stub common.file_utils for Docker deepdoc service.
import os

_PROJECT_BASE = None


def get_project_base_directory(*args):
    global _PROJECT_BASE
    if _PROJECT_BASE is None:
        _PROJECT_BASE = os.environ.get("RAGFLOW_PROJECT_BASE", "/app")
    if args:
        return os.path.join(_PROJECT_BASE, *args)
    return _PROJECT_BASE
""",
)

# Real common.misc_utils imports 15+ modules.  The server only calls
# pip_install_torch() inside load_model()'s cuda_is_available() guard.
# On CPU-only images torch is not installed, so the try/except silently
# returns False and onnxruntime falls back to CPUExecutionProvider.

write(
    "common/misc_utils.py",
    """
# Stub common.misc_utils for Docker deepdoc service.


def pip_install_torch(*args, **kwargs):
    try:
        import torch  # noqa: F401
    except ImportError:
        pass
""",
)

# ── rag ────────────────────────────────────────────────────────────────

write(
    "rag/__init__.py",
    """
# Stub rag package for Docker deepdoc service.
""",
)

# table_structure_recognizer.py imports rag_tokenizer at module level.
# Its tokenize/tag methods are only called from blockType() /
# construct_table(), which are NOT invoked by the TSR adapter's
# __call__() path.  The stub exists solely to satisfy the module-level
# import; its methods are never called at server runtime.

write(
    "rag/nlp/__init__.py",
    """
# Stub rag.nlp module for Docker deepdoc service.
# Provides minimal rag_tokenizer to satisfy table_structure_recognizer import.


class _StubTokenizer:
    def tokenize(self, text):
        return text

    def tag(self, word):
        return ""


rag_tokenizer = _StubTokenizer()
""",
)

# operators.py imports ensure_pil_image at module level and calls it in
# NormalizeImage.__call__ / ToCHWImage.__call__ (OCR text detection path).
# The real rag.utils.lazy_image imports concat_img from rag.nlp, pulling
# in the entire NLP stack.

write(
    "rag/utils/lazy_image.py",
    """
# Stub rag.utils.lazy_image for Docker.
from PIL import Image


def ensure_pil_image(img):
    if isinstance(img, Image.Image):
        return img
    return None
""",
)


if __name__ == "__main__":
    print(f"Docker stubs written to {TARGET}")
