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

    def test_encoder_attribute_still_resolves(self, monkeypatch):
        """`from common.token_utils import encoder` must keep working.

        `rag/prompts/generator.py` imports the name directly, so deleting it is
        not an acceptable way to defer the build.
        """
        built = []

        def fake_get_encoding(name):
            built.append(name)
            return tiktoken.Encoding(
                name="test",
                pat_str=r".",
                mergeable_ranks={b"a": 0},
                special_tokens={},
            )

        module = _reimport_token_utils(monkeypatch, fake_get_encoding)
        assert built == []

        encoder = module.encoder
        assert built == ["cl100k_base"]
        assert encoder is module.encoder, "the encoder must be built once and reused"

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

    def test_unknown_attribute_still_raises(self, monkeypatch):
        """The module __getattr__ must not mask genuine typos."""
        module = _reimport_token_utils(monkeypatch, lambda name: None)

        missing_name = "no_such_name"
        with pytest.raises(AttributeError):
            getattr(module, missing_name)
