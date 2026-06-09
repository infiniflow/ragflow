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

import hashlib
import hmac
import json
import logging
import time

import aiohttp

from api.channels.base import Channel, IncomingMessage, OutgoingMessage
from api.channels.registry import channel

logger = logging.getLogger(__name__)

_FEISHU_API_BASE = "https://open.feishu.cn/open-apis"


@channel("feishu")
class FeishuChannel(Channel):
    """Feishu (Lark) channel integration using webhook callbacks.

    Expected config keys (top-level in config dict):
      - app_id
      - app_secret
      - encrypt_key   (optional, for payload decryption)

    ``dialog_id`` must be set in config to identify which RAGFlow chatbot to use.
    """

    def __init__(self, tenant_id: str, config: dict):
        super().__init__(tenant_id, config)
        self._app_id: str = config.get("app_id", "")
        self._app_secret: str = config.get("app_secret", "")
        self._encrypt_key: str = config.get("encrypt_key", "")
        self._dialog_id: str = config.get("dialog_id", "")
        if not self._dialog_id:
            raise ValueError("FeishuChannel requires non-empty dialog_id in config")
        self._token_cache: tuple[str, float] | None = None  # (token, expiry_timestamp)
        self._session: aiohttp.ClientSession | None = None
        # sender_id -> session_id for conversation continuity
        self._sessions: dict[str, str] = {}

    async def start(self):
        self._session = aiohttp.ClientSession()
        logger.info("FeishuChannel started for tenant=%s dialog=%s", self.tenant_id, self._dialog_id)

    async def stop(self):
        if self._session:
            await self._session.close()
            self._session = None
        logger.info("FeishuChannel stopped for tenant=%s", self.tenant_id)

    async def handle_webhook(self, payload: dict) -> dict:
        """Entry point called by the Quart webhook route.

        Verifies the payload, dispatches the message, and returns the
        Feishu-compatible JSON response.
        """
        # Handle Feishu URL verification challenge
        if payload.get("type") == "url_verification":
            return {"challenge": payload.get("challenge", "")}

        event = payload.get("event", {})
        msg = event.get("message", {})
        sender = event.get("sender", {})

        sender_id = sender.get("sender_id", {}).get("open_id", "")
        msg_type = msg.get("message_type", "")

        if msg_type != "text":
            return {}

        try:
            content_json = json.loads(msg.get("content", "{}"))
            text = content_json.get("text", "").strip()
        except (json.JSONDecodeError, AttributeError):
            return {}

        if not text or not sender_id:
            return {}

        incoming = IncomingMessage(
            content=text,
            sender_id=sender_id,
            channel_id=str(id(self)),
            raw=payload,
        )
        reply = await self._dispatch(incoming)
        if reply:
            await self.send(reply)
        return {}

    async def _dispatch(self, incoming: IncomingMessage) -> OutgoingMessage | None:
        """Call the RAGFlow dialog completion and return the answer."""
        from api.db.services.conversation_service import async_iframe_completion

        session_id = self._sessions.get(incoming.sender_id)
        answer_text = ""
        try:
            req = {
                "question": incoming.content,
                "stream": False,
                "session_id": session_id,
            }
            async for chunk in async_iframe_completion(self._dialog_id, tenant_id=self.tenant_id, **req):
                if isinstance(chunk, dict):
                    answer_text = chunk.get("answer", "")
                    new_session_id = chunk.get("session_id")
                    if new_session_id:
                        self._sessions[incoming.sender_id] = new_session_id
        except Exception:
            logger.exception("FeishuChannel._dispatch failed for sender=%s", incoming.sender_id)
            return None

        if not answer_text:
            return None

        return OutgoingMessage(
            content=answer_text,
            recipient_id=incoming.sender_id,
            channel_id=incoming.channel_id,
        )

    async def send(self, outgoing: OutgoingMessage):
        """Post a text message to the Feishu open platform."""
        token = await self._get_tenant_access_token()
        if not token:
            logger.error("FeishuChannel.send: could not obtain access token")
            return

        url = f"{_FEISHU_API_BASE}/im/v1/messages?receive_id_type=open_id"
        payload = {
            "receive_id": outgoing.recipient_id,
            "msg_type": "text",
            "content": json.dumps({"text": outgoing.content}),
        }
        headers = {
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json; charset=utf-8",
        }
        try:
            async with self._session.post(url, json=payload, headers=headers) as resp:
                if resp.status != 200:
                    body = await resp.text()
                    logger.error("FeishuChannel.send HTTP %d: %s", resp.status, body)
        except Exception:
            logger.exception("FeishuChannel.send failed for recipient=%s", outgoing.recipient_id)

    async def _get_tenant_access_token(self) -> str | None:
        """Fetch a short-lived tenant access token from Feishu, with caching and auto-refresh."""
        now = time.time()
        if self._token_cache is not None:
            token, expiry = self._token_cache
            if now < expiry - 60:
                return token

        url = f"{_FEISHU_API_BASE}/auth/v3/tenant_access_token/internal"
        payload = {"app_id": self._app_id, "app_secret": self._app_secret}
        try:
            async with self._session.post(url, json=payload) as resp:
                data = await resp.json()
                token = data.get("tenant_access_token")
                if not token:
                    logger.error("FeishuChannel: no tenant_access_token in response")
                    return None
                expire = data.get("expire", 7200)
                self._token_cache = (token, now + expire)
                return token
        except Exception:
            logger.exception("FeishuChannel: failed to fetch tenant access token")
            return None

    def verify_signature(self, timestamp: str, nonce: str, body: str, signature: str) -> bool:
        """Verify an incoming webhook signature using the encrypt_key."""
        if not self._encrypt_key:
            return True
        content = (timestamp + nonce + self._encrypt_key + body).encode()
        digest = hmac.new(self._encrypt_key.encode(), content, hashlib.sha256).hexdigest()
        return hmac.compare_digest(digest, signature)
