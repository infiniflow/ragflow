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
import importlib
import inspect
from types import ModuleType
from typing import Dict, Type

_package_path = os.path.dirname(__file__)
__all_classes: Dict[str, Type] = {}

def _import_submodules() -> None:
    for filename in os.listdir(_package_path): # noqa: F821
        if filename.startswith("__") or not filename.endswith(".py") or filename.startswith("base"):
            continue
        module_name = filename[:-3]

        try:
            module = importlib.import_module(f".{module_name}", package=__name__)
            _extract_classes_from_module(module)  # noqa: F821
        except ImportError as e:
            print(f"Warning: Failed to import module {module_name}: {str(e)}")

def _extract_classes_from_module(module: ModuleType) -> None:
    for name, obj in inspect.getmembers(module):
        if (inspect.isclass(obj) and
                obj.__module__ == module.__name__ and not name.startswith("_")):
            __all_classes[name] = obj
            globals()[name] = obj

_import_submodules()

__all__ = list(__all_classes.keys()) + ["__all_classes"]

del _package_path, _import_submodules, _extract_classes_from_module


def component_class(class_name):
    for module_name in ["agent.component", "agent.tools", "rag.flow"]:
        try:
            return getattr(importlib.import_module(module_name), class_name)
        except Exception:
            # logging.warning(f"Can't import module: {module_name}, error: {e}")
            pass
    assert False, f"Can't import {class_name}"
