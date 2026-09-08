from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import Dict, Optional, Tuple

from aiohttp import web
from linebot.v3 import WebhookParser
from linebot.v3.exceptions import InvalidSignatureError
from linebot.v3.messaging import (
    AsyncApiClient,
    AsyncMessagingApi,
    Configuration,
    PushMessageRequest,
    ReplyMessageRequest,
    TextMessage,
)
from linebot.v3.webhooks import (
    GroupSource,
    MessageEvent,
    RoomSource,
    TextMessageContent,
    UserSource,
)

from ..core.base import Channel, IncomingMessage, OutgoingMessage
from ..core.registry import register_channel

LOGGER = logging.getLogger(__name__)


@dataclass
class LineAccount:
    account_id: str
    channel_secret: str
    channel_access_token: str
    webhook_host: str = "0.0.0.0"
    webhook_port: int = 3001


class _SharedWebhookServer:
    def __init__(self, host: str, port: int) -> None:
        self.host = host
        self.port = port
        self.app = web.Application()
        self.app.router.add_post("/line/{account_id}/webhook", self._handle_request)
        self.runner: Optional[web.AppRunner] = None
        self.site: Optional[web.TCPSite] = None
        self.channels: Dict[str, "LineChannel"] = {}

    async def start(self) -> None:
        if self.runner is not None:
            return
        self.runner = web.AppRunner(self.app)
        await self.runner.setup()
        self.site = web.TCPSite(self.runner, self.host, self.port)
        await self.site.start()
        LOGGER.info(
            "[line] webhook listening on http://%s:%s/line/<account_id>/webhook",
            self.host,
            self.port,
        )

    async def stop(self) -> None:
        if self.site is not None:
            await self.site.stop()
        if self.runner is not None:
            await self.runner.cleanup()
        self.runner = None
        self.site = None

    async def _handle_request(self, request: web.Request) -> web.Response:
        account_id = request.match_info.get("account_id", "")
        try:
            body = await request.text()
            signature = request.headers.get("x-line-signature", "")
            channel = self.channels.get(account_id)
            if channel is None:
                return web.Response(status=404, text="unknown account")
            try:
                events = channel.parser.parse(body, signature)
            except InvalidSignatureError:
                return web.Response(status=403, text="bad signature")
            for event in events:
                try:
                    await channel.handle_event(event)
                except Exception:
                    LOGGER.error("[line:%s] event handling error", account_id, exc_info=True)
        except Exception:
            LOGGER.error("[line:%s] inbound request handling error", account_id, exc_info=True)
        return web.Response(status=200, text="ok")


_servers: Dict[Tuple[str, int], _SharedWebhookServer] = {}
_active_per_server: Dict[Tuple[str, int], int] = {}


async def _acquire_server(host: str, port: int) -> _SharedWebhookServer:
    key = (host, port)
    server = _servers.get(key)
    if server is None:
        server = _SharedWebhookServer(host, port)
        _servers[key] = server
        await server.start()
    _active_per_server[key] = _active_per_server.get(key, 0) + 1
    return server


async def _release_server(host: str, port: int) -> None:
    key = (host, port)
    remaining = _active_per_server.get(key, 0) - 1
    _active_per_server[key] = remaining
    if remaining <= 0:
        server = _servers.pop(key, None)
        _active_per_server.pop(key, None)
        if server is not None:
            await server.stop()


def _chat_type_and_id(source) -> Tuple[str, str]:
    if isinstance(source, GroupSource):
        return ("group", source.group_id or "")
    if isinstance(source, RoomSource):
        return ("group", source.room_id or "")
    if isinstance(source, UserSource):
        return ("p2p", source.user_id or "")
    return (type(source).__name__, getattr(source, "user_id", "") or "")


class LineChannel(Channel):
    channel_id = "line"

    def __init__(self, account: LineAccount) -> None:
        super().__init__()
        self.account = account
        self.account_id = account.account_id
        self.parser = WebhookParser(account.channel_secret)
        self._config = Configuration(access_token=account.channel_access_token)
        self._server: Optional[_SharedWebhookServer] = None
        # LINE reply tokens are single-use and expire ~30s after the event.
        self._reply_tokens: Dict[str, str] = {}

    async def start(self) -> None:
        self._server = await _acquire_server(self.account.webhook_host, self.account.webhook_port)
        self._server.channels[self.account_id] = self
        LOGGER.info(
            "[line:%s] registered at path /line/%s/webhook",
            self.account_id,
            self.account_id,
        )

    async def stop(self) -> None:
        if self._server is not None:
            self._server.channels.pop(self.account_id, None)
            await _release_server(self.account.webhook_host, self.account.webhook_port)
            self._server = None

    async def handle_event(self, event) -> None:
        try:
            if not isinstance(event, MessageEvent):
                return
            content = event.message
            if not isinstance(content, TextMessageContent):
                return
            chat_type, chat_id = _chat_type_and_id(event.source)
            sender_id = getattr(event.source, "user_id", "") or ""
            if event.reply_token:
                self._reply_tokens[content.id] = event.reply_token
            incoming = IncomingMessage(
                channel=self.channel_id,
                account_id=self.account_id,
                chat_id=chat_id,
                chat_type=chat_type,
                message_id=content.id,
                sender_id=sender_id,
                text=content.text or "",
                raw=event,
            )
            await self._dispatch(incoming)
        except Exception:
            LOGGER.error("[line:%s] inbound message handling error", self.account_id, exc_info=True)

    async def send(self, message: OutgoingMessage) -> None:
        reply_token: Optional[str] = None
        if message.reply_to_message_id:
            reply_token = self._reply_tokens.pop(message.reply_to_message_id, None)

        try:
            async with AsyncApiClient(self._config) as api_client:
                api = AsyncMessagingApi(api_client)
                if reply_token:
                    await api.reply_message(
                        ReplyMessageRequest(
                            reply_token=reply_token,
                            messages=[TextMessage(text=message.text)],
                        )
                    )
                else:
                    if not message.chat_id:
                        LOGGER.error("[line:%s] no chat_id for push send", self.account_id)
                        return
                    await api.push_message(
                        PushMessageRequest(
                            to=message.chat_id,
                            messages=[TextMessage(text=message.text)],
                        )
                    )
        except Exception:
            LOGGER.error("[line:%s] send failed", self.account_id, exc_info=True)


def _build(account_id: str, cfg: dict) -> Channel:
    channel_secret = cfg.get("channel_secret")
    channel_access_token = cfg.get("channel_access_token")
    if not channel_secret or not channel_access_token:
        raise ValueError(f"line account '{account_id}' missing channel_secret or channel_access_token")
    return LineChannel(
        LineAccount(
            account_id=account_id,
            channel_secret=str(channel_secret),
            channel_access_token=str(channel_access_token),
            webhook_host=str(cfg.get("webhook_host", "0.0.0.0")),
            webhook_port=int(cfg.get("webhook_port", 3001)),
        )
    )


register_channel("line", _build)
