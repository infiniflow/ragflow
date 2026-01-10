import json
import time
from typing import Any, Dict, List, Optional

from .http_client import HttpClient
from .metrics import ChatSample


class ChatError(RuntimeError):
    pass


def delete_chat(client: HttpClient, chat_id: str) -> None:
    payload = {"ids": [chat_id]}
    res = client.request_json("DELETE", "/chats", json_body=payload)
    if res.get("code") != 0:
        raise ChatError(f"Delete chat failed: {res.get('message')}")


def create_chat(
    client: HttpClient,
    name: str,
    dataset_ids: Optional[List[str]] = None,
    payload: Optional[Dict[str, Any]] = None,
) -> Dict[str, Any]:
    body = dict(payload or {})
    if "name" not in body:
        body["name"] = name
    if dataset_ids is not None and "dataset_ids" not in body:
        body["dataset_ids"] = dataset_ids
    res = client.request_json("POST", "/chats", json_body=body)
    if res.get("code") != 0:
        raise ChatError(f"Create chat failed: {res.get('message')}")
    return res.get("data", {})


def get_chat(client: HttpClient, chat_id: str) -> Dict[str, Any]:
    res = client.request_json("GET", "/chats", params={"id": chat_id})
    if res.get("code") != 0:
        raise ChatError(f"Get chat failed: {res.get('message')}")
    data = res.get("data", [])
    if not data:
        raise ChatError("Chat not found")
    return data[0]


def resolve_model(model: Optional[str], chat_data: Optional[Dict[str, Any]]) -> str:
    if model:
        return model
    if chat_data:
        llm = chat_data.get("llm") or {}
        llm_name = llm.get("model_name")
        if llm_name:
            return llm_name
    raise ChatError("Model name is required; provide --model or use a chat with llm.model_name.")


def _parse_stream_error(response) -> Optional[str]:
    content_type = response.headers.get("Content-Type", "")
    if "text/event-stream" in content_type:
        return None
    try:
        payload = response.json()
    except Exception:
        return f"Unexpected non-stream response (status {response.status_code})"
    if payload.get("code") not in (0, None):
        return payload.get("message", "Unknown error")
    return f"Unexpected non-stream response (status {response.status_code})"


def stream_chat_completion(
    client: HttpClient,
    chat_id: str,
    model: str,
    messages: List[Dict[str, Any]],
    extra_body: Optional[Dict[str, Any]] = None,
) -> ChatSample:
    payload: Dict[str, Any] = {"model": model, "messages": messages, "stream": True}
    if extra_body:
        payload["extra_body"] = extra_body
    t0 = time.perf_counter()
    response = client.request(
        "POST",
        f"/chats_openai/{chat_id}/chat/completions",
        json_body=payload,
        stream=True,
    )
    error = _parse_stream_error(response)
    if error:
        response.close()
        return ChatSample(t0=t0, t1=None, t2=None, error=error)

    t1: Optional[float] = None
    t2: Optional[float] = None
    stream_error: Optional[str] = None
    content_parts: List[str] = []
    try:
        for raw_line in response.iter_lines(decode_unicode=True):
            if raw_line is None:
                continue
            line = raw_line.strip()
            if not line or not line.startswith("data:"):
                continue
            data = line[5:].strip()
            if not data:
                continue
            if data == "[DONE]":
                t2 = time.perf_counter()
                break
            try:
                chunk = json.loads(data)
            except Exception as exc:
                stream_error = f"Invalid JSON chunk: {exc}"
                t2 = time.perf_counter()
                break
            choices = chunk.get("choices") or []
            choice = choices[0] if choices else {}
            delta = choice.get("delta") or {}
            content = delta.get("content")
            if t1 is None and isinstance(content, str) and content != "":
                t1 = time.perf_counter()
            if isinstance(content, str) and content:
                content_parts.append(content)
            finish_reason = choice.get("finish_reason")
            if finish_reason:
                t2 = time.perf_counter()
                break
    finally:
        response.close()

    if t2 is None:
        t2 = time.perf_counter()
    response_text = "".join(content_parts) if content_parts else None
    if stream_error:
        return ChatSample(t0=t0, t1=t1, t2=t2, error=stream_error, response_text=response_text)
    if t1 is None:
        return ChatSample(t0=t0, t1=None, t2=t2, error="No assistant content received", response_text=response_text)
    return ChatSample(t0=t0, t1=t1, t2=t2, error=None, response_text=response_text)
