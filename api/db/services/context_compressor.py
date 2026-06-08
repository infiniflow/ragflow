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
"""Rolling context compression for long RAG conversations.

When the cumulative token tally for a session exceeds a configurable fraction
of the model's context limit, the oldest 40% of uncompressed turns are
summarised by the same chat model and replaced with a synthetic summary
message.  The summary is persisted to the ``conversation`` table under
``compressed_message`` and the ``compression_cursor`` field tracks which turns
have already been compressed so they are never re-processed.
"""
import json
import logging
import re

from api.db.db_models import DB, Conversation
from common.token_utils import num_tokens_from_string

logger = logging.getLogger(__name__)

_COMPRESSION_SYSTEM_PROMPT = """\
You are a precise conversation summariser.  Given the conversation history
below, produce a JSON object with exactly three keys:
- "summary": a concise prose summary of what was discussed (≤200 words)
- "key_facts": a list of short factual statements established in the conversation
- "open_questions": a list of questions that remain unanswered

Respond with only the JSON object, no markdown fences.\
"""


def _estimate_tokens(messages: list) -> int:
    total = 0
    for m in messages:
        content = m.get("content", "")
        if isinstance(content, str):
            try:
                total += num_tokens_from_string(content)
            except Exception:
                total += len(content) // 4
    return total


class ContextCompressor:
    """Manages rolling summarisation for a single conversation."""

    def __init__(self, conv_id: str, chat_mdl, model_ctx_limit: int = 8192,
                 threshold_pct: float = 0.80):
        self.conv_id = conv_id
        self.chat_mdl = chat_mdl
        self.model_ctx_limit = model_ctx_limit
        self.threshold_pct = threshold_pct
        self._threshold_tokens = int(model_ctx_limit * threshold_pct)

    def should_compress(self, token_tally: int) -> bool:
        return token_tally >= self._threshold_tokens

    async def compress(self, messages: list) -> dict:
        """Summarise ``messages`` via the chat model.

        Returns a dict with keys ``summary``, ``key_facts``, ``open_questions``.
        Falls back gracefully on parse errors.
        """
        history_text = "\n".join(
            f"{m.get('role', 'unknown').upper()}: {m.get('content', '')}"
            for m in messages
        )
        user_msg = f"Conversation to summarise:\n\n{history_text}"

        try:
            raw = await self.chat_mdl.async_chat(
                _COMPRESSION_SYSTEM_PROMPT,
                [{"role": "user", "content": user_msg}],
                {"temperature": 0.2, "max_tokens": 1024},
            )
            raw = re.sub(r"^.*</think>", "", raw, flags=re.DOTALL).strip()
            stripped = re.sub(r"^```(?:json)?\s*|\s*```$", "", raw, flags=re.DOTALL).strip()
            result = json.loads(stripped)
            if not isinstance(result, dict):
                raise ValueError("LLM returned non-dict JSON")
            return {
                "summary": str(result.get("summary", "")),
                "key_facts": list(result.get("key_facts", [])),
                "open_questions": list(result.get("open_questions", [])),
            }
        except Exception as e:
            logger.warning("context_compressor compress fallback for conv %s: %s", self.conv_id, e)
            short = " ".join(
                str(m.get("content", ""))[:100] for m in messages[:5]
            )
            return {"summary": short, "key_facts": [], "open_questions": []}

    async def rolling_compress(self) -> bool:
        """Identify and compress the oldest 40% uncompressed turns for this conversation.

        Uses an advisory DB lock to prevent concurrent compression of the same
        session.  Returns True if compression was performed, False otherwise.
        """
        lock_name = f"ctx_compress:{self.conv_id}"
        with DB.lock(lock_name):
            try:
                conv = Conversation.get_by_id(self.conv_id)
            except Conversation.DoesNotExist:
                return False

            messages: list = conv.message or []
            cursor: int = conv.compression_cursor or 0

            # Only consider messages after the current cursor (already-uncompressed)
            uncompressed = messages[cursor:]

            # Need at least 2 turns (user+assistant pairs) before compressing
            if len(uncompressed) < 4:
                return False

            window_size = max(2, int(len(uncompressed) * 0.40))
            # Round down to nearest even number so we compress whole turn pairs
            window_size = (window_size // 2) * 2
            to_compress = messages[cursor: cursor + window_size]

            summary_payload = await self.compress(to_compress)

            existing = conv.compressed_message or {}
            merged_summary = (
                (existing.get("summary", "") + "\n\n" + summary_payload["summary"]).strip()
                if existing.get("summary") else summary_payload["summary"]
            )
            merged_facts = list(existing.get("key_facts", [])) + summary_payload["key_facts"]
            merged_questions = list(existing.get("open_questions", [])) + summary_payload["open_questions"]

            new_payload = {
                "summary": merged_summary,
                "key_facts": merged_facts,
                "open_questions": merged_questions,
            }
            new_cursor = cursor + window_size

            Conversation.update(
                compressed_message=new_payload,
                compression_cursor=new_cursor,
            ).where(Conversation.id == self.conv_id).execute()

            return True

    def get_effective_history(self, conv: "Conversation") -> list:
        """Return the effective message list for the LLM.

        If there is a compressed summary, a synthetic system message is prepended
        summarising the compressed portion, followed by messages from the cursor
        onward.
        """
        messages: list = conv.message or []
        cursor: int = conv.compression_cursor or 0
        compressed: dict | None = conv.compressed_message

        recent = messages[cursor:]

        if not compressed:
            return recent

        parts = [compressed.get("summary", "")]
        key_facts = compressed.get("key_facts", [])
        if key_facts:
            parts.append("Key facts established:\n" + "\n".join(f"- {f}" for f in key_facts))
        open_q = compressed.get("open_questions", [])
        if open_q:
            parts.append("Open questions:\n" + "\n".join(f"- {q}" for q in open_q))

        synthetic = {
            "role": "system",
            "content": "[Earlier conversation summary]\n\n" + "\n\n".join(parts),
        }
        return [synthetic] + recent
