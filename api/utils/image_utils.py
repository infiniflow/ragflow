#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

from io import BytesIO

from PIL import Image

from common import settings

IMAGE_UPDATE_MODE_APPEND = "append"
IMAGE_UPDATE_MODE_REPLACE = "replace"
IMAGE_UPDATE_MODE_REMOVE = "remove"
IMAGE_UPDATE_MODES = frozenset(
    {
        IMAGE_UPDATE_MODE_APPEND,
        IMAGE_UPDATE_MODE_REPLACE,
        IMAGE_UPDATE_MODE_REMOVE,
    }
)


def store_chunk_image(bucket, name, image_binary, mode=IMAGE_UPDATE_MODE_APPEND):
    if mode not in (IMAGE_UPDATE_MODE_APPEND, IMAGE_UPDATE_MODE_REPLACE):
        raise ValueError(f"unsupported image update mode for store: {mode}")

    if mode == IMAGE_UPDATE_MODE_REPLACE or not settings.STORAGE_IMPL.obj_exist(bucket, name):
        settings.STORAGE_IMPL.put(bucket, name, image_binary)
        return

    old_binary = settings.STORAGE_IMPL.get(bucket, name)
    old_img = Image.open(BytesIO(old_binary))
    new_img = Image.open(BytesIO(image_binary))
    old_img = old_img.convert("RGB")
    new_img = new_img.convert("RGB")
    width = max(old_img.width, new_img.width)
    height = old_img.height + new_img.height
    combined = Image.new("RGB", (width, height), (255, 255, 255))
    combined.paste(old_img, (0, 0))
    combined.paste(new_img, (0, old_img.height))
    buf = BytesIO()
    combined.save(buf, format="JPEG")
    settings.STORAGE_IMPL.put(bucket, name, buf.getvalue())


def remove_chunk_image(bucket, name):
    if settings.STORAGE_IMPL.obj_exist(bucket, name):
        settings.STORAGE_IMPL.rm(bucket, name)
