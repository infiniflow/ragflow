#!/usr/bin/env python3
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
"""Emit one chunker case's Python output as JSON, for live Go<->Python parity.

This is the live counterpart of ``capture_golden.py``. Instead of writing a
golden fixture to disk, it runs the Python reference chunker on a single case
and prints its chunks to stdout, so a Go test can invoke it as a subprocess and
diff the result against the Go port without any committed golden file.

Usage (invoked by live_parity_test.go, not by hand):

    .venv/bin/python tool-py/live_chunk.py <case_id>

It reuses ``capture_golden.run_case`` verbatim, so the emitted chunks have
exactly the same shape as the golden fixtures. ``_assert_tokenizer_alive`` runs
first and exits non-zero if the encoder is unavailable, so a dead tokenizer
produces a hard failure instead of a plausible-looking one-chunk baseline.

Tier: this script is only ever called from a ``//go:build manual`` Go test,
because it requires the ``.venv`` Python environment with the ragflow
dependencies and the cl100k table.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

TOOL_DIR = Path(__file__).resolve().parent
sys.path.insert(0, str(TOOL_DIR))

from capture_golden import CASES_DIR, _assert_tokenizer_alive, run_case


def main() -> int:
    if len(sys.argv) < 2:
        sys.stderr.write("usage: live_chunk.py <case_id>\n")
        return 2
    case_id = sys.argv[1]
    path = CASES_DIR / f"{case_id}.json"
    if not path.exists():
        sys.stderr.write(f"no case file {path}\n")
        return 2

    case = json.loads(path.read_text(encoding="utf-8"))
    _assert_tokenizer_alive()
    result = run_case(case)
    sys.stdout.write(json.dumps(result, ensure_ascii=False, sort_keys=True, indent=1) + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
