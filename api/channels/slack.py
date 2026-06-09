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

import asyncio
import logging

from api.channels.base import Channel, IncomingMessage, OutgoingMessage
from api.channels.registry import channel

logger = logging.getLogger(__name__)


@channel("slack")
class SlackChannel(Channel):
    """Slack channel integration using Socket Mode (no public webhook required).

    Expected config keys under ``credential``:
      - bot_token   (xoxb-...)
      - app_token   (xapp-...)

    ``dialog_id`` must be set in config to identify which RAGFlow chatbot to use.
    """

    def __init__(self, tenant_id: str, config: dict):
        super().__init__(tenant_id, config)
        cred = config.get("credential", {})
        self._bot_token: str = cred.get("bot_token", "")
        self._app_token: str = cred.get("app_token", "")
        self._dialog_id: str = config.get("dialog_id", "")
        self._web_client = None
        self._socket_client = None
        self._socket_task: asyncio.Task | None = None
        # user_id -> session_id for conversation continuity
        self._sessions: dict[str, str] = {}

    async def start(self):
        """Create the Slack clients and start listening for events."""
        from slack_sdk.web.async_client import AsyncWebClient
        from slack_sdk.socket_mode.aiohttp import SocketModeClient
        from slack_sdk.socket_mode.response import SocketModeResponse

        self._web_client = AsyncWebClient(token=self._bot_token)
        self._socket_client = SocketModeClient(
            app_token=self._app_token,
            web_client=self._web_client,
        )

        async def _handle_event(client, req):
            if req.type == "events_api":
                payload = req.payload
                event = payload.get("event", {})
                # Acknowledge immediately to avoid Slack retries
                await client.send_socket_mode_response(
                    SocketModeResponse(envelope_id=req.envelope_id)
                )
                # Only handle genuine user messages (ignore bot echoes)
                if event.get("type") == "message" and not event.get("bot_id"):
                    text = event.get("text", "").strip()
                    user_id = event.get("user", "")
                    slack_channel_id = event.get("channel", "")
                    if text and user_id:
                        incoming = IncomingMessage(
                            content=text,
                            sender_id=user_id,
                            channel_id=slack_channel_id,
                            raw=event,
                        )
                        reply = await self._dispatch(incoming)
                        if reply:
                            await self.send(reply)

        self._socket_client.socket_mode_request_listeners.append(_handle_event)

        self._socket_task = asyncio.create_task(self._socket_client.connect())
        logger.info("SlackChannel started for tenant=%s dialog=%s", self.tenant_id, self._dialog_id)

    async def stop(self):
        """Disconnect the socket client and cancel the background task."""
        try:
            if self._socket_client:
                await self._socket_client.disconnect()
        except Exception:
            logger.exception("SlackChannel: error during socket disconnect")
        if self._socket_task and not self._socket_task.done():
            self._socket_task.cancel()
            try:
                await self._socket_task
            except asyncio.CancelledError:
                pass
        logger.info("SlackChannel stopped for tenant=%s", self.tenant_id)

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
            logger.exception("SlackChannel._dispatch failed for sender=%s", incoming.sender_id)
            return None

        if not answer_text:
            return None

        return OutgoingMessage(
            content=answer_text,
            recipient_id=incoming.sender_id,
            channel_id=incoming.channel_id,
        )

    async def send(self, outgoing: OutgoingMessage):
        """Post a message to the originating Slack channel."""
        if not self._web_client:
            logger.error("SlackChannel.send called before start()")
            return
        try:
            await self._web_client.chat_postMessage(
                channel=outgoing.channel_id,
                text=outgoing.content,
            )
        except Exception:
            logger.exception("SlackChannel.send failed for channel=%s", outgoing.channel_id)
