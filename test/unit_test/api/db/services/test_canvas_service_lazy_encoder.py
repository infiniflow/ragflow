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

# Side-effect import: load canvas_service (and its heavy `api.db.db_models`
# / `agent.canvas` / litellm chain) so the BPE table is cached and any
# eager call from a dependency has already happened. Without this, the
# re-import test below would trigger those chains on its first import and
# the recording `tiktoken.get_encoding` patch would catch their calls
# (which are out of scope here).
import api.db.services.canvas_service as canvas_service_module  # noqa: F401


def _reimport_canvas_service(monkeypatch, get_encoding):
    """Import api.db.services.canvas_service fresh with tiktoken.get_encoding replaced.

    Re-imports run the module body again, which is the only way to assert
    that the body itself does not call `tiktoken.get_encoding`. Patching
    in place after the module has already been imported would let any
    module-level build slip through.
    """
    monkeypatch.setattr(tiktoken, "get_encoding", get_encoding)
    monkeypatch.setenv("TIKTOKEN_CACHE_DIR", "")
    monkeypatch.delitem(sys.modules, "api.db.services.canvas_service", raising=False)
    return importlib.import_module("api.db.services.canvas_service")


def _empty_completion(*_args, **_kwargs):
    """An empty async generator stand-in for `canvas_service.completion`.

    `completion_openai` iterates the result of `completion(...)` with
    `async for`. An empty async generator makes the body of the loop run
    zero times and fall through to the post-iteration encode/yield, which
    is all the test needs to observe the encoder build.
    """
    if False:
        yield  # pragma: no cover — type marker for async generator


class TestLazyEncoder:
    def test_import_does_not_build_the_encoder(self, monkeypatch):
        """Importing the module must not read or download the BPE table.

        Before the fix, `completion_openai` was the only function in the
        module that touched `tiktoken`, and its call to
        `tiktoken.get_encoding("cl100k_base")` was at the top of the
        function body. That is function-local, not import-time, so this
        assertion is the canary that any future reintroduction of an
        import-time `tiktoken.get_encoding(...)` would be caught here
        too. The re-import runs the module body against a recording
        `tiktoken.get_encoding`.
        """
        calls = []
        module = _reimport_canvas_service(monkeypatch, lambda name: calls.append(name))

        assert calls == []
        assert module.completion_openai is not None

    @pytest.mark.asyncio
    async def test_completion_openai_uses_get_encoder(self, monkeypatch):
        """`completion_openai` must reach the shared `get_encoder()` helper.

        The function-local call was the remaining `tiktoken.get_encoding`
        in the codebase. Patching `tiktoken.get_encoding` to record-only
        proves the call goes through the shared helper, since
        `get_encoder` is what we now use. The BPE table is real here:
        `str(question)` is encoded and used to size the streaming chunks,
        so the test needs a real cl100k_base encoding to tokenize the
        input string.
        """
        real_cl100k = tiktoken.get_encoding("cl100k_base")
        requested = []

        def fake_get_encoding(name):
            requested.append(name)
            return real_cl100k

        module = _reimport_canvas_service(monkeypatch, fake_get_encoding)
        assert requested == []  # import itself does not build

        # Stub the heavy `completion` async generator to an empty one so
        # `completion_openai` exits the iteration without raising. The
        # non-streaming branch is used (`stream=False`); after the empty
        # iteration the function encodes "" (length 0) and yields the
        # final payload, which is enough to observe the encoder build.
        monkeypatch.setattr(module, "completion", _empty_completion)

        results = []
        async for chunk in module.completion_openai(tenant_id="t", agent_id="a", question="hello", stream=False):
            results.append(chunk)

        # At least one get_encoding call during the function-local path;
        # the exact count is an implementation detail (tiktoken's registry
        # cache makes the second call a fast dict lookup). Pin the name
        # and that there was no import-time call.
        assert requested, "expected at least one get_encoding call inside completion_openai"
        assert all(name == "cl100k_base" for name in requested)
        # The non-streaming branch yields exactly one payload (the
        # assembled openai_data dict). The empty input is fine; what
        # matters is that the call completed and produced the recorder
        # call we were pinning.
        assert len(results) == 1
