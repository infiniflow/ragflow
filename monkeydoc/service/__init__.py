"""Service layer for MonkeyDoc.

Contains runtime model/service abstractions. Phase 1 exposes only a
placeholder service with a minimal lifecycle and no ML calls yet.
"""

from .model_service import MonkeyOCRService  # noqa: F401

__all__ = [
    "MonkeyOCRService",
]


