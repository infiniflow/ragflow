import time
from typing import Any, Dict, List, Optional

from .http_client import HttpClient
from .metrics import RetrievalSample


class RetrievalError(RuntimeError):
    pass


def build_payload(
    question: str,
    dataset_ids: List[str],
    document_ids: Optional[List[str]] = None,
    payload: Optional[Dict[str, Any]] = None,
) -> Dict[str, Any]:
    body = dict(payload or {})
    if "question" not in body:
        body["question"] = question
    if "dataset_ids" not in body:
        body["dataset_ids"] = dataset_ids
    if document_ids is not None and "document_ids" not in body:
        body["document_ids"] = document_ids
    return body


def run_retrieval(client: HttpClient, payload: Dict[str, Any]) -> RetrievalSample:
    t0 = time.perf_counter()
    response = client.request("POST", "/retrieval", json_body=payload, stream=False)
    raw = response.content
    t1 = time.perf_counter()
    try:
        res = client.parse_json_bytes(raw)
    except Exception as exc:
        return RetrievalSample(t0=t0, t1=t1, error=f"Invalid JSON response: {exc}")
    if res.get("code") != 0:
        return RetrievalSample(t0=t0, t1=t1, error=res.get("message"), response=res)
    return RetrievalSample(t0=t0, t1=t1, error=None, response=res)
