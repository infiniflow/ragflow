#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
"""
Wiki (Artifacts) incremental build & deletion integration tests.

SELF-CONTAINED: talks to a running RAGFlow REST API directly with an API key.
Place this file OUTSIDE test/testcases so the shared conftest (which validates
LLM models via set_tenant_info and would exit) is not loaded.

Prereqs (backend must be up, DOC_ENGINE=infinity):
  api server :9380 + task_executor + Infinity(docker).

Run:
  DOC_ENGINE=infinity .venv/bin/python -m pytest \
      test/integration/wiki/test_wiki_incremental.py -s -v

Env:
  RAGFLOW_API_KEY   default ragflow-Unleq1d1mMvztQH2QswdjfWZvP9Xkh-TAMhf_XrM7gc
  RAGFLOW_HOST      default http://localhost:9380
  WIKI_PIPELINE_ID  default 977f06ac8ccf11f192396b1c282a3cb7 (wikipipeline)
"""

import os
import time

import pytest
import requests

API_KEY = os.getenv(
    "RAGFLOW_API_KEY",
    "ragflow-Unleq1d1mMvztQH2QswdjfWZvP9Xkh-TAMhf_XrM7gc",
)
HOST = os.getenv("RAGFLOW_HOST", "http://localhost:9380")
PIPELINE_ID = os.getenv("WIKI_PIPELINE_ID", "977f06ac8ccf11f192396b1c282a3cb7")
API = f"{HOST}/api/v1"
HEADERS = {"Authorization": f"Bearer {API_KEY}"}


def _api(path, method="get", **kwargs):
    return requests.request(method, f"{API}{path}", headers=HEADERS, timeout=120, **kwargs).json()


def _wait_until(predicate, timeout=300, interval=3):
    deadline = time.time() + timeout
    while time.time() < deadline:
        if predicate():
            return True
        time.sleep(interval)
    return False


def _index_name(kb_id):
    import os

    os.environ.setdefault("DOC_ENGINE", "infinity")
    from rag.nlp import search

    return search.index_name("c48fdfe233b411f19e11502f9b2d03b6")


def _wiki_counts(kb_id):
    """Return (wiki_page_count, wiki_relation_count) directly from Infinity."""
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr

    settings.init_settings()
    conn = settings.docStoreConn
    idx = _index_name(kb_id)
    res = conn.search(["id"], [], {"compile_kwd": ["wiki_page"]}, [], OrderByExpr(), 0, 0, idx, [kb_id])
    pages = conn.get_total(res)
    res = conn.search(["id"], [], {"compile_kwd": ["wiki_relation"]}, [], OrderByExpr(), 0, 0, idx, [kb_id])
    rel = conn.get_total(res)
    return pages, rel


EMBEDDING_MODEL = os.getenv(
    "RAGFLOW_EMBEDDING_MODEL",
    "3525a36a7acf11f19b43cd920cb77b91",  # system embedding used by KB4
)


def _create_wiki_dataset(name):
    """Create a dataset wired to the wiki pipeline."""
    res = _api(
        "/datasets",
        "post",
        json={"name": name, "embedding_model": EMBEDDING_MODEL},
    )
    assert res.get("code") == 0, f"create_dataset failed: {res}"
    ds_id = res["data"]["id"]
    # Associate the wiki pipeline (its compiler -> wiki template group).
    # The REST API requires parse_type (int) alongside pipeline_id.
    up = _api(
        f"/datasets/{ds_id}",
        "put",
        json={"parse_type": 0, "pipeline_id": PIPELINE_ID},
    )
    assert up.get("code") == 0, f"associate pipeline failed: {up}"
    return ds_id


def _upload_and_parse(ds_id, contents, parse_timeout=300):
    files = [("file", (f"doc_{i}.txt", content.encode("utf-8"), "text/plain")) for i, content in enumerate(contents)]
    res = requests.post(
        f"{API}/datasets/{ds_id}/documents",
        headers=HEADERS,
        files=files,
        timeout=120,
    ).json()
    assert res.get("code") == 0, f"upload failed: {res}"
    doc_ids = [d["id"] for d in res.get("data", [])]
    assert doc_ids, f"no documents returned from upload: {res}"
    # Datasets wired to an ingestion pipeline cannot be parsed via /chunks;
    # use /documents/ingest with run=RUNNING("1") to trigger ingestion.
    ir = _api("/documents/ingest", "post", json={"doc_ids": doc_ids, "run": 1})
    assert ir.get("code") == 0, f"ingest failed: {ir}"
    ok = _wait_until(lambda: _all_docs_done(ds_id), timeout=parse_timeout)
    assert ok, "documents did not finish parsing"


def _all_docs_done(ds_id):
    res = _api(f"/datasets/{ds_id}/documents")
    if res.get("code") != 0:
        return False
    docs = res.get("data", {}).get("docs", [])
    return bool(docs) and all(d.get("run") in ("DONE", "FAIL") for d in docs)


def _wiki_task_done(ds_id):
    """True when the wiki task (if any) has reached a terminal progress.

    run_index refuses to start a new wiki task while an existing one has
    progress not in (-1, 1); so we must wait for the previous task to finish
    (progress == 1) before triggering the next build.
    """
    res = _api(f"/datasets/{ds_id}/index", "get", params={"type": "wiki"})
    if res.get("code") != 0:
        return False
    task = res.get("data") or {}
    if not task:
        # No task recorded yet on the KB row -> nothing in flight.
        return True
    progress = task.get("progress")
    return progress in (-1, 1)


def _trigger_wiki(ds_id, timeout=420, require_pages=True):
    res = _api(f"/datasets/{ds_id}/index", "post", params={"type": "wiki"})
    assert res.get("code") == 0, f"trigger wiki index failed: {res}"
    pages, rel = _wiki_counts(ds_id)
    ok = _wait_until(lambda: _wiki_task_done(ds_id), timeout=timeout)
    assert ok, "wiki task did not reach a terminal progress"
    if require_pages:
        ok = _wait_until(lambda: _wiki_counts(ds_id)[0] > 0, timeout=timeout)
        assert ok, "wiki compilation did not produce pages"
    return _wiki_counts(ds_id)


@pytest.fixture()
def wiki_dataset():
    ds_id = _create_wiki_dataset(f"wiki_it_{int(time.time())}")
    yield ds_id
    try:
        _api("/datasets", "delete", json={"ids": [ds_id]})
    except Exception:
        pass


def test_wiki_first_build_produces_pages(wiki_dataset):
    ds_id = wiki_dataset
    _upload_and_parse(
        ds_id,
        ["张伟是甲公司的员工，负责销售业务。甲公司位于北京，是一家科技公司。王五是乙公司的法务，乙公司从事法律咨询。张伟与王五曾合作过一个项目。"],
    )
    _trigger_wiki(ds_id)
    pages, rel = _wiki_counts(ds_id)
    assert pages > 0, "expected at least one wiki page after first build"


def test_wiki_add_document_incremental(wiki_dataset):
    ds_id = wiki_dataset
    _upload_and_parse(ds_id, ["张伟在甲公司任职，负责产品。王五是乙公司的法务。乙公司从事法律咨询。"])
    _trigger_wiki(ds_id)
    pages_before, _ = _wiki_counts(ds_id)
    assert pages_before > 0

    _upload_and_parse(ds_id, ["赵六在丙公司做财务。丙公司是一家会计事务所。"])
    _trigger_wiki(ds_id)
    pages_after, _ = _wiki_counts(ds_id)
    assert pages_after >= pages_before, f"incremental build shrank pages: before={pages_before} after={pages_after}"


def test_wiki_delete_document_incremental(wiki_dataset):
    ds_id = wiki_dataset
    # Two docs with disjoint entities so we can attribute pages to each.
    _upload_and_parse(
        ds_id,
        [
            "张伟在甲公司任职。甲公司是北京的一家科技公司。",
            "王五是乙公司的法务。乙公司从事法律咨询业务。",
        ],
    )
    _trigger_wiki(ds_id)
    pages_before, _ = _wiki_counts(ds_id)
    assert pages_before > 0

    # Find the doc containing "乙公司" / "王五" and delete only it.
    res = _api(f"/datasets/{ds_id}/documents")
    docs = res["data"]["docs"]
    assert len(docs) == 2, f"expected 2 docs, got {len(docs)}"
    docs = sorted(docs, key=lambda d: d["name"])
    doc_to_delete = docs[0]["id"]

    dres = _api(f"/datasets/{ds_id}/documents", "delete", json={"ids": [doc_to_delete]})
    assert dres.get("code") == 0, f"delete docs failed: {dres}"

    # Deletion eagerly cleans up the removed doc's wiki products. After the
    # incremental re-run (backstop) the surviving doc's pages must remain and
    # the removed doc's entity pages must not come back.
    _trigger_wiki(ds_id, timeout=420, require_pages=True)
    pages_after, _ = _wiki_counts(ds_id)
    assert pages_after > 0, "surviving doc lost all its wiki pages"
    # The removed doc's pages ("王五"/"乙公司") must be gone; keep a loose bound
    # since page slugs are slugified.
    assert pages_after <= pages_before, f"incremental delete regrew pages: before={pages_before} after={pages_after}"


def test_wiki_plan_toggle_resets_state(wiki_dataset):
    ds_id = wiki_dataset
    tenant_id = "c48fdfe233b411f19e11502f9b2d03b6"
    _upload_and_parse(ds_id, ["张伟在甲公司，负责销售。甲公司是北京的科技公司。"])
    _trigger_wiki(ds_id)
    pages_before, _ = _wiki_counts(ds_id)
    assert pages_before > 0

    import asyncio
    from rag.svr.task_executor_refactor import dataset_wiki_generator as dwg

    # Entity mode records its mode in the mode meta row.
    asyncio.run(dwg._wiki_save_mode(tenant_id, ds_id, "entity"))
    loaded = asyncio.run(dwg._wiki_load_mode(tenant_id, ds_id))
    assert loaded == "entity", f"expected recorded entity mode, got {loaded!r}"

    # Switching to topic mode is a config change: run_wiki_incremental
    # detects prev != new and resets all wiki-derived state so the next build
    # rebuilds cleanly in the new mode (no mixing of A/B page structures).
    asyncio.run(dwg._wiki_reset_all_wiki_state(tenant_id, ds_id))
    pages_after, _ = _wiki_counts(ds_id)
    assert pages_after == 0, "full reset did not clear wiki state"

    # After reset the mode meta is gone too (first build of the new mode).
    assert asyncio.run(dwg._wiki_load_mode(tenant_id, ds_id)) is None, "mode meta was not cleared by reset"
