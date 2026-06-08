#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
"""Adaptive context compression for long-running chat sessions.

Monitors token consumption and automatically summarises the oldest turns
when a configurable threshold is crossed. The original message list is
never deleted — only ``compressed_message`` and ``compression_cursor`` are
written, so lossless retrieval via ``include_raw=true`` is always possible.

Compression is serialised per session with a database advisory lock so
concurrent requests cannot trigger duplicate passes.
"""

import logging
import math

from api.db.db_models import DB

logger = logging.getLogger(__name__)

_COMPRESSION_SYSTEM_PROMPT = """\
You are a lossless conversation summariser. Your task is to condense the \
provided chat turns into a compact summary that preserves ALL of the \
following:
- Entity names (people, products, places, IDs)
- Numeric values, dates, measurements
- Decisions that were made and their rationale
- Unresolved questions or open action items
- The user's expressed preferences or constraints

Output ONLY a JSON object with these keys (no markdown fences):
{
  "summary": "<prose paragraph summarising the conversation so far>",
  "key_facts": ["<fact 1>", "<fact 2>", ...],
  "open_questions": ["<unresolved question 1>", ...]
}
"""

_DEFAULT_THRESHOLD_PCT = 0.80
_DEFAULT_WINDOW_PCT = 0.40


def _estimate_tokens(messages: list[dict]) -> int:
    """Estimate total tokens in a list of chat messages.

    Uses ``common.token_utils.num_tokens_from_string`` when available and
    falls back to the ``len / 3.5`` character heuristic for any content
    type that raises (e.g. non-string multimodal content).
    """
    from common.token_utils import num_tokens_from_string

    total = 0
    for m in messages:
        content = m.get("content") or ""
        if not isinstance(content, str):
            content = str(content)
        try:
            total += num_tokens_from_string(content)
        except Exception:
            total += max(1, len(content) // 4)
    return total


class ContextCompressor:
    """Manages rolling summarisation for a single ``Conversation`` record."""

    def __init__(self, conv_id: str, chat_mdl, model_ctx_limit: int, threshold_pct: float = _DEFAULT_THRESHOLD_PCT):
        self.conv_id = conv_id
        self.chat_mdl = chat_mdl
        self.model_ctx_limit = model_ctx_limit
        self.threshold_pct = threshold_pct

    def should_compress(self, token_tally: int) -> bool:
        return token_tally > self.threshold_pct * self.model_ctx_limit

    async def compress(self, messages: list[dict]) -> dict:
        """Summarise ``messages`` into a structured dict via the LLM.

        Returns a dict with keys ``summary``, ``key_facts``, ``open_questions``.
        Falls back to a plain text summary when the LLM returns unparseable JSON.
        """
        import json
        import re

        turns_text = "\n".join(
            f"[{m['role'].upper()}]: {m.get('content', '')}" for m in messages
        )
        user_prompt = f"Conversation to summarise:\n\n{turns_text}\n\nJSON output:"

        try:
            raw = await self.chat_mdl.async_chat(
                _COMPRESSION_SYSTEM_PROMPT,
                [{"role": "user", "content": user_prompt}],
                {"temperature": 0.1, "max_tokens": 1024},
            )
            # Strip leading think blocks from reasoning models
            raw = re.sub(r"^.*</think>", "", raw, flags=re.DOTALL).strip()
            # Strip optional markdown code fences
            raw = re.sub(r"^```(?:json)?\s*|\s*```$", "", raw, flags=re.DOTALL).strip()
            return json.loads(raw)
        except Exception as e:
            logger.warning("ContextCompressor.compress JSON parse failed (%s); using plain summary", e)
            try:
                plain = await self.chat_mdl.async_chat(
                    "Summarise the following conversation in 2–3 sentences, preserving all key facts.",
                    [{"role": "user", "content": turns_text}],
                    {"temperature": 0.1, "max_tokens": 512},
                )
                plain = re.sub(r"^.*</think>", "", plain, flags=re.DOTALL).strip()
                return {"summary": plain, "key_facts": [], "open_questions": []}
            except Exception as e2:
                logger.error("ContextCompressor.compress fallback also failed: %s", e2)
                return {"summary": "", "key_facts": [], "open_questions": []}

    async def rolling_compress(self) -> bool:
        """Identify the oldest uncompressed 40% of turns, summarise them,
        and write the result back to the database atomically.

        The DB advisory lock prevents concurrent requests from triggering
        duplicate passes on the same session.

        Returns ``True`` if compression was performed, ``False`` otherwise.
        """
        from api.db.services.conversation_service import ConversationService

        ok, conv = ConversationService.get_by_id(self.conv_id)
        if not ok:
            logger.warning("rolling_compress: session %s not found", self.conv_id)
            return False

        messages = conv.message or []
        cursor = conv.compression_cursor or 0
        uncompressed = messages[cursor:]

        if len(uncompressed) < 4:
            return False

        window_size = max(2, math.floor(_DEFAULT_WINDOW_PCT * len(uncompressed)))
        # Only compress full user/assistant pairs
        if window_size % 2 != 0:
            window_size -= 1
        if window_size < 2:
            return False

        to_compress = uncompressed[:window_size]
        new_cursor = cursor + window_size

        logger.info(
            "rolling_compress: session=%s cursor=%d→%d window=%d total_msgs=%d",
            self.conv_id, cursor, new_cursor, window_size, len(messages),
        )

        with DB.lock(f"ctx_compress:{self.conv_id}"):
            # Re-fetch inside the lock in case another request won the race
            ok, conv = ConversationService.get_by_id(self.conv_id)
            if not ok or (conv.compression_cursor or 0) >= new_cursor:
                return False

            new_summary = await self.compress(to_compress)

            existing = conv.compressed_message
            if existing and existing.get("summary"):
                combined_summary = existing["summary"] + "\n\n" + new_summary.get("summary", "")
                combined_facts = list({*existing.get("key_facts", []), *new_summary.get("key_facts", [])})
                combined_questions = list({*existing.get("open_questions", []), *new_summary.get("open_questions", [])})
                new_summary = {
                    "summary": combined_summary,
                    "key_facts": combined_facts,
                    "open_questions": combined_questions,
                }

            ConversationService.update_by_id(self.conv_id, {
                "compressed_message": new_summary,
                "compression_cursor": new_cursor,
            })
            return True

    def get_effective_history(self, conv) -> list[dict]:
        """Return the message list to pass to the LLM.

        Replaces turns before ``compression_cursor`` with a single synthetic
        assistant message carrying the compressed summary so the LLM has
        full context in a small token footprint.
        """
        messages = conv.message or []
        cursor = conv.compression_cursor or 0
        summary = conv.compressed_message

        if cursor == 0 or not summary or not summary.get("summary"):
            return messages

        summary_content = summary["summary"]
        if summary.get("key_facts"):
            summary_content += "\n\nKey facts:\n" + "\n".join(f"- {f}" for f in summary["key_facts"])
        if summary.get("open_questions"):
            summary_content += "\n\nOpen questions:\n" + "\n".join(f"- {q}" for q in summary["open_questions"])

        synthetic = {
            "role": "assistant",
            "content": f"[Conversation summary up to turn {cursor}]\n{summary_content}",
        }
        return [synthetic] + messages[cursor:]
