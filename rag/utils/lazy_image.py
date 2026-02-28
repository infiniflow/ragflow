import logging
from io import BytesIO

from PIL import Image

from rag.nlp import concat_img


class LazyDocxImage:
    def __init__(self, blobs, source=None):
        self._blobs = [b for b in (blobs or []) if b]
        self.source = source
        self._pil = None

    def __bool__(self):
        return bool(self._blobs)

    def to_pil(self):
        if self._pil is not None:
            try:
                self._pil.load()
                return self._pil
            except Exception:
                try:
                    self._pil.close()
                except Exception:
                    pass
                self._pil = None
        res_img = None
        for blob in self._blobs:
            try:
                image = Image.open(BytesIO(blob)).convert("RGB")
            except Exception as e:
                logging.info(f"LazyDocxImage: skip bad image blob: {e}")
                continue

            if res_img is None:
                res_img = image
                continue

            new_img = concat_img(res_img, image)
            if new_img is not res_img:
                try:
                    res_img.close()
                except Exception:
                    pass
            try:
                image.close()
            except Exception:
                pass
            res_img = new_img

        self._pil = res_img
        return self._pil

    def to_pil_detached(self):
        pil = self.to_pil()
        self._pil = None
        return pil

    def close(self):
        if self._pil is not None:
            try:
                self._pil.close()
            except Exception:
                pass
            self._pil = None
        return None

    def __getattr__(self, name):
        pil = self.to_pil()
        if pil is None:
            raise AttributeError(name)
        return getattr(pil, name)

    def __array__(self, dtype=None):
        import numpy as np

        pil = self.to_pil()
        if pil is None:
            return np.array([], dtype=dtype)
        return np.array(pil, dtype=dtype)

    def __enter__(self):
        return self.to_pil()

    def __exit__(self, exc_type, exc, tb):
        self.close()
        return False


def ensure_pil_image(img):
    if isinstance(img, Image.Image):
        return img
    if isinstance(img, LazyDocxImage):
        return img.to_pil()
    return None


def is_image_like(img):
    return isinstance(img, Image.Image) or isinstance(img, LazyDocxImage)


def open_image_for_processing(img, *, allow_bytes=False):
    if isinstance(img, Image.Image):
        return img, False
    if isinstance(img, LazyDocxImage):
        return img.to_pil_detached(), True
    if allow_bytes and isinstance(img, (bytes, bytearray)):
        try:
            pil = Image.open(BytesIO(img)).convert("RGB")
            return pil, True
        except Exception as e:
            logging.info(f"open_image_for_processing: bad bytes: {e}")
            return None, False
    return img, False
