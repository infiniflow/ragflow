"""Real intake worker contract; run only in the disposable live QA stack."""

import json
import os
from pathlib import Path
from time import monotonic, sleep
from uuid import uuid4

import pytest

from test.playwright.helpers.parsed_document import api_data


@pytest.mark.auth
def test_real_business_document_intake(ensure_auth_context, base_url):
    assert os.environ.get("QA_DISPOSABLE_PROJECT", "").startswith("ragflow-t1-live-20260906-b-"), "Disposable live runner required"
    assert base_url.rstrip("/") == "http://127.0.0.1:19382"
    page = ensure_auth_context
    headers = {"Authorization": page.evaluate("localStorage.getItem('Authorization')")}
    root = base_url.rstrip("/") + "/api/v1/business-documents"
    document = api_data(
        page.request.post(
            root,
            headers=headers,
            data={
                "schema_version": "1",
                "document_type": "business_requirements",
                "title": "regression-probe-" + uuid4().hex,
                "idea": "Нужна система бронирования синтетических телескопов. Пользователи, ограничения и критерии успеха пока не определены.",
            },
        )
    )
    url = root + "/" + document["document_id"]
    try:
        key = uuid4().hex
        command = api_data(
            page.request.post(
                url + "/commands",
                headers=headers,
                data={
                    "schema_version": "1",
                    "command_id": key,
                    "idempotency_key": key,
                    "expected_state_version": document["state_version"],
                    "type": "REQUEST_INTAKE_ASSESSMENT",
                    "payload": {},
                },
            )
        )
        deadline = monotonic() + 240
        while True:
            projection = api_data(page.request.get(url, headers=headers))
            job = projection.get("latest_job") or {}
            if job.get("status") == "COMPLETED":
                break
            assert job.get("status") not in {"FAILED", "DEAD", "CANCELLED"}, f"Intake worker failed: {job}"
            assert monotonic() < deadline, f"Intake worker timed out: {job}"
            sleep(2)
        assert job["job_id"] == command["job_id"]
        questions = projection["protocol"]["questions"]
        assert questions, "Incomplete idea must produce clarification questions"
        assert len({question["question_id"] for question in questions}) == len(questions)
        for question in questions:
            assert 2 <= len(question["options"]) <= 4
            assert all(option["option_id"] and option["label"] for option in question["options"])
        assert projection["state_version"] > document["state_version"]
        evidence = Path(os.environ["QA_LIVE_EVIDENCE"]) / "business-intake-proof.json"
        evidence.write_text(json.dumps({"status": job["status"], "question_count": len(questions), "state_version": projection["state_version"], "real_worker": True}, indent=2), encoding="utf-8")
    finally:
        api_data(page.request.delete(url, headers=headers))
        assert page.request.get(url, headers=headers).status == 404, "Synthetic business document remains after cleanup"
