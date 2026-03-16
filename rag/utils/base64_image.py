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

test_image_base64 = "iVBORw0KGgoAAAANSUhEUgAAAGQAAABkCAIAAAD/gAIDAAAA6ElEQVR4nO3QwQ3AIBDAsIP9d25XIC+EZE8QZc18w5l9O+AlZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBWYFZgVmBT+IYAHHLHkdEgAAAABJRU5ErkJggg=="
test_image = base64.b64decode(test_image_base64)


async def image2id(d: dict, storage_put_func: partial, objname: str, bucket: str = "imagetemps"):
    import logging
    from io import BytesIO
    from rag.svr.task_executor import minio_limiter

    if "image" not in d:
        return
    if not d["image"]:
        del d["image"]
        return

    def encode_image():
        with BytesIO() as buf:
            img = d["image"]

            if isinstance(img, bytes):
                buf.write(img)
                buf.seek(0)
                return buf.getvalue()

            if img.mode in ("RGBA", "P"):
                img = img.convert("RGB")

            try:
                img.save(buf, format="JPEG")
            except OSError as e:
                logging.warning(f"Saving image exception: {e}")
                return None

            buf.seek(0)
            return buf.getvalue()

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


def id2image(image_id: str | None, storage_get_func: partial):
    if not image_id:
        return
    arr = image_id.split("-")
    if len(arr) != 2:
        return
    bkt, nm = image_id.split("-")
    try:
        blob = storage_get_func(bucket=bkt, fnm=nm)
        if not blob:
            return
        return Image.open(BytesIO(blob))
    except Exception as e:
        logging.exception(e)
