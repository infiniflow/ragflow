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

"""Regression tests for the lazy tiktoken encoder inside ``rag.app.resume``.

The previous module-level ``tiktoken.encoding_for_model(\"gpt-3.5-turbo\")`` call
read (or downloaded) the cl100k_base BPE table every time any module imported
``rag.app.resume``. Several API handlers import it eagerly even when the resume
pipeline is not in use, so the eager build defeated the lazy helper that
``common.token_utils.get_encoder`` provides and that the merged ``#18986`` /
cycle 65 (``graph_extractor``) / cycle 66 (``canvas_service``) work steered
every caller toward.

The fix moves the encoder behind a function-local helper that calls
``common.token_utils.get_encoder`` on first use and memoizes the result in a
holder list. The SmartResume heuristic falls back to a case/digit alternation
test when tiktoken is unavailable, so the helper also accepts a None result.
"""

import json
import subprocess
import sys
from pathlib import Path


def test_rag_app_resume_does_not_load_the_bpe_table_on_import():
    """Importing ``rag.app.resume`` must not reach the BPE table.

    A fresh interpreter covers the whole import chain. ``rag.app.resume`` is
    imported by a few API handlers, so an eager build there used to make the
    cl100k_base loader fire on every server start. Every way to build an
    encoding (``tiktoken.get_encoding``, ``tiktoken.encoding_for_model``,
    ``tiktoken.registry.get_encoding``) ends in
    ``tiktoken.load.load_tiktoken_bpe``, so patching the entry point catches
    the whole family.
    """
    code = (
        "import json\n"
        "import tiktoken.load\n"
        "\n"
        "requested = []\n"
        "\n"
        "\n"
        "def unreachable(tiktoken_bpe_file, expected_hash=None):\n"
        "    requested.append(tiktoken_bpe_file)\n"
        '    raise OSError("BPE table unreachable")\n'
        "\n"
        "\n"
        "tiktoken.load.load_tiktoken_bpe = unreachable\n"
        "\n"
        "import rag.app.resume\n"
        "\n"
        'print(json.dumps({"requested": requested, "holder": len(rag.app.resume._tiktoken_encoding_holder)}))\n'
    )

    result = subprocess.run(
        [sys.executable, "-c", code],
        cwd=Path(__file__).resolve().parents[4],
        capture_output=True,
        text=True,
        timeout=120,
    )

    assert result.returncode == 0, result.stderr
    payload = json.loads(result.stdout.strip().splitlines()[-1])
    assert payload["requested"] == [], "rag.app.resume imported the BPE table at module load: " + repr(payload["requested"])
    # Holder is populated lazily by _get_tiktoken_encoding(); an import alone
    # must not call the helper.
    assert payload["holder"] == 0


def test_get_tiktoken_encoding_falls_back_to_none_when_helper_fails(monkeypatch):
    """A missing BPE table or absent tiktoken must yield None, not crash.

    ``_should_remove_random_str`` and ``_text_shingles`` both fall back to the
    case/digit alternation heuristic (or an empty shingle set) when the
    encoder is unavailable, so the helper has to report failure rather than
    propagate an exception.
    """
    import rag.app.resume as resume_module

    monkeypatch.setattr(resume_module, "_tiktoken_encoding_holder", [])

    def _boom():
        raise RuntimeError("BPE table unavailable")

    # The lazy import happens inside _get_tiktoken_encoding; reach in and
    # replace common.token_utils.get_encoder with a stub that raises.
    import common.token_utils

    monkeypatch.setattr(common.token_utils, "get_encoder", _boom)

    assert resume_module._get_tiktoken_encoding() is None
    # Subsequent calls must keep returning the cached None rather than
    # re-importing and re-raising on every request.
    assert resume_module._get_tiktoken_encoding() is None


def test_get_tiktoken_encoding_memoizes_the_encoder(monkeypatch):
    """The lazy encoder is built once and reused across calls.

    The SmartResume heuristic walks a long resume document once per request;
    building the encoder on every short-circuit check would re-read the BPE
    table per call.
    """
    import rag.app.resume as resume_module

    monkeypatch.setattr(resume_module, "_tiktoken_encoding_holder", [])

    class _StubEncoder:
        def encode(self, text):
            return [ord(c) for c in text[:8]]

    sentinel = _StubEncoder()
    calls = []

    import common.token_utils

    def _fake():
        calls.append(1)
        return sentinel

    monkeypatch.setattr(common.token_utils, "get_encoder", _fake)

    first = resume_module._get_tiktoken_encoding()
    second = resume_module._get_tiktoken_encoding()
    third = resume_module._get_tiktoken_encoding()

    assert first is sentinel
    assert second is sentinel
    assert third is sentinel
    assert len(calls) == 1
