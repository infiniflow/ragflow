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

import importlib
import inspect
import pkgutil
from pathlib import Path
from types import ModuleType
from typing import Dict, Type

__all_classes: Dict[str, Type] = {}

_pkg_dir = Path(__file__).resolve().parent
_pkg_name = __name__


def _should_skip_module(mod_name: str) -> bool:
    leaf = mod_name.rsplit(".", 1)[-1]
    return leaf in {"__init__"} or leaf.startswith("__") or leaf.startswith("_") or leaf.startswith("base")


def _import_submodules() -> None:
    for modinfo in pkgutil.walk_packages([str(_pkg_dir)], prefix=_pkg_name + "."):  # noqa: F821
        mod_name = modinfo.name
        if _should_skip_module(mod_name):  # noqa: F821
            continue
        try:
            module = importlib.import_module(mod_name)
            _extract_classes_from_module(module)  # noqa: F821
        except ImportError as e:
            print(f"Warning: Failed to import module {mod_name}: {e}")


def _extract_classes_from_module(module: ModuleType) -> None:
    for name, obj in inspect.getmembers(module):
        if inspect.isclass(obj) and obj.__module__ == module.__name__ and not name.startswith("_"):
            __all_classes[name] = obj
            globals()[name] = obj


_import_submodules()

__all__ = list(__all_classes.keys()) + ["__all_classes"]

del _pkg_dir, _pkg_name, _import_submodules, _extract_classes_from_module
