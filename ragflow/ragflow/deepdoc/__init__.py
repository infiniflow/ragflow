# Shim package to make top-level `deepdoc` importable when tests run from inner folder (ragflow/ragflow).
# This file loads the real top-level deepdoc package located at the repository root
# and exposes it under the package name `deepdoc` so tests that run with
# repo/ragflow as the working directory can import deepdoc.parser.* successfully.

import os
import sys
import importlib.util

# Path to the repository root from this file: ../../
PROJECT_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
REAL_DEEPDOC_PATH = os.path.join(PROJECT_ROOT, "deepdoc")

if os.path.isdir(REAL_DEEPDOC_PATH):
    # Ensure package import machinery finds the real deepdoc package submodules
    __path__ = [REAL_DEEPDOC_PATH]

    # Load the real deepdoc.__init__.py into a private module and copy attributes
    real_init = os.path.join(REAL_DEEPDOC_PATH, "__init__.py")
    try:
        spec = importlib.util.spec_from_file_location("_deepdoc_real", real_init)
        module = importlib.util.module_from_spec(spec)
        sys.modules["_deepdoc_real"] = module
        spec.loader.exec_module(module)
        for k, v in module.__dict__.items():
            if k in ("__name__", "__spec__", "__package__", "__path__"):
                continue
            globals()[k] = v
    except Exception:
        # If loading fails, fall back to letting the normal import system handle things.
        pass
