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
from types import SimpleNamespace

import tiktoken

# Side-effect import: load the graph_extractor module (and its `litellm`
# dependency chain) so the BPE table is cached before the re-import tests
# run. Without this, the first re-import would trigger `litellm`'s own
# import-time `tiktoken.get_encoding("cl100k_base")` call, which the
# re-import's patch would intercept and trip the "no calls" assertion.
import rag.graphrag.general.graph_extractor as graph_extractor_module  # noqa: F401


def _reimport_graph_extractor(monkeypatch, get_encoding):
    """Import rag.graphrag.general.graph_extractor fresh with tiktoken.get_encoding replaced.

    Re-imports run the module body again, which is the only way to assert that
    the body itself does not call `tiktoken.get_encoding`. Patching in place
    after the module has already been imported would let any import-time build
    slip through.
    """
    monkeypatch.setattr(tiktoken, "get_encoding", get_encoding)
    monkeypatch.setenv("TIKTOKEN_CACHE_DIR", "")
    monkeypatch.delitem(sys.modules, "rag.graphrag.general.graph_extractor", raising=False)
    return importlib.import_module("rag.graphrag.general.graph_extractor")


def _build_llm_stub():
    return SimpleNamespace(llm_name="test-llm", max_length=4096)


class TestLazyEncoder:
    def test_import_does_not_build_the_encoder(self, monkeypatch):
        """Importing the module must not read or download the BPE table.

        Before the fix, `tiktoken.get_encoding("cl100k_base")` ran at the top
        of the module body, so every importer paid the BPE-table cost even
        when the caller never instantiated `GraphExtractor` (for example
        anything that pulled in the type for typing, mocking, or to read
        the module's constants). The shared `common.token_utils.get_encoder`
        helper now builds the encoding on first use, and the test
        re-imports the module so the import-time body runs against a
        recording `tiktoken.get_encoding`.
        """
        calls = []
        module = _reimport_graph_extractor(monkeypatch, lambda name: calls.append(name))

        assert calls == []
        assert module.GraphExtractor is not None

    def test_constructor_uses_the_shared_get_encoder(self, monkeypatch):
        """Constructing a GraphExtractor must reach the lazy `get_encoder()` helper.

        The constructor still needs the cl100k_base encoding to build
        `_loop_args`, so it must call into `common.token_utils.get_encoder`.
        Patching `tiktoken.get_encoding` to record-only proves the call goes
        through the shared helper, since `get_encoder` is what we now use.
        The BPE table is real here: the test pins the call going through
        `tiktoken.get_encoding` (and asserts the name), and the real
        encoding is needed to tokenize the `"YES"` and `"NO"` strings that
        feed the logit_bias map.
        """
        # Build the real encoder once so `.encode("YES")` and `.encode("NO")`
        # succeed; subsequent calls hit tiktoken's registry cache.
        real_cl100k = tiktoken.get_encoding("cl100k_base")
        requested = []

        def fake_get_encoding(name):
            requested.append(name)
            return real_cl100k

        module = _reimport_graph_extractor(monkeypatch, fake_get_encoding)
        assert requested == []  # import itself does not build

        extractor = module.GraphExtractor(_build_llm_stub(), entity_types=["person"])

        # The constructor still encodes "YES" and "NO" to derive the logit_bias
        # tokens, so the loop_args reflect the same `real_cl100k` encoding.
        yes_token = real_cl100k.encode("YES")[0]
        no_token = real_cl100k.encode("NO")[0]
        assert extractor._loop_args == {
            "logit_bias": {yes_token: 100, no_token: 100},
            "max_tokens": 1,
        }
        # At least one get_encoding call during construction; the exact count
        # depends on `num_tokens_from_string`'s reach into the same helper, so
        # pin the name and that there was no import-time call.
        assert requested, "expected at least one get_encoding call during construction"
        assert all(name == "cl100k_base" for name in requested)
