# Stub rag.utils.lazy_image for Docker.
from PIL import Image


def ensure_pil_image(img):
    """Return img as a PIL Image, or None if not convertible.

    Matches the real rag.utils.lazy_image.ensure_pil_image behavior:
    - PIL Image → return as-is
    - LazyImage → return img.to_pil() (not available in Docker stub)
    - numpy array or other → return None (caller handles numpy directly)
    """
    if isinstance(img, Image.Image):
        return img
    return None
