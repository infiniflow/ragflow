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

# Break the redis_conn <-> common.settings import cycle the same way runtime does.
import common.settings  # noqa: F401

from rag.utils.redis_conn import _total_system_memory_human


class TestTotalSystemMemoryHuman:
    def test_prefers_total_system_memory_human(self):
        info = {
            "total_system_memory_human": "16.00G",
            "maxmemory_human": "4.00G",
        }
        assert _total_system_memory_human(info) == "16.00G"

    def test_falls_back_to_maxmemory_human(self):
        info = {
            "used_memory_human": "128.00M",
            "maxmemory_human": "4.00G",
        }
        assert _total_system_memory_human(info) == "4.00G"

    def test_empty_when_neither_field_present(self):
        assert _total_system_memory_human({"used_memory_human": "128.00M"}) == ""
