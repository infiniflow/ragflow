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

import base64
import logging
from functools import partial
from io import BytesIO

from PIL import Image



from common.misc_utils import thread_pool_exec
from rag.utils.lazy_image import open_image_for_processing

test_image_base64 = "iVBORw0KGgoAAAANSUhEUgAAAGQAAABkCAIAAAD/gAIDAAAA6ElEQVR4nO3QwQ3AIBDAsIP9d25XIC+EZE8QZc18w5l9O+AlZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBT+IYAHHLHkdEgAAAABJRU5ErkJggg=="
test_image = base64.b64decode(test_image_base64)


async def image2id(d: dict, storage_put_func: partial, objname: str, bucket: str = "imagetemps"):
    import logging
    from io import BytesIO
    from rag.svr.task_executor_limiter import minio_limiter

    if "image" not in d:
        return
    if not d["image"]:
        del d["image"]
        return

    def encode_image():
        with BytesIO() as buf:
            img, close_after = open_image_for_processing(d["image"], allow_bytes=False)

            if isinstance(img, bytes):
                buf.write(img)
                buf.seek(0)
                return buf.getvalue()

            if not isinstance(img, Image.Image):
                return None

            if img.mode in ("RGBA", "P"):
                orig_img = img
                img = img.convert("RGB")
                if close_after:
                    try:
                        orig_img.close()
                    except Exception:
                        pass

            try:
                img.save(buf, format="JPEG")
                buf.seek(0)
                return buf.getvalue()
            except OSError as e:
                logging.warning(f"Saving image exception: {e}")
                return None
            finally:
                if close_after:
                    try:
                        img.close()
                    except Exception:
                        pass

    jpeg_binary = await thread_pool_exec(encode_image)
    if jpeg_binary is None:
        del d["image"]
        return

    async with minio_limiter:
        await thread_pool_exec(
            lambda: storage_put_func(bucket=bucket, fnm=objname, binary=jpeg_binary)
        )

    d["img_id"] = f"{bucket}-{objname}"

    if not isinstance(d["image"], bytes):
        d["image"].close()
    del d["image"]


def parse_storage_composite_id(composite_id: str) -> tuple[str, str] | None:
    """Split a ``{bucket}-{object_key}`` storage ID on the first hyphen only.

    ``image2id`` stores ``img_id`` as ``f"{bucket}-{objname}"``. The object key
    may contain additional hyphens (e.g. ``page-1.jpg``).

    Args:
        composite_id: Composite storage identifier.

    Returns:
        ``(bucket, object_key)`` when valid, otherwise ``None``.
    """
    parts = composite_id.split("-", 1)
    if len(parts) != 2 or not parts[0] or not parts[1] or composite_id.endswith("-"):
        return None
    return parts[0], parts[1]


def id2image(image_id: str | None, storage_get_func: partial):
    """Load a PIL image from storage using a composite ``img_id``.

    Args:
        image_id: Value produced by ``image2id`` (``{bucket}-{object_key}``).
        storage_get_func: Callable ``(bucket=, fnm=)`` returning raw bytes.

    Returns:
        A PIL ``Image`` instance, or ``None`` when the ID is invalid or load fails.
    """
    if not image_id:
        return
    parsed = parse_storage_composite_id(image_id)
    if not parsed:
        logging.debug("Invalid image_id composite format: %s", image_id)
        return
    bkt, nm = parsed
    try:
        blob = storage_get_func(bucket=bkt, fnm=nm)
        if not blob:
            return
        return Image.open(BytesIO(blob))
    except Exception as e:
        logging.exception(e)
