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

import os
import queue
import threading
from typing import Any, Callable, Coroutine, Optional, Type, Union
import asyncio
from functools import wraps
from quart import make_response, jsonify
from common.constants import RetCode

TimeoutException = Union[Type[BaseException], BaseException]
OnTimeoutCallback = Union[Callable[..., Any], Coroutine[Any, Any, Any]]

# Bound sync @timeout worker threads to avoid unbounded growth when timed-out
# work keeps running (Python cannot kill threads).
_timeout_sem: Optional[threading.BoundedSemaphore] = None
_timeout_sem_lock = threading.Lock()


def _timeout_thread_limit() -> int:
    raw = os.getenv("TIMEOUT_THREAD_MAX_WORKERS") or os.getenv("THREAD_POOL_MAX_WORKERS", "128")
    try:
        n = int(raw)
    except ValueError:
        n = 128
    return max(n, 1)


def _get_timeout_sem() -> threading.BoundedSemaphore:
    global _timeout_sem
    if _timeout_sem is None:
        with _timeout_sem_lock:
            if _timeout_sem is None:
                _timeout_sem = threading.BoundedSemaphore(_timeout_thread_limit())
    return _timeout_sem


def timeout(seconds: float | int | str = None, attempts: int = 2, *, exception: Optional[TimeoutException] = None, on_timeout: Optional[OnTimeoutCallback] = None):
    if isinstance(seconds, str):
        seconds = float(seconds)

    def decorator(func):
        @wraps(func)
        def wrapper(*args, **kwargs):
            result_queue = queue.Queue(maxsize=1)
            sem = _get_timeout_sem()
            enable_timeout = bool(os.environ.get("ENABLE_TIMEOUT_ASSERTION"))
            acquire_timeout = seconds if enable_timeout and seconds is not None else None
            if not sem.acquire(timeout=acquire_timeout):
                raise TimeoutError(
                    f"Function '{func.__name__}' timed out after {seconds} seconds waiting for a timeout worker slot."
                )

            def target():
                try:
                    result = func(*args, **kwargs)
                    result_queue.put(result)
                except Exception as e:
                    result_queue.put(e)
                finally:
                    sem.release()

            thread = threading.Thread(target=target, daemon=True)
            try:
                thread.start()
            except Exception:
                sem.release()
                raise

            for _ in range(attempts):
                try:
                    if enable_timeout:
                        result = result_queue.get(timeout=seconds)
                    else:
                        result = result_queue.get()
                    if isinstance(result, Exception):
                        raise result
                    return result
                except queue.Empty:
                    pass
            raise TimeoutError(f"Function '{func.__name__}' timed out after {seconds} seconds and {attempts} attempts.")

        @wraps(func)
        async def async_wrapper(*args, **kwargs) -> Any:
            if seconds is None:
                return await func(*args, **kwargs)

            for a in range(attempts):
                try:
                    if os.environ.get("ENABLE_TIMEOUT_ASSERTION"):
                        return await asyncio.wait_for(func(*args, **kwargs), timeout=seconds)
                    else:
                        return await func(*args, **kwargs)
                except asyncio.TimeoutError:
                    if a < attempts - 1:
                        continue
                    if on_timeout is not None:
                        if callable(on_timeout):
                            result = on_timeout()
                            if isinstance(result, Coroutine):
                                return await result
                            return result
                        return on_timeout

                    if exception is None:
                        raise TimeoutError(f"Operation timed out after {seconds} seconds and {attempts} attempts.")

                    if isinstance(exception, BaseException):
                        raise exception

                    if isinstance(exception, type) and issubclass(exception, BaseException):
                        raise exception(f"Operation timed out after {seconds} seconds and {attempts} attempts.")

                    raise RuntimeError("Invalid exception type provided")

        if asyncio.iscoroutinefunction(func):
            return async_wrapper
        return wrapper

    return decorator


async def construct_response(code=RetCode.SUCCESS, message="success", data=None, auth=None):
    result_dict = {"code": code, "message": message, "data": data}
    response_dict = {}
    for key, value in result_dict.items():
        if value is None and key != "code":
            continue
        else:
            response_dict[key] = value
    response = await make_response(jsonify(response_dict))
    if auth:
        response.headers["Authorization"] = auth
    response.headers["Access-Control-Allow-Origin"] = "*"
    response.headers["Access-Control-Allow-Method"] = "*"
    response.headers["Access-Control-Allow-Headers"] = "*"
    response.headers["Access-Control-Expose-Headers"] = "Authorization"
    return response


def sync_construct_response(code=RetCode.SUCCESS, message="success", data=None, auth=None):
    import flask

    result_dict = {"code": code, "message": message, "data": data}
    response_dict = {}
    for key, value in result_dict.items():
        if value is None and key != "code":
            continue
        else:
            response_dict[key] = value
    response = flask.make_response(flask.jsonify(response_dict))
    if auth:
        response.headers["Authorization"] = auth
    response.headers["Access-Control-Allow-Origin"] = "*"
    response.headers["Access-Control-Allow-Method"] = "*"
    response.headers["Access-Control-Allow-Headers"] = "*"
    response.headers["Access-Control-Expose-Headers"] = "Authorization"
    return response
