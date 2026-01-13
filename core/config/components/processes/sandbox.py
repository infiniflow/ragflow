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

from pydantic import BaseModel, Field


class SandboxConfig(BaseModel):
    enabled: bool = Field(default=True)
    host: str = Field(default="sandbox-executor-manager")
    max_memory: str = Field(default="256m", description="b, k, m, g")
    timeout: str = Field(default="10s", description="Timeout in seconds, s, m, e.g. 1m30s")
    base_python_image: str = Field(default="sandbox-base-python:latest")
    base_nodejs_image: str = Field(default="sandbox-base-nodejs:latest")
    executor_manager_port: int = Field(default=9385)
    executor_manager_pool_size: int = Field(default=3)
    enable_seccomp: bool = Field(default=False)
