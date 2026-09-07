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

import asyncio
import weakref


class LoopLocalSemaphore:
    """
    Asyncio synchronization primitives bind to the event loop that waits on them.
    Keep one semaphore per running loop for module-level concurrency limiters.
    """

    def __init__(self, value: int):
        self._value = int(value)
        self._semaphores: "weakref.WeakKeyDictionary[asyncio.AbstractEventLoop, asyncio.Semaphore]" = (
            weakref.WeakKeyDictionary()
        )

    def _get(self) -> asyncio.Semaphore:
        loop = asyncio.get_running_loop()
        for cached_loop in list(self._semaphores):
            if cached_loop.is_closed():
                self._semaphores.pop(cached_loop, None)
        sem = self._semaphores.get(loop)
        if sem is None:
            sem = asyncio.Semaphore(self._value)
            self._semaphores[loop] = sem
        return sem

    async def acquire(self) -> bool:
        return await self._get().acquire()

    def release(self) -> None:
        self._get().release()

    async def __aenter__(self):
        await self.acquire()
        return self

    async def __aexit__(self, exc_type, exc, tb):
        self.release()
        return False
