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
import time
import uuid

from rag.utils.redis_conn import REDIS_CONN


class RedisDistributedLock:
    def __init__(self, lock_key, timeout=10):
        self.lock_key = lock_key
        self.lock_value = str(uuid.uuid4())
        self.timeout = timeout

    @staticmethod
    def clean_lock(lock_key):
        REDIS_CONN.REDIS.delete(lock_key)

    def acquire_lock(self):
        end_time = time.time() + self.timeout
        while time.time() < end_time:
            if REDIS_CONN.REDIS.setnx(self.lock_key, self.lock_value):
                return True
            time.sleep(1)
        return False

    def release_lock(self):
        if REDIS_CONN.REDIS.get(self.lock_key) == self.lock_value:
            REDIS_CONN.REDIS.delete(self.lock_key)

    def __enter__(self):
        self.acquire_lock()

    def __exit__(self, exception_type, exception_value, exception_traceback):
        self.release_lock()