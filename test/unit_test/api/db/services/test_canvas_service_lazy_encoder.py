#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#

"""Cycle 83: pin ``canvas_service`` to the canonical ``get_encoder`` helper.

The cycle 73/74 audit established ``common.token_utils.get_encoder`` as the
single entry point for the cl100k_base encoder so the BPE table loads once
per process and every consumer gets the same cached object. ``canvas_service``
was a straggler — ``completion_openai`` was calling
``tiktoken.get_encoding("cl100k_base")`` directly. These tests pin the
post-fix contract so the straggler cannot drift back.
"""

import ast
from pathlib import Path


CANVAS_SERVICE_PATH = Path("api/db/services/canvas_service.py")


def _module_source() -> str:
    return CANVAS_SERVICE_PATH.read_text(encoding="utf-8")


def _module_tree() -> ast.Module:
    return ast.parse(_module_source())


class TestCanvasServiceUsesCanonicalEncoderHelper:
    def setup_method(self):
        self.tree = _module_tree()
        self.source = _module_source()

    def test_no_direct_tiktoken_import(self):
        """The module must not import tiktoken — the canonical helper is used instead."""
        for node in ast.walk(self.tree):
            if isinstance(node, ast.Import):
                for alias in node.names:
                    assert not alias.name.startswith("tiktoken"), (
                        f"direct `import tiktoken` in canvas_service is forbidden; use `from common.token_utils import get_encoder` (found at line {node.lineno})"
                    )
            if isinstance(node, ast.ImportFrom):
                assert node.module != "tiktoken", f"direct `from tiktoken import ...` in canvas_service is forbidden; use `from common.token_utils import get_encoder` (found at line {node.lineno})"

    def test_get_encoder_is_imported_from_common_token_utils(self):
        """The module must import ``get_encoder`` from ``common.token_utils``."""
        found = False
        for node in ast.walk(self.tree):
            if isinstance(node, ast.ImportFrom):
                if node.module == "common.token_utils":
                    for alias in node.names:
                        if alias.name == "get_encoder":
                            found = True
        assert found, "canvas_service must import `get_encoder` from `common.token_utils`; this is the canonical helper introduced in the cycle 73/74 audit"

    def test_no_direct_tiktoken_get_encoding_call(self):
        """The module must not call ``tiktoken.get_encoding(...)`` directly.

        All ``cl100k_base`` access goes through ``get_encoder()``.
        """
        offenders: list[tuple[int, str]] = []
        for node in ast.walk(self.tree):
            if not isinstance(node, ast.Call):
                continue
            func = node.func
            # Match both ``tiktoken.get_encoding(...)`` and
            # ``tiktoken.encoding_for_model(...)`` shapes.
            if isinstance(func, ast.Attribute) and isinstance(func.value, ast.Name) and func.value.id == "tiktoken":
                offenders.append((node.lineno, ast.unparse(node).split("\n")[0]))

        assert not offenders, "direct tiktoken.* call(s) in canvas_service are forbidden; use `get_encoder()` from common.token_utils:\n" + "\n".join(
            f"  line {ln}: {snippet}" for ln, snippet in offenders
        )

    def test_completion_openai_references_get_encoder(self):
        """The ``completion_openai`` function body must bind the encoder via ``get_encoder()``."""
        target = None
        for node in ast.walk(self.tree):
            if isinstance(node, ast.AsyncFunctionDef) and node.name == "completion_openai":
                target = node
                break
        assert target is not None, "completion_openai function not found in canvas_service"

        saw_get_encoder_call = False
        for node in ast.walk(target):
            if isinstance(node, ast.Call) and isinstance(node.func, ast.Name) and node.func.id == "get_encoder":
                saw_get_encoder_call = True
                break
        assert saw_get_encoder_call, "completion_openai must call `get_encoder()` to bind the cl100k_base encoder"
