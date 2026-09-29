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
"""
Conftest for admin unit tests.

The admin package is invoked as a script (`python admin/server/admin_server.py`)
and its internal modules use top-level imports like `from config import
SERVICE_CONFIGS`. To make those modules importable from pytest, we prepend
``admin/server`` to ``sys.path`` for the duration of the test session.
"""

import os
import sys

_ADMIN_SERVER = os.path.join(
    os.path.dirname(os.path.abspath(__file__)),
    "..",
    "..",
    "..",
    "admin",
    "server",
)
_ADMIN_SERVER = os.path.normpath(_ADMIN_SERVER)
if _ADMIN_SERVER not in sys.path:
    sys.path.insert(0, _ADMIN_SERVER)
