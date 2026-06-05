#!/usr/bin/env python3
"""Standalone checks for PR #15507 download_attachment behavior (no full app import)."""

from __future__ import annotations

import asyncio
from enum import IntEnum


class RetCode(IntEnum):
    DATA_ERROR = 102


def get_data_error_result(message=""):
    return {"code": int(RetCode.DATA_ERROR), "message": message}


async def thread_pool_exec(func, *args, **kwargs):
    return func(*args, **kwargs)


class _DummyResponse:
    def __init__(self, data):
        self.data = data
        self.headers = {}


CONTENT_TYPE_MAP = {"pdf": "application/pdf", "markdown": "text/markdown"}


def apply_safe_file_response_headers(response, content_type, extension):
    response.headers["content_type"] = content_type
    response.headers["extension"] = extension


# --- Logic under test (mirrors agent_api.py on fix/15502-agent-attachment-auth) ---

def _agent_attachment_accessible(attachment_id, user_id, *, accessible_fn, in_conversation_fn):
    if not attachment_id:
        return False
    if accessible_fn(attachment_id, user_id):
        return True
    return in_conversation_fn(attachment_id, user_id)


async def download_attachment(
    tenant_id,
    attachment_id,
    *,
    current_user_id,
    accessible_fn,
    in_conversation_fn,
    storage_get,
    ext="markdown",
):
    try:
        doc_id = attachment_id
        if not await thread_pool_exec(
            lambda: _agent_attachment_accessible(
                doc_id, current_user_id, accessible_fn=accessible_fn, in_conversation_fn=in_conversation_fn
            )
        ):
            return get_data_error_result(message="Document not found!")

        data = await thread_pool_exec(storage_get, tenant_id, doc_id)
        if not data:
            return get_data_error_result(message="Document not found!")
        response = _DummyResponse(data)
        content_type = CONTENT_TYPE_MAP.get(ext, f"application/{ext}")
        apply_safe_file_response_headers(response, content_type, ext)
        return response
    except Exception as e:
        return {"code": 500, "message": str(e)}


def _run(coro):
    return asyncio.run(coro)


def main() -> int:
    results: list[tuple[str, bool, str]] = []

    def record(name: str, ok: bool, detail: str = "ok"):
        results.append((name, ok, detail))
        print(f"{'PASS' if ok else 'FAIL'}\t{name}\t{detail}")

    # A — unauthorized
    res = _run(
        download_attachment(
            "tenant-1",
            "att-a",
            current_user_id="user-b",
            accessible_fn=lambda *_: False,
            in_conversation_fn=lambda *_: False,
            storage_get=lambda *_: b"secret",
        )
    )
    record(
        "A_unauthorized_denied",
        res["code"] == RetCode.DATA_ERROR and "Document not found!" in res["message"],
        str(res),
    )

    # B — authorized (document path)
    res = _run(
        download_attachment(
            "tenant-1",
            "att-b",
            current_user_id="user-a",
            accessible_fn=lambda *_: True,
            in_conversation_fn=lambda *_: False,
            storage_get=lambda *_: b"%PDF-1.4",
            ext="pdf",
        )
    )
    record(
        "B_authorized_success",
        isinstance(res, _DummyResponse) and res.data == b"%PDF-1.4" and res.headers["content_type"] == "application/pdf",
        type(res).__name__,
    )

    # C — missing blob
    res = _run(
        download_attachment(
            "tenant-1",
            "att-c",
            current_user_id="user-a",
            accessible_fn=lambda *_: True,
            in_conversation_fn=lambda *_: False,
            storage_get=lambda *_: None,
        )
    )
    record(
        "C_missing_blob_4xx",
        res["code"] == RetCode.DATA_ERROR and "Document not found!" in res["message"],
        str(res),
    )

    # D — conversation fallback (no document row)
    calls = []

    def accessible(doc_id, user_id):
        calls.append(("doc", doc_id, user_id))
        return False

    def in_conv(doc_id, user_id):
        calls.append(("conv", doc_id, user_id))
        return True

    res = _run(
        download_attachment(
            "tenant-1",
            "att-d",
            current_user_id="user-a",
            accessible_fn=accessible,
            in_conversation_fn=in_conv,
            storage_get=lambda *_: b"runtime-output",
        )
    )
    record(
        "D_conversation_fallback",
        isinstance(res, _DummyResponse)
        and res.data == b"runtime-output"
        and calls == [("doc", "att-d", "user-a"), ("conv", "att-d", "user-a")],
        str(calls),
    )

    # E — regression: no 500 on empty (would be make_response(None) before fix)
    res = _run(
        download_attachment(
            "tenant-1",
            "att-e",
            current_user_id="user-a",
            accessible_fn=lambda *_: True,
            in_conversation_fn=lambda *_: False,
            storage_get=lambda *_: None,
        )
    )
    record(
        "E_no_500_on_empty",
        not (isinstance(res, dict) and res.get("code") == 500),
        str(res),
    )

    failed = [r for r in results if not r[1]]
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
