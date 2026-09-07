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
import functools
import inspect
import logging
import os
import time


def singleton(cls, *args, **kw):
    instances = {}

    def _singleton():
        key = str(cls) + str(os.getpid())
        if key not in instances:
            instances[key] = cls(*args, **kw)
        return instances[key]

    return _singleton


def timing(func=None, *, name=None, context=None):
    """Decorator that records function execution time.

    Usage:
        @timing
        async def my_func(): ...

        @timing(name="custom_name")
        def my_func(): ...

        @timing(context=recording_ctx)
        async def my_func(): ...

    Args:
        func: The function to decorate (auto-passed when used as @timing)
        name: Custom name for the timing record, defaults to function name
        context: A RecordingContext-like object to record timing data into.
                 If not provided, will try to use global recording_context from task_executor.
                 Timing data will be recorded as "{name}_time".
    """
    if func is None:
        return functools.partial(timing, name=name, context=context)

    func_name = name or func.__name__
    log = logging.getLogger(__name__)

    if inspect.iscoroutinefunction(func):
        @functools.wraps(func)
        async def async_wrapper(*args, **kwargs):
            start = time.perf_counter()
            try:
                result = await func(*args, **kwargs)
                return result
            finally:
                elapsed = time.perf_counter() - start
                log.debug(f"[TIMING] {func_name} took {elapsed:.3f}s")
                if context is not None:
                    context.record(f"{func_name}_time", elapsed)
        return async_wrapper
    else:
        @functools.wraps(func)
        def sync_wrapper(*args, **kwargs):
            start = time.perf_counter()
            try:
                result = func(*args, **kwargs)
                return result
            finally:
                elapsed = time.perf_counter() - start
                log.debug(f"[TIMING] {func_name} took {elapsed:.3f}s")
                if context is not None:
                    context.record(f"{func_name}_time", elapsed)
        return sync_wrapper