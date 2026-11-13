import time
from typing import Any, Dict, List, Optional

import requests

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
    """Measure one retrieval, recording transport and API failures as samples."""
    t0 = time.perf_counter()
    response = None
    try:
        response = client.request("POST", "/retrieval", json_body=payload, stream=False)
        raw = response.content
        t1 = time.perf_counter()
        if not 200 <= response.status_code < 300:
            return RetrievalSample(t0=t0, t1=t1, error=f"HTTP {response.status_code}")
        try:
            res = client.parse_json_bytes(raw)
        except ValueError as exc:
            return RetrievalSample(t0=t0, t1=t1, error=f"Invalid JSON response: {exc}")
    except requests.RequestException as exc:
        return RetrievalSample(t0=t0, t1=time.perf_counter(), error=f"Transport error ({type(exc).__name__})")
    finally:
        if response is not None:
            response.close()
    if not isinstance(res, dict):
        return RetrievalSample(t0=t0, t1=t1, error="Invalid retrieval response: expected an object")
    if res.get("code") != 0:
        error = str(res.get("message") or f"Retrieval failed (code {res.get('code')!r})")
        return RetrievalSample(t0=t0, t1=t1, error=error, response=res)
    return RetrievalSample(t0=t0, t1=t1, error=None, response=res)
