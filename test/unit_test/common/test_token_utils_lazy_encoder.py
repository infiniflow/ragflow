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

import hashlib
import importlib
import json
import os
import subprocess
import sys
from pathlib import Path

import pytest
import tiktoken

import common


def _reimport_token_utils(monkeypatch, get_encoding):
    """Import common.token_utils fresh with tiktoken.get_encoding replaced.

    Re-importing also rebinds `token_utils` as an attribute of the `common`
    package, and restoring the sys.modules entry does not undo that. Registering
    the attribute with monkeypatch restores it at teardown, so later tests that
    read `common.token_utils` do not get the module built against a fake encoder.
    """
    monkeypatch.setattr(common, "token_utils", getattr(common, "token_utils", None), raising=False)
    # Importing re-runs _ensure_tiktoken_cache, which rewrites TIKTOKEN_CACHE_DIR.
    # Register it first so teardown puts back whatever the suite was using.
    monkeypatch.setenv("TIKTOKEN_CACHE_DIR", os.environ.get("TIKTOKEN_CACHE_DIR", ""))
    monkeypatch.setattr(tiktoken, "get_encoding", get_encoding)
    monkeypatch.delitem(sys.modules, "common.token_utils", raising=False)
    return importlib.import_module("common.token_utils")


class TestLazyEncoder:
    def test_import_does_not_build_the_encoder(self, monkeypatch):
        """Importing the module must not read or download the BPE table.

        `rag.nlp` imports `num_tokens_from_string` from here, so an import-time
        build made every module in that chain depend on reaching
        openaipublic.blob.core.windows.net, including tests that never tokenize.
        """
        calls = []
        module = _reimport_token_utils(monkeypatch, lambda name: calls.append(name))

        assert calls == []
        assert module.num_tokens_from_string is not None

    def test_get_encoder_delegates_to_tiktoken(self, monkeypatch):
        """`get_encoder()` asks tiktoken for cl100k_base on every call.

        Sharing is tiktoken's registry cache (`tiktoken.registry.ENCODINGS`), not
        this module's. Recording every requested name pins both halves: each call
        reaches tiktoken, and nothing here short-circuits the second one.
        """
        shared = tiktoken.Encoding(
            name="test",
            pat_str=r".",
            mergeable_ranks={b"a": 0},
            special_tokens={},
        )
        requested = []

        def fake_get_encoding(name):
            requested.append(name)
            return shared

        module = _reimport_token_utils(monkeypatch, fake_get_encoding)
        assert requested == []

        assert module.get_encoder() is shared
        assert module.get_encoder() is shared
        assert requested == ["cl100k_base", "cl100k_base"]

    def test_unavailable_table_is_not_reported_as_zero_tokens(self, monkeypatch):
        """A missing BPE table must fail loudly, not count as an empty string.

        `num_tokens_from_string` guards `encode` with `except Exception: return 0`.
        Building the encoder inside that guard would turn an unreachable table into
        a silent zero-token result for every caller.
        """

        def unavailable(name):
            raise RuntimeError("BPE table unavailable")

        module = _reimport_token_utils(monkeypatch, unavailable)

        with pytest.raises(RuntimeError, match="BPE table unavailable"):
            module.num_tokens_from_string("hello")


class TestImportChain:
    def test_rag_nlp_and_rag_prompts_import_without_the_bpe_table(self):
        """Importing `rag.nlp` and `rag.prompts` must not load the BPE table.

        The tests above re-import `common.token_utils` on its own, so they would
        still pass if an import-time build came back higher up the chain, for
        example in `rag/prompts/generator.py`, which imports `get_encoder` and is
        pulled in by `rag/prompts/__init__.py`. A fresh interpreter covers the
        whole chain. The loader is the patch point: every way to build an
        encoding (`tiktoken.get_encoding`, `tiktoken.encoding_for_model`,
        `tiktoken.registry.get_encoding`) ends in
        `tiktoken.load.load_tiktoken_bpe`, which `tiktoken_ext.openai_public`
        binds on first build, so patching one entry point would let the others
        through.
        """
        code = """
import json
import tiktoken.load

requested = []


def unreachable(tiktoken_bpe_file, expected_hash=None):
    requested.append(tiktoken_bpe_file)
    raise OSError("BPE table unreachable")


tiktoken.load.load_tiktoken_bpe = unreachable

import rag.nlp
import rag.prompts

print(json.dumps(requested))
"""
        result = subprocess.run(
            [sys.executable, "-c", code],
            cwd=Path(__file__).resolve().parents[3],
            capture_output=True,
            text=True,
            timeout=120,
        )

        assert result.returncode == 0, result.stderr
        assert json.loads(result.stdout.strip().splitlines()[-1]) == []


class TestTiktokenCacheOnImport:
    """The helper is only useful if importing the module runs it."""

    def test_import_seeds_the_cache_from_the_bundled_table(self, tmp_path, monkeypatch):
        """Importing must copy the bundled table and point tiktoken at it.

        Testing _ensure_tiktoken_cache alone passes even if nothing calls it,
        so drive it through a real import.
        """
        bundled = tmp_path / "ragflow_deps" / "cl100k_base.tiktoken"
        bundled.parent.mkdir()
        bundled.write_bytes(b"bundled cl100k table")
        monkeypatch.setenv("RAG_PROJECT_BASE", str(tmp_path))

        module = _reimport_token_utils(monkeypatch, lambda name: object())

        encoding_url = "https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken"
        cache_path = tmp_path / hashlib.sha1(encoding_url.encode()).hexdigest()
        assert cache_path.read_bytes() == b"bundled cl100k table"
        assert os.environ["TIKTOKEN_CACHE_DIR"] == str(tmp_path)
        assert module.tiktoken_cache_dir == str(tmp_path)
