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
    # Ensure Python can find the real deepdoc package by adding the repository root
    # to sys.path. Some test runners set PYTHONPATH to the inner folder (ragflow/ragflow)
    # which prevents the real top-level package from being importable. Inserting
    # PROJECT_ROOT ensures imports like `deepdoc.parser` resolve to the package
    # under the repository root.
    # Do NOT modify sys.path at import time; this causes side effects that can
    # affect other packages and test runners. Tests that need the repository root
    # on PYTHONPATH should configure it in their test runner (e.g., pytest.ini or
    # the test command) instead of relying on runtime path mutation.

    # Set package __path__ so submodules (deepdoc.parser) are searched inside
    # the real deepdoc directory. This is sufficient for importlib to resolve
    # submodules when the test runner's PYTHONPATH is configured correctly.
    __path__ = [REAL_DEEPDOC_PATH]

    # Try to load the real deepdoc.__init__.py to copy any top-level attributes
    # (optional). If loading fails, relying on __path__ is sufficient for submodule imports.
    real_init = os.path.join(REAL_DEEPDOC_PATH, "__init__.py")
    try:
        spec = importlib.util.spec_from_file_location("_deepdoc_real", real_init)
        if spec and spec.loader:
            module = importlib.util.module_from_spec(spec)
            sys.modules["_deepdoc_real"] = module
            spec.loader.exec_module(module)
            for k, v in module.__dict__.items():
                if k in ("__name__", "__spec__", "__package__", "__path__"):
                    continue
                globals()[k] = v
    except Exception:
        # If loading the real __init__ fails for any reason, don't raise; the
        # __path__ setting will still allow imports of submodules like deepdoc.parser
        # However, we will log an informational message to help debugging test setup issues.
        try:
            import logging
            logging.getLogger(__name__).info("deepdoc shim could not import real __init__; ensure PYTHONPATH is set to repository root for test execution.")
        except Exception:
            pass
