from __future__ import annotations

import asyncio
import logging
import time
from dataclasses import dataclass
from typing import Dict, Optional, Tuple

import aiohttp
from aiohttp import web
from wechatpy.enterprise import parse_message
from wechatpy.enterprise.crypto import WeChatCrypto
from wechatpy.exceptions import InvalidSignatureException

from ..core.base import Channel, IncomingMessage, OutgoingMessage
from ..core.registry import register_channel

LOGGER = logging.getLogger(__name__)

WECOM_API_BASE = "https://qyapi.weixin.qq.com/cgi-bin"


@dataclass
class WeComAccount:
    account_id: str
    corp_id: str
    agent_id: int
    secret: str
    token: str
    aes_key: str
    webhook_host: str = "0.0.0.0"
    webhook_port: int = 3002


class _SharedWebhookServer:
    """Single aiohttp server shared by all WeComChannel instances."""

    def __init__(self, host: str, port: int) -> None:
        self.host = host
        self.port = port
        self.app = web.Application()
        self.app.router.add_get("/wecom/{account_id}/callback", self._handle_request)
        self.app.router.add_post("/wecom/{account_id}/callback", self._handle_request)
        self.runner: Optional[web.AppRunner] = None
        self.site: Optional[web.TCPSite] = None
        self.channels: Dict[str, "WeComChannel"] = {}

    async def start(self) -> None:
        if self.runner is not None:
            return
        self.runner = web.AppRunner(self.app)
        await self.runner.setup()
        self.site = web.TCPSite(self.runner, self.host, self.port)
        await self.site.start()
        LOGGER.info(
            "[wecom] webhook listening on http://%s:%s/wecom/<account_id>/callback",
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
            channel = self.channels.get(account_id)
            if channel is None:
                return web.Response(status=404, text="unknown account")

            signature = request.query.get("msg_signature", "")
            timestamp = request.query.get("timestamp", "")
            nonce = request.query.get("nonce", "")

            # GET = URL verification on first save in the WeCom admin console.
            if request.method == "GET":
                echo_str = request.query.get("echostr", "")
                try:
                    decrypted = channel.crypto.check_signature(
                        signature, timestamp, nonce, echo_str
                    )
                    return web.Response(text=decrypted)
                except InvalidSignatureException:
                    return web.Response(status=403, text="bad signature")

            # POST = encrypted inbound event.
            body = await request.text()
            try:
                xml = channel.crypto.decrypt_message(body, signature, timestamp, nonce)
            except InvalidSignatureException:
                return web.Response(status=403, text="bad signature")
            try:
                msg = parse_message(xml)
            except Exception:
                LOGGER.error("[wecom:%s] parse error", account_id, exc_info=True)
                return web.Response(text="")

            try:
                await channel.handle_decrypted_message(msg)
            except Exception:
                LOGGER.error("[wecom:%s] handler error", account_id, exc_info=True)
        except Exception:
            LOGGER.error("[wecom:%s] inbound request handling error", account_id, exc_info=True)
        # Empty 200 OK tells WeCom we accepted the event.
        return web.Response(text="")


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


class WeComChannel(Channel):
    channel_id = "wecom"

    def __init__(self, account: WeComAccount) -> None:
        super().__init__()
        self.account = account
        self.account_id = account.account_id
        self.crypto = WeChatCrypto(
            account.token, account.aes_key, account.corp_id
        )
        self._server: Optional[_SharedWebhookServer] = None
        self._access_token: Optional[str] = None
        self._access_token_expires_at: float = 0.0
        self._access_token_lock = asyncio.Lock()

    async def start(self) -> None:
        self._server = await _acquire_server(
            self.account.webhook_host, self.account.webhook_port
        )
        self._server.channels[self.account_id] = self
        LOGGER.info(
            "[wecom:%s] registered at path /wecom/%s/callback (agent_id=%s)",
            self.account_id,
            self.account_id,
            self.account.agent_id,
        )

    async def stop(self) -> None:
        if self._server is not None:
            self._server.channels.pop(self.account_id, None)
            await _release_server(
                self.account.webhook_host, self.account.webhook_port
            )
            self._server = None

    async def handle_decrypted_message(self, msg) -> None:
        try:
            # Only handle plain text events; ignore image/voice/event etc.
            if getattr(msg, "type", "") != "text":
                return
            user_id = str(getattr(msg, "source", "") or "")
            if not user_id:
                return
            incoming = IncomingMessage(
                channel=self.channel_id,
                account_id=self.account_id,
                chat_id=user_id,
                chat_type="p2p",
                message_id=str(getattr(msg, "id", "") or ""),
                sender_id=user_id,
                text=getattr(msg, "content", "") or "",
                raw=msg,
            )
            await self._dispatch(incoming)
        except Exception:
            LOGGER.error("[wecom:%s] inbound message handling error", self.account_id, exc_info=True)

    async def _get_access_token(self) -> str:
        async with self._access_token_lock:
            now = time.time()
            if self._access_token and now < self._access_token_expires_at:
                return self._access_token
            params = {
                "corpid": self.account.corp_id,
                "corpsecret": self.account.secret,
            }
            async with aiohttp.ClientSession() as session:
                async with session.get(
                    f"{WECOM_API_BASE}/gettoken", params=params
                ) as resp:
                    data = await resp.json(content_type=None)
            if data.get("errcode", 0) != 0 or "access_token" not in data:
                raise RuntimeError(f"wecom gettoken failed: {data}")
            self._access_token = data["access_token"]
            # 60s safety margin against clock skew / in-flight calls.
            self._access_token_expires_at = (
                now + int(data.get("expires_in", 7200)) - 60
            )
            return self._access_token

    async def send(self, message: OutgoingMessage) -> None:
        if not message.chat_id:
            LOGGER.error("[wecom:%s] missing chat_id; cannot send", self.account_id)
            return
        try:
            token = await self._get_access_token()
        except Exception:
            LOGGER.error("[wecom:%s] access_token error", self.account_id, exc_info=True)
            return

        payload = {
            "touser": message.chat_id,
            "msgtype": "text",
            "agentid": int(self.account.agent_id),
            "text": {"content": message.text},
            "safe": 0,
        }
        try:
            async with aiohttp.ClientSession() as session:
                async with session.post(
                    f"{WECOM_API_BASE}/message/send",
                    params={"access_token": token},
                    json=payload,
                ) as resp:
                    data = await resp.json(content_type=None)
        except Exception:
            LOGGER.error("[wecom:%s] send transport error", self.account_id, exc_info=True)
            return

        if data.get("errcode", 0) != 0:
            # 40014 / 42001 = access_token expired or invalid; drop cache.
            if data.get("errcode") in (40014, 42001):
                self._access_token = None
                self._access_token_expires_at = 0.0
            LOGGER.error("[wecom:%s] send failed: %s", self.account_id, data)


def _build(account_id: str, cfg: dict) -> Channel:
    required = ("corp_id", "agent_id", "secret", "token", "aes_key")
    missing = [k for k in required if not cfg.get(k)]
    if missing:
        raise ValueError(
            f"wecom account '{account_id}' missing required fields: {missing}"
        )
    try:
        agent_id = int(cfg["agent_id"])
    except (TypeError, ValueError) as err:
        raise ValueError(
            f"wecom account '{account_id}' agent_id must be int: {err}"
        ) from err
    # WeCom EncodingAESKey is always 43 characters; reject placeholders early so
    # the failure is a clear message instead of a base64 "Incorrect padding" error.
    aes_key = str(cfg["aes_key"])
    if len(aes_key) != 43:
        raise ValueError(
            f"wecom account '{account_id}' aes_key (EncodingAESKey) must be 43 characters, got {len(aes_key)}"
        )
    return WeComChannel(
        WeComAccount(
            account_id=account_id,
            corp_id=str(cfg["corp_id"]),
            agent_id=agent_id,
            secret=str(cfg["secret"]),
            token=str(cfg["token"]),
            aes_key=str(cfg["aes_key"]),
            webhook_host=str(cfg.get("webhook_host", "0.0.0.0")),
            webhook_port=int(cfg.get("webhook_port", 3002)),
        )
    )


register_channel("wecom", _build)
