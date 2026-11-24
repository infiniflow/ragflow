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
"""SDK unit-test conftest.

The SDK's ``__init__.py`` reads ``importlib.metadata.version("ragflow_sdk")``
to populate ``__version__``. When the SDK is loaded via ``PYTHONPATH`` rather
than as an installed distribution (the unit-test setup), the lookup raises
``PackageNotFoundError`` and aborts every import. Stub ``version()`` before
the SDK modules are loaded so the tests can import them.
"""

import importlib.metadata as _metadata

_original_version = _metadata.version


def _version(name):
    if name == "ragflow_sdk":
        return "0.0.0+test"
    return _original_version(name)


_metadata.version = _version

import sys as _sys

if "sdk/python" not in _sys.path:
    _sys.path.insert(0, "sdk/python")
