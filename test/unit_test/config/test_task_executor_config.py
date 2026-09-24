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

from core.config import AppConfig


def test_task_executor_defaults(monkeypatch):
    monkeypatch.delenv("TASK_EXECUTOR__MESSAGE_QUEUE_TYPE", raising=False)
    te_cfg = AppConfig().task_executor
    assert te_cfg.message_queue_type == "redis"
