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
"""
Unit test for ``close_connection`` in ``api/db/db_models.py``, the request
teardown.

The teardown returns the current thread's pooled connection. It must not
call the pool's ``close_stale``: that closes the sockets of connections
other threads still hold and leaves them pointing at dead objects, so
their next query raises the client library's closed-socket error
``(0, '')``, which the retrying pool logs as "Database connection issue
(attempt 1/5)" and repairs by reconnecting, once per thread every thirty
seconds of quiet.
"""

import unittest
from unittest.mock import patch

from api.db import db_models
from common import settings


class _RecordingPool:
    """Stands in for the pooled database: records what the teardown asks of it."""

    def __init__(self, closed=False):
        self.calls = []
        self._closed = closed

    def is_closed(self):
        self.calls.append("is_closed")
        return self._closed

    def close(self):
        self.calls.append("close")
        self._closed = True
        return True

    def close_stale(self, age=600):
        self.calls.append(f"close_stale({age})")
        return 0


class CloseConnectionTest(unittest.TestCase):
    def test_the_teardown_returns_its_own_connection_and_leaves_other_threads_alone(self):
        for db_type in ("mysql", "postgres"):
            with self.subTest(db_type):
                pool = _RecordingPool()
                with patch.object(db_models, "DB", pool), patch.object(settings, "DATABASE_TYPE", db_type):
                    db_models.close_connection()
                self.assertIn("close", pool.calls, pool.calls)
                self.assertEqual([c for c in pool.calls if c.startswith("close_stale")], [], pool.calls)

    def test_an_already_returned_connection_is_left_as_it_is(self):
        pool = _RecordingPool(closed=True)
        with patch.object(db_models, "DB", pool), patch.object(settings, "DATABASE_TYPE", "mysql"):
            db_models.close_connection()
        self.assertNotIn("close", pool.calls, pool.calls)


if __name__ == "__main__":
    unittest.main()
