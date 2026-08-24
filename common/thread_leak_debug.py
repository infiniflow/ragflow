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

"""Opt-in diagnostics for OS-thread leaks.

Enable with RAGFLOW_DEBUG_THREAD_LEAK=1. When enabled, every thread start is
logged together with the stack that spawned it, and dump_alive_threads() can be
called at lifecycle boundaries (e.g. canvas teardown) to log what is still
running. Disabled by default with zero overhead.
"""

import logging
import os
import sys
import threading
import traceback

logger = logging.getLogger(__name__)

_ENABLED = os.getenv("RAGFLOW_DEBUG_THREAD_LEAK", "").strip().lower() in ("1", "true", "yes")
_orig_thread_start = None


def enabled() -> bool:
    return _ENABLED


def install() -> None:
    """Patch Thread.start to log every spawned thread with its creation stack."""
    global _orig_thread_start
    if not _ENABLED or _orig_thread_start is not None:
        return

    _orig_thread_start = threading.Thread.start

    def traced_start(self):
        stack = "".join(traceback.format_stack(sys._getframe(1)))
        logger.warning("[thread-leak] start name=%s daemon=%s\n%s", self.name, self.daemon, stack)
        return _orig_thread_start(self)

    threading.Thread.start = traced_start
    logger.warning("[thread-leak] diagnostics installed")


def dump_alive_threads(tag: str) -> None:
    if not _ENABLED:
        return
    threads = threading.enumerate()
    names = ", ".join(sorted(f"{t.name}{'(daemon)' if t.daemon else ''}" for t in threads))
    logger.warning("[thread-leak] alive=%d at %s: %s", len(threads), tag, names)
