#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

from typing import Optional, Literal

from pydantic import BaseModel, Field


class RAGConfig(BaseModel):
    device: Literal["cpu", "gpu"] = "cpu"

    embedding_batch_size: int = Field(default=16)

    ocr_gpu_mem_limit_mb: int = Field(default=2048)
    ocr_arena_extend_strategy: str = Field(default="kNextPowerOfTwo")

    parallel_devices: int = Field(default=0)
    enable_timeout_assertion: Optional[bool] = Field(default=None)

    # Document & File
    doc_bulk_size: int = Field(default=4)
    doc_maximum_size: int = Field(default=128 * 1024 * 1024)
    max_file_num_per_user: int = Field(
        default=0,
        description="Max number of files per user, 0 means unlimited"
    )

    tei_model: str = Field(default="Qwen/Qwen3-Embedding-0.6B")
    tei_port: int = Field(default=6380)
    teo_host: str = Field(default="tei")