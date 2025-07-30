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