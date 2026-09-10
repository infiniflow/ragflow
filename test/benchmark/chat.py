import json
import time
from typing import Any, Dict, List, Optional

import requests

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
    if dataset_ids is not None and "kb_ids" not in body:
        body["kb_ids"] = dataset_ids
    res = client.request_json("POST", "/chats", json_body=body)
    if res.get("code") != 0:
        raise ChatError(f"Create chat failed: {res.get('message')}")
    return res.get("data", {})


def get_chat(client: HttpClient, chat_id: str) -> Dict[str, Any]:
    res = client.request_json("GET", f"/chats/{chat_id}")
    if res.get("code") != 0:
        raise ChatError(f"Get chat failed: {res.get('message')}")
    data = res.get("data", {})
    if not data:
        raise ChatError("Chat not found")
    return data


def resolve_model(model: Optional[str], chat_data: Optional[Dict[str, Any]]) -> str:
    if model:
        return model
    if chat_data:
        llm_id = chat_data.get("llm_id")
        if llm_id:
            return llm_id
    raise ChatError("Model name is required; provide --model or use a chat with llm_id.")


def _parse_stream_error(response) -> Optional[str]:
    if not 200 <= response.status_code < 300:
        return f"HTTP {response.status_code}"
    content_type = response.headers.get("Content-Type", "")
    if "text/event-stream" in content_type:
        return None
    try:
        payload = response.json()
    except ValueError:
        return f"Unexpected non-stream response (status {response.status_code})"
    if isinstance(payload, dict) and payload.get("code") not in (0, None):
        return str(payload.get("message") or f"Chat failed (code {payload['code']})")
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
    response = None
    t1: Optional[float] = None
    t2: Optional[float] = None
    stream_error: Optional[str] = None
    completed = False
    content_parts: List[str] = []
    try:
        response = client.request(
            "POST",
            f"/openai/{chat_id}/chat/completions",
            json_body=payload,
            stream=True,
        )
        error = _parse_stream_error(response)
        if error:
            return ChatSample(t0=t0, t1=None, t2=time.perf_counter(), error=error)
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
                completed = True
                t2 = time.perf_counter()
                break
            try:
                chunk = json.loads(data)
            except ValueError as exc:
                stream_error = f"Invalid JSON chunk: {exc}"
                t2 = time.perf_counter()
                break
            if not isinstance(chunk, dict):
                stream_error = "Invalid stream chunk: expected an object"
                break
            if chunk.get("error") is not None or chunk.get("code") not in (0, None):
                stream_error = "Chat stream reported an error"
                break
            choices = chunk.get("choices") or []
            if not isinstance(choices, list) or (choices and not isinstance(choices[0], dict)):
                stream_error = "Invalid stream chunk: expected choices to be a list of objects"
                break
            choice = choices[0] if choices else {}
            delta = choice.get("delta") or {}
            if not isinstance(delta, dict):
                stream_error = "Invalid stream chunk: expected delta to be an object"
                break
            content = delta.get("content")
            if t1 is None and isinstance(content, str) and content != "":
                t1 = time.perf_counter()
            if isinstance(content, str) and content:
                content_parts.append(content)
            finish_reason = choice.get("finish_reason")
            if finish_reason:
                completed = True
                t2 = time.perf_counter()
                break
    except requests.RequestException as exc:
        stream_error = f"Transport error ({type(exc).__name__})"
    finally:
        if response is not None:
            response.close()

    if t2 is None:
        t2 = time.perf_counter()
    response_text = "".join(content_parts) if content_parts else None
    if stream_error:
        return ChatSample(t0=t0, t1=t1, t2=t2, error=stream_error, response_text=response_text)
    if t1 is None:
        return ChatSample(t0=t0, t1=None, t2=t2, error="No assistant content received", response_text=response_text)
    if not completed:
        return ChatSample(t0=t0, t1=t1, t2=t2, error="Stream ended before a completion marker", response_text=response_text)
    return ChatSample(t0=t0, t1=t1, t2=t2, error=None, response_text=response_text)
