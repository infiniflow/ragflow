"""Synthetic document fixture that exercises the real ingestion worker."""

from contextlib import contextmanager
from time import monotonic, sleep
from uuid import uuid4


def api_data(response):
    assert response.ok, f"Fixture HTTP {response.status}"
    payload = response.json()
    assert payload.get("code") == 0, f"Fixture API failed: {payload}"
    return payload.get("data")


@contextmanager
def parsed_document(page, base_url, dataset):
    def headers():
        token = page.evaluate("localStorage.getItem('Authorization') || localStorage.getItem('Token')")
        assert token, "Missing fixture authorization"
        return {"Authorization": token}

    url = f"{base_url.rstrip('/')}/api/v1/datasets/{dataset['kb_id']}/documents"
    name = f"retrieval-proof-{uuid4().hex}.txt"
    snippet = "RAGFlow is a retrieval augmented generation application. RAGFlow retrieval proof: the violet observatory stores seven copper telescopes."
    documents = api_data(
        page.request.post(
            url,
            headers=headers(),
            multipart={
                "file": {"name": name, "mimeType": "text/plain", "buffer": snippet.encode()},
            },
        )
    )
    assert len(documents) == 1
    document_id = documents[0]["id"]
    try:
        api_data(page.request.post(url + "/parse", headers=headers(), data={"document_ids": [document_id]}))
        deadline = monotonic() + 240
        while True:
            result = api_data(page.request.get(url, headers=headers(), params={"ids": document_id}))
            document = next(item for item in result["docs"] if item["id"] == document_id)
            if document.get("run") == "DONE":
                break
            assert document.get("run") != "FAIL", f"Fixture parse failed: {document.get('progress_msg')}"
            assert monotonic() < deadline, f"Fixture parse timed out: {document.get('progress_msg')}"
            sleep(2)
        assert document.get("chunk_count", document.get("chunk_num", 0)) > 0, "Parsed fixture has no indexed chunks"
        yield {**dataset, "seed_document_id": document_id, "seed_document_name": name, "seed_snippet": snippet}
    finally:
        api_data(page.request.delete(url, headers=headers(), data={"ids": [document_id]}))
        remaining = api_data(page.request.get(url, headers=headers(), params={"ids": document_id}))
        assert remaining["docs"] == [], "Synthetic document remains after cleanup"
