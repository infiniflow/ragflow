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

"""Per-request identifiers forwarded to upstream LLM providers.

An agent run (or any LLM-issuing flow) installs the originating ``session_id`` /
``user_id`` here. The chat model layer reads it and forwards an end-user
identifier as the OpenAI-standard ``user`` request field, which providers such as
OpenAI and OpenRouter include in the request body. This lets upstream activity be
correlated back to the session/user that produced it.

The value is a small dict (or ``None`` when no request context is active), e.g.
``{"session_id": "...", "user_id": "..."}``.
"""

import contextvars
import logging

llm_request_context: contextvars.ContextVar = contextvars.ContextVar("ragflow_llm_request_context", default=None)


def set_llm_request_context(session_id: str | None = None, user_id: str | None = None):
    """Install the current request identifiers and return the reset token.

    Pass the returned token to ``reset_llm_request_context`` (typically in a
    ``finally`` block) so the value does not leak to later calls in the same task.
    """
    ctx = {}
    if session_id:
        ctx["session_id"] = str(session_id)[:128]
    if user_id:
        ctx["user_id"] = str(user_id)[:128]
    # Log only presence flags, never the raw identifiers.
    logging.debug("Installing LLM request context (session=%s, user=%s)", bool(session_id), bool(user_id))
    return llm_request_context.set(ctx or None)


def reset_llm_request_context(token) -> None:
    try:
        llm_request_context.reset(token)
    except (ValueError, RuntimeError):
        # The context may be reset from a different context (e.g. an async generator
        # closed on client disconnect -> ValueError) or with an already-consumed
        # token (Python 3.13+ -> RuntimeError); fall back to clearing the value.
        logging.debug("LLM request context reset failed; clearing active context", exc_info=True)
        llm_request_context.set(None)


def current_llm_user() -> str | None:
    """Return the identifier to forward as the provider ``user`` field.

    Prefers ``session_id`` (so upstream activity can be traced per chat session),
    falling back to ``user_id``. Returns ``None`` when no context is active.
    """
    ctx = llm_request_context.get()
    if not ctx:
        return None
    return ctx.get("session_id") or ctx.get("user_id") or None
