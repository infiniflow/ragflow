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

"""Restore the real ``common.data_source`` package before importing rag unit tests.

``test/unit_test/data_source/conftest.py`` registers a lightweight
``sys.modules["common.data_source"]`` stub so submodule imports skip the heavy
package ``__init__.py``. Pytest collection order visits ``data_source/`` before
``rag/``, so without this hook ``rag.svr.sync_data_source`` fails on
``from common.data_source import BlobStorageConnector``.
"""

from __future__ import annotations

import importlib
import sys
import types


def _restore_common_data_source_package() -> None:
    mod = sys.modules.get("common.data_source")
    if mod is None:
        return
    # Stub is a bare types.ModuleType with __path__ and no __file__; real package has __init__.py.
    if getattr(mod, "__file__", None) is not None:
        return
    if not isinstance(mod, types.ModuleType) or not getattr(mod, "__path__", None):
        return
    keys = [
        key
        for key in sys.modules
        if key == "common.data_source" or key.startswith("common.data_source.")
    ]
    for key in keys:
        del sys.modules[key]
    importlib.invalidate_caches()
    try:
        importlib.import_module("common.data_source")
    except Exception as exc:  # pragma: no cover
        raise ImportError(
            "conftest: failed to restore real common.data_source package"
        ) from exc


_restore_common_data_source_package()
