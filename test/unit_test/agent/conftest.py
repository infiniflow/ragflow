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
"""Test isolation shims for ``test/unit_test/agent``.

Both ``agent.tools`` and ``agent.component`` use an ``__init__.py`` that
auto-imports every sibling ``.py`` so their loader can populate
``__all_classes``. That side effect pulls in unrelated third-party libraries
(``scholarly``, ``xgboost``, ``pymysql``, …), some of which raise
``SyntaxError`` under Python 3.12, and aborts pytest collection long before
the tool we're actually trying to test is loaded.

This conftest pre-registers no-op package stubs that share the real
``__path__`` so subsequent ``from agent.tools.<name> import ...`` calls find
and load only the specific submodule, skipping the auto-discovery scan.
The stubs are only installed when the real packages haven't been imported
yet, so this is a no-op in environments where the full app already loaded.
"""
import os
import sys
import types

_AGENT_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", "..", "agent"))

# Make sure the parent ``agent`` package itself is loaded (its __init__.py is
# empty / side-effect-free) so we can hang the stub submodules off it.
import agent  # noqa: E402,F401


def _stub_package(dotted_name: str, package_dir: str) -> None:
    if dotted_name in sys.modules:
        return
    pkg = types.ModuleType(dotted_name)
    pkg.__path__ = [package_dir]
    sys.modules[dotted_name] = pkg
    # Mirror what Python does for real packages: bind the child on its parent
    # so attribute-style lookups (e.g. pytest monkeypatch's dotted-path
    # resolution) work.
    parent_name, _, child_name = dotted_name.rpartition(".")
    if parent_name and parent_name in sys.modules:
        setattr(sys.modules[parent_name], child_name, pkg)


_stub_package("agent.tools", os.path.join(_AGENT_ROOT, "tools"))
_stub_package("agent.component", os.path.join(_AGENT_ROOT, "component"))
