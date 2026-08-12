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

import importlib
import sys
from contextlib import contextmanager

import pytest

_MISSING = object()
_DEEPDOC_PACKAGE = "deepdoc"
_PARSER_PACKAGE = "deepdoc.parser"
_PDF_PARSER_MODULE = f"{_PARSER_PACKAGE}.pdf_parser"


def _parser_modules():
    return {name: module for name, module in sys.modules.items() if name == _PARSER_PACKAGE or name.startswith(f"{_PARSER_PACKAGE}.")}


def _clear_parser_modules():
    for name in list(sys.modules):
        if name == _PARSER_PACKAGE or name.startswith(f"{_PARSER_PACKAGE}."):
            del sys.modules[name]


@contextmanager
def _production_parser_package():
    """Replace the rag test PDF stub while a DeepDoc test imports or runs."""
    pdf_parser = sys.modules.get(_PDF_PARSER_MODULE)
    if pdf_parser is None or getattr(pdf_parser, "__file__", None) is not None:
        yield
        return

    deepdoc_package = sys.modules.get(_DEEPDOC_PACKAGE)
    previous_parser_attribute = getattr(deepdoc_package, "parser", _MISSING) if deepdoc_package is not None else _MISSING
    previous = _parser_modules()
    _clear_parser_modules()
    importlib.invalidate_caches()
    importlib.import_module(_PARSER_PACKAGE)
    try:
        yield
    finally:
        _clear_parser_modules()
        sys.modules.update(previous)
        if deepdoc_package is not None:
            if previous_parser_attribute is _MISSING:
                if hasattr(deepdoc_package, "parser"):
                    delattr(deepdoc_package, "parser")
            else:
                deepdoc_package.parser = previous_parser_attribute


@pytest.hookimpl(hookwrapper=True)
def pytest_make_collect_report(collector):
    if not isinstance(collector, pytest.Module):
        yield
        return

    with _production_parser_package():
        yield


@pytest.fixture(autouse=True)
def production_parser_package():
    with _production_parser_package():
        yield
