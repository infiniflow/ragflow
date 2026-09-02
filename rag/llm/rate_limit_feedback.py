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
import inspect
import logging
from contextvars import ContextVar
from typing import Awaitable, Callable


RateLimitReporter = Callable[[], Awaitable[None] | None]
current_rate_limit_reporter: ContextVar[RateLimitReporter | None] = ContextVar("current_rate_limit_reporter", default=None)


async def report_rate_limit() -> None:
    reporter = current_rate_limit_reporter.get()
    if reporter is None:
        return
    try:
        result = reporter()
        if inspect.isawaitable(result):
            await result
    except Exception:
        logging.exception("LLM rate-limit feedback reporter failed")
