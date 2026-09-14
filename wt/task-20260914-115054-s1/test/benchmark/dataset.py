from pathlib import Path
from typing import Any, Dict, Iterable, List, Optional

from .http_client import HttpClient

try:
    from requests_toolbelt import MultipartEncoder
except Exception:  # pragma: no cover - fallback without toolbelt
    MultipartEncoder = None


class DatasetError(RuntimeError):
    pass


def create_dataset(client: HttpClient, name: str, payload: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
    body = dict(payload or {})
    if "name" not in body:
        body["name"] = name
    res = client.request_json("POST", "/datasets", json_body=body)
    if res.get("code") != 0:
        raise DatasetError(f"Create dataset failed: {res.get('message')}")
    return res.get("data", {})


def list_datasets(client: HttpClient, dataset_id: Optional[str] = None, name: Optional[str] = None) -> List[Dict[str, Any]]:
    params = {}
    if dataset_id is not None:
        params["id"] = dataset_id
    if name is not None:
        params["name"] = name
    res = client.request_json("GET", "/datasets", params=params or None)
    if res.get("code") != 0:
        raise DatasetError(f"List datasets failed: {res.get('message')}")
    return res.get("data", [])


def delete_dataset(client: HttpClient, dataset_id: str) -> None:
    payload = {"ids": [dataset_id]}
    res = client.request_json("DELETE", "/datasets", json_body=payload)
    if res.get("code") != 0:
        raise DatasetError(f"Delete dataset failed: {res.get('message')}")


def upload_documents(client: HttpClient, dataset_id: str, file_paths: Iterable[str]) -> List[Dict[str, Any]]:
    paths = [Path(p) for p in file_paths]
    if MultipartEncoder is None:
        files = [("file", (p.name, p.open("rb"))) for p in paths]
        try:
            response = client.request(
                "POST",
                f"/datasets/{dataset_id}/documents",
                headers=None,
                data=None,
                json_body=None,
                files=files,
                params=None,
                stream=False,
                auth_kind="api",
            )
        finally:
            for _, (_, fh) in files:
                fh.close()
        res = response.json()
    else:
        fields = []
        file_handles = []
        try:
            for path in paths:
                fh = path.open("rb")
                fields.append(("file", (path.name, fh)))
                file_handles.append(fh)
            encoder = MultipartEncoder(fields=fields)
            headers = {"Content-Type": encoder.content_type}
            response = client.request(
                "POST",
                f"/datasets/{dataset_id}/documents",
                headers=headers,
                data=encoder,
                json_body=None,
                params=None,
                stream=False,
                auth_kind="api",
            )
            res = response.json()
        finally:
            for fh in file_handles:
                fh.close()
    if res.get("code") != 0:
        raise DatasetError(f"Upload documents failed: {res.get('message')}")
    return res.get("data", [])


def parse_documents(client: HttpClient, dataset_id: str, document_ids: List[str]) -> Dict[str, Any]:
    payload = {"document_ids": document_ids}
    res = client.request_json("POST", f"/datasets/{dataset_id}/chunks", json_body=payload)
    if res.get("code") != 0:
        raise DatasetError(f"Parse documents failed: {res.get('message')}")
    return res


def list_documents(client: HttpClient, dataset_id: str, params: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
    res = client.request_json("GET", f"/datasets/{dataset_id}/documents", params=params)
    if res.get("code") != 0:
        raise DatasetError(f"List documents failed: {res.get('message')}")
    return res.get("data", {})


def wait_for_parse_done(
    client: HttpClient,
    dataset_id: str,
    document_ids: Optional[List[str]],
    timeout: float,
    interval: float,
) -> None:
    import time

    start = time.monotonic()
    while True:
        data = list_documents(client, dataset_id)
        docs = data.get("docs", [])
        target_ids = set(document_ids or [])
        all_done = True
        for doc in docs:
            if target_ids and doc.get("id") not in target_ids:
                continue
            if doc.get("run") != "DONE":
                all_done = False
                break
        if all_done:
            return
        if time.monotonic() - start > timeout:
            raise DatasetError("Document parsing timeout")
        time.sleep(max(interval, 0.1))


def extract_document_ids(documents: Iterable[Dict[str, Any]]) -> List[str]:
    return [doc["id"] for doc in documents if "id" in doc]


def dataset_has_chunks(dataset_info: Dict[str, Any]) -> bool:
    for key in ("chunk_count", "chunk_num"):
        value = dataset_info.get(key)
        if isinstance(value, int) and value > 0:
            return True
    return False
