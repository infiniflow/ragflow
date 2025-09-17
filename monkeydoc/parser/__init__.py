"""Parser layer for MonkeyDoc.

Phase 1 exposes the `MonkeyDocPdfParser` with the same public interface
as DeepDoc's `RAGFlowPdfParser`, but returns empty results.
"""

from .pdf_parser import MonkeyDocPdfParser  # noqa: F401

__all__ = [
    "MonkeyDocPdfParser",
]


