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
"""Capture Python chunker outputs as golden fixtures for the Go parity test.

Python is the reference implementation; this script records what it actually
produces so `golden_parity_test.go` can assert the Go port reproduces it.

Usage (from the repository root):

    source .venv/bin/activate
    export PYTHONPATH=$(pwd)
    python internal/ingestion/component/chunker/tool-py/capture_golden.py --all

Why components are built with ``__new__`` instead of the normal constructor:
``ComponentBase.__init__`` asserts its ``canvas`` argument is a real ``Graph``,
which a fixture harness has no way to supply. Bypassing ``__init__`` and
attaching the three attributes ``_invoke`` actually reads (``_canvas``,
``_param``, ``callback``) runs the genuine chunking code path with the real
``num_tokens_from_string`` and the real ``naive_merge``.

Do NOT swap in the module stubs from ``rag/flow/tests/test_token_chunker.py``:
that file replaces ``num_tokens_from_string`` with ``lambda text: 1`` and
``naive_merge`` with a function returning ``[]``. Golden values captured under
those stubs would be systematically wrong.

``num_tokens_from_string`` (``common/token_utils.py``) builds the tiktoken encoder
on first use and lets an unavailable encoding raise, but a failed encode of a
valid string is swallowed and reported as 0, which silently turns every token
budget into "never exceeded" and collapses each case into a single chunk. That
failure looks like a legitimate baseline, so ``_assert_tokenizer_alive`` runs
first and aborts the capture on either shape rather than writing a poisoned golden.
"""

from __future__ import annotations

import argparse
import asyncio
import json
import subprocess
import sys
import types
from datetime import datetime, timezone
from pathlib import Path

TOOL_DIR = Path(__file__).resolve().parent
PARITY_DIR = TOOL_DIR.parent / "testdata" / "parity"
CASES_DIR = PARITY_DIR / "cases"
GOLDEN_DIR = PARITY_DIR / "golden"
MANIFEST = PARITY_DIR / "_manifest.json"
REPO_ROOT = TOOL_DIR.parents[4]

CAPTURE_CMD = "python internal/ingestion/component/chunker/tool-py/capture_golden.py --all"


def _build_component(component: str, param: dict):
    """Instantiate a chunker while reproducing every effect of the real constructor.

    ``__new__`` skips ``ComponentBase.__init__`` (agent/component/base.py:375),
    which is the point — that constructor asserts its canvas is a real ``Graph``.
    But skipping it means its three effects must be reproduced by hand, not just
    the ones needed to make the code run:

    1. ``self._canvas = canvas`` — a real ``Graph`` is unobtainable in a
       fixture harness, but ``None`` is not a safe stand-in: PDF cases read
       ``canvas._doc_id`` (pdf_chunk_metadata.py:190) and would ``AttributeError``.
       A throwaway ``types.SimpleNamespace`` with the two attributes that matter
       is enough to keep those code paths alive without touching a real canvas.
    2. ``self._param = param``
    3. ``self._param.check()`` — the parameter validation. ``__new__`` bypasses
       the constructor, so without an explicit ``check()`` Python will accept
       values it would reject in production and emit a baseline for a
       configuration that cannot occur, which would then be used to judge Go. A
       baseline must only ever record legal behaviour.
    """
    if component == "TokenChunker":
        from rag.flow.chunker.token_chunker import TokenChunker, TokenChunkerParam

        cls, param_cls = TokenChunker, TokenChunkerParam
    elif component in ("TitleChunker", "GroupTitleChunker", "HierarchyTitleChunker"):
        from rag.flow.chunker.title_chunker.common import TitleChunkerParam
        from rag.flow.chunker.title_chunker.title_chunker import TitleChunker

        cls, param_cls = TitleChunker, TitleChunkerParam
    else:
        raise ValueError(f"unsupported component {component!r}")

    p = param_cls()
    # GroupTitleChunker / HierarchyTitleChunker are distinct registry names on
    # the Go side but a single Python class dispatching on `method`. Derive the
    # method so one case file drives both sides unchanged.
    if component == "GroupTitleChunker":
        p.method = "group"
    elif component == "HierarchyTitleChunker":
        p.method = "hierarchy"
    for key, value in param.items():
        setattr(p, key, value)
    # __new__ bypassed ComponentBase.__init__, which would normally have run
    # _param.check(); reproduce that validation so illegal params are rejected
    # here too instead of being baked into the golden baseline.
    p.check()

    comp = cls.__new__(cls)
    # None would crash PDF cases that read canvas._doc_id; a minimal namespace
    # keeps those paths alive without a real Graph.
    comp._canvas = types.SimpleNamespace(_doc_id=None, _tenant_id="t")
    comp._param = p
    comp.callback = lambda *_args, **_kwargs: None
    return comp


def _assert_tokenizer_alive() -> None:
    """Abort unless the real tiktoken encoder is loaded and counting.

    Every token-budget decision in ``naive_merge`` reads
    ``num_tokens_from_string``, which fails in two shapes: an unavailable BPE
    table raises out of the lazy encoder build, and a failed encode of a valid
    string is reported as 0. The raise would only crash the capture; the 0 means
    the budget is never exceeded and every case degenerates to one chunk -- a
    plausible-looking baseline that is entirely an artefact of the capture
    environment. Both abort here before any golden is written.
    """
    from rag.nlp import num_tokens_from_string

    probe = "RAGFlow chunker parity tokenizer liveness probe."
    hint = "Fix the encoding first: uv run python3 ragflow_deps/download_deps.py (provides ragflow_deps/cl100k_base.tiktoken)."
    # Two shapes to catch. common/token_utils.py builds the encoder on first use and
    # lets that failure propagate, so an unavailable BPE table raises here; a failure
    # to encode an otherwise valid string is still reported as 0.
    try:
        got = num_tokens_from_string(probe)
    except Exception as exc:
        sys.exit(f"tokenizer is dead: num_tokens_from_string() raised {exc!r}.\n{hint}")
    if got <= 0:
        sys.exit(f"tokenizer is dead: num_tokens_from_string() returned {got} for a {len(probe)}-character probe, so capturing now would write goldens where no token budget is ever exceeded.\n{hint}")


def run_case(case: dict) -> dict:
    """Run one case and return its recorded outputs."""
    comp = _build_component(case["component"], case.get("param", {}))
    asyncio.run(comp._invoke(**case["input"]))

    outputs = comp._param.outputs
    error = outputs.get("_ERROR", {}).get("value")
    if error is not None:
        return {"case_id": case["id"], "error": error}
    chunks = outputs.get("chunks", {}).get("value") or []
    return {"case_id": case["id"], "chunks": chunks}


def serialize(result: dict) -> str:
    return json.dumps(result, ensure_ascii=False, sort_keys=True, indent=1) + "\n"


def load_cases(only: str | None) -> list[dict]:
    paths = sorted(CASES_DIR.glob("*.json"))
    if not paths:
        sys.exit(f"no case files found under {CASES_DIR}")
    cases = []
    for path in paths:
        case = json.loads(path.read_text(encoding="utf-8"))
        if case["id"] != path.stem:
            sys.exit(f"{path}: case id {case['id']!r} does not match filename stem {path.stem!r}")
        if only is None or case["id"] == only:
            cases.append(case)
    if only is not None and not cases:
        sys.exit(f"no case with id {only!r} under {CASES_DIR}")
    return cases


def write_manifest() -> None:
    """Record provenance in one shared file.

    Kept out of the per-case golden files so a re-capture does not touch every
    fixture's diff with a new timestamp.
    """
    try:
        commit = subprocess.run(
            ["git", "rev-parse", "HEAD"],
            cwd=REPO_ROOT,
            capture_output=True,
            text=True,
            check=True,
        ).stdout.strip()
    except (subprocess.CalledProcessError, FileNotFoundError):
        commit = "unknown"
    MANIFEST.write_text(
        json.dumps(
            {
                "captured_at": datetime.now(timezone.utc).isoformat(timespec="seconds"),
                "commit": commit,
                "python": sys.version.split()[0],
                "command": CAPTURE_CMD,
            },
            indent=1,
            sort_keys=True,
        )
        + "\n",
        encoding="utf-8",
    )


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    group = ap.add_mutually_exclusive_group(required=True)
    group.add_argument("--all", action="store_true", help="capture every case")
    group.add_argument("--case", metavar="ID", help="capture a single case")
    ap.add_argument(
        "--check",
        action="store_true",
        help="re-run cases and compare against the golden files on disk without writing; exit non-zero on drift",
    )
    args = ap.parse_args()

    _assert_tokenizer_alive()

    GOLDEN_DIR.mkdir(parents=True, exist_ok=True)
    cases = load_cases(args.case)

    drifted: list[str] = []
    for case in cases:
        first = serialize(run_case(case))
        # Determinism guard: a chunker that is not reproducible would otherwise
        # bake its nondeterminism into the baseline, and every later parity
        # failure would be unexplainable.
        second = serialize(run_case(case))
        if first != second:
            sys.exit(f"{case['id']}: non-deterministic output across two runs; refusing to write a golden file")

        target = GOLDEN_DIR / f"{case['id']}.json"
        if args.check:
            existing = target.read_text(encoding="utf-8") if target.exists() else None
            if existing != first:
                drifted.append(case["id"])
                print(f"DRIFT {case['id']}", file=sys.stderr)
            continue
        target.write_text(first, encoding="utf-8")
        print(f"wrote {target.relative_to(REPO_ROOT)}")

    if args.check:
        if drifted:
            print(f"\n{len(drifted)} golden file(s) stale; re-run: {CAPTURE_CMD}", file=sys.stderr)
            return 1
        print(f"{len(cases)} golden file(s) up to date")
        return 0

    write_manifest()
    return 0


if __name__ == "__main__":
    sys.exit(main())
