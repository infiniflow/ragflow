from __future__ import annotations

import asyncio
import json
import logging
import time
from dataclasses import dataclass
from typing import Any, Dict, Optional, Tuple

import aiohttp
from aiohttp import web
from wechatpy.enterprise import parse_message
from wechatpy.enterprise.crypto import WeChatCrypto
from wechatpy.exceptions import InvalidSignatureException

from ..core.base import Channel, IncomingMessage, OutgoingMessage
from ..core.registry import register_channel

LOGGER = logging.getLogger(__name__)

WECOM_API_BASE = "https://qyapi.weixin.qq.com/cgi-bin"
WECOM_WS_URL = "wss://openws.work.weixin.qq.com"


@dataclass
class WeComAccount:
    account_id: str
    connection_type: str = "webhook"
    corp_id: str = ""
    agent_id: int = 0
    secret: str = ""
    token: str = ""
    aes_key: str = ""
    bot_id: str = ""
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
        self.connection_type = (account.connection_type or "webhook").strip().lower()
        self.crypto = (
            WeChatCrypto(account.token, account.aes_key, account.corp_id)
            if self.connection_type == "webhook"
            else None
        )
        self._server: Optional[_SharedWebhookServer] = None
        self._access_token: Optional[str] = None
        self._access_token_expires_at: float = 0.0
        self._access_token_lock = asyncio.Lock()
        self._ws_task: Optional[asyncio.Task] = None
        self._ws: Optional[aiohttp.ClientWebSocketResponse] = None
        self._ws_send_lock: Optional[asyncio.Lock] = None
        self._heartbeat_task: Optional[asyncio.Task] = None
        self._stop_requested = False

    async def start(self) -> None:
        self._stop_requested = False
        if self.connection_type == "websocket":
            if self._ws_task and not self._ws_task.done():
                return
            self._ws_send_lock = asyncio.Lock()
            self._ws_task = asyncio.create_task(
                self._run_websocket(),
                name=f"wecom-ws-{self.account_id}",
            )
            LOGGER.info(
                "[wecom:%s] starting websocket client (bot_id=%s)",
                self.account_id,
                self.account.bot_id,
            )
            return

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
        self._stop_requested = True
        if self._heartbeat_task and not self._heartbeat_task.done():
            self._heartbeat_task.cancel()
            try:
                await self._heartbeat_task
            except BaseException:
                pass
            self._heartbeat_task = None
        if self._ws is not None and not self._ws.closed:
            try:
                await self._ws.close()
            except Exception:
                pass
        if self._ws_task and not self._ws_task.done():
            self._ws_task.cancel()
            try:
                await self._ws_task
            except BaseException:
                pass
        self._ws_task = None
        self._ws = None
        if self._server is not None:
            self._server.channels.pop(self.account_id, None)
            await _release_server(
                self.account.webhook_host, self.account.webhook_port
            )
            self._server = None

    async def _handle_text_message(
        self,
        *,
        chat_id: str,
        sender_id: str,
        message_id: str,
        text: str,
        raw: Any,
        chat_type: str = "p2p",
    ) -> None:
        try:
            if not (text or "").strip():
                return
            incoming = IncomingMessage(
                channel=self.channel_id,
                account_id=self.account_id,
                chat_id=chat_id,
                chat_type=chat_type,
                message_id=message_id,
                sender_id=sender_id,
                text=text,
                raw=raw,
            )
            await self._dispatch(incoming)
        except Exception:
            LOGGER.error(
                "[wecom:%s] inbound message handling error",
                self.account_id,
                exc_info=True,
            )

    async def handle_decrypted_message(self, msg) -> None:
        # Short-connection webhook mode.
        try:
            if getattr(msg, "type", "") != "text":
                return
            user_id = str(getattr(msg, "source", "") or "")
            if not user_id:
                return
            await self._handle_text_message(
                chat_id=user_id,
                sender_id=user_id,
                message_id=str(getattr(msg, "id", "") or ""),
                text=getattr(msg, "content", "") or "",
                raw=msg,
                chat_type="p2p",
            )
        except Exception:
            LOGGER.error(
                "[wecom:%s] inbound message handling error",
                self.account_id,
                exc_info=True,
            )

    async def _run_websocket(self) -> None:
        while not self._stop_requested:
            try:
                async with aiohttp.ClientSession() as session:
                    async with session.ws_connect(WECOM_WS_URL, heartbeat=None) as ws:
                        self._ws = ws
                        LOGGER.info(
                            "[wecom:%s] websocket connected",
                            self.account_id,
                        )
                        await self._subscribe_websocket(ws)
                        self._heartbeat_task = asyncio.create_task(
                            self._heartbeat_loop(ws)
                        )
                        async for msg in ws:
                            if self._stop_requested:
                                break
                            if msg.type == aiohttp.WSMsgType.TEXT:
                                await self._handle_ws_payload(msg.data)
                            elif msg.type == aiohttp.WSMsgType.BINARY:
                                await self._handle_ws_payload(
                                    msg.data.decode("utf-8", "ignore")
                                )
                            elif msg.type == aiohttp.WSMsgType.PONG:
                                LOGGER.debug("[wecom:%s] websocket pong", self.account_id)
                            elif msg.type in (
                                aiohttp.WSMsgType.CLOSE,
                                aiohttp.WSMsgType.CLOSED,
                                aiohttp.WSMsgType.ERROR,
                            ):
                                break
            except PermissionError as ex:
                self._stop_requested = True
                LOGGER.error(
                    "[wecom:%s] websocket auth failed; stop reconnecting: %s",
                    self.account_id,
                    ex,
                )
                break
            except asyncio.CancelledError:
                break
            except Exception:
                LOGGER.error(
                    "[wecom:%s] websocket loop error",
                    self.account_id,
                    exc_info=True,
                )
            finally:
                if self._heartbeat_task and not self._heartbeat_task.done():
                    self._heartbeat_task.cancel()
                    try:
                        await self._heartbeat_task
                    except BaseException:
                        pass
                self._heartbeat_task = None
                self._ws = None
            if not self._stop_requested:
                await asyncio.sleep(3)

    async def _heartbeat_loop(self, ws: aiohttp.ClientWebSocketResponse) -> None:
        try:
            while not self._stop_requested and not ws.closed:
                await asyncio.sleep(25)
                await ws.ping()
                LOGGER.debug("[wecom:%s] websocket ping", self.account_id)
        except asyncio.CancelledError:
            pass
        except Exception:
            LOGGER.error("[wecom:%s] websocket heartbeat failed", self.account_id, exc_info=True)

    async def _subscribe_websocket(self, ws: aiohttp.ClientWebSocketResponse) -> None:
        if not self.account.bot_id:
            raise RuntimeError(f"wecom account '{self.account_id}' missing bot_id")
        payload = {
            "cmd": "aibot_subscribe",
            "headers": {"req_id": f"req-{time.time_ns()}"},
            "body": {
                "bot_id": self.account.bot_id,
                "secret": self.account.secret,
            },
        }
        await ws.send_json(payload)
        LOGGER.info("[wecom:%s] websocket subscribe sent", self.account_id)
        try:
            resp = await asyncio.wait_for(ws.receive_json(), timeout=10)
        except Exception as err:
            raise RuntimeError("wecom websocket subscribe ack timeout") from err
        if not isinstance(resp, dict):
            raise RuntimeError(f"wecom websocket subscribe response invalid: {resp!r}")
        if resp.get("cmd") != "aibot_subscribe":
            LOGGER.warning(
                "[wecom:%s] unexpected subscribe response: %s",
                self.account_id,
                resp,
            )
        errcode = int(resp.get("errcode", 0) or 0)
        if errcode != 0:
            if errcode == 853000:
                raise PermissionError(
                    f"wecom websocket subscribe failed: invalid bot_id or secret: {resp}"
                )
            raise RuntimeError(f"wecom websocket subscribe failed: {resp}")
        LOGGER.info("[wecom:%s] websocket subscribed", self.account_id)

    async def _handle_ws_payload(self, payload: str) -> None:
        if not payload:
            return
        try:
            obj = json.loads(payload)
        except json.JSONDecodeError:
            LOGGER.error(
                "[wecom:%s] invalid websocket payload: %r",
                self.account_id,
                payload[:200],
            )
            return
        cmd = str(obj.get("cmd") or "")
        headers = obj.get("headers") or {}
        body = obj.get("body") or {}
        if cmd == "aibot_msg_callback":
            await self._handle_ws_message(headers, body, obj)
            return
        if cmd == "aibot_event_callback":
            await self._handle_ws_event(headers, body, obj)
            return
        if cmd in ("aibot_subscribe", "aibot_respond_msg", "aibot_respond_welcome_msg"):
            errcode = obj.get("errcode")
            if errcode not in (None, 0, "0"):
                LOGGER.error("[wecom:%s] websocket response error: %s", self.account_id, obj)
            else:
                LOGGER.debug("[wecom:%s] websocket response: %s", self.account_id, obj)
            return
        LOGGER.debug("[wecom:%s] websocket ignored cmd=%s", self.account_id, cmd)

    async def _handle_ws_message(self, headers: Any, body: Any, raw: Any) -> None:
        if not isinstance(body, dict):
            return
        msgtype = str(body.get("msgtype") or "")
        if msgtype != "text":
            return
        sender = body.get("from") or {}
        sender_id = str(sender.get("userid") or "")
        if not sender_id:
            return
        chat_type = str(body.get("chattype") or "")
        chat_id = str(body.get("chatid") or sender_id or "")
        req_id = str((headers or {}).get("req_id") or body.get("msgid") or "")
        content = str((body.get("text") or {}).get("content") or "")
        await self._handle_text_message(
            chat_id=chat_id or sender_id,
            sender_id=sender_id,
            message_id=req_id,
            text=content,
            raw=raw,
            chat_type="group" if chat_type == "group" else "p2p",
        )

    async def _handle_ws_event(self, headers: Any, body: Any, raw: Any) -> None:
        if not isinstance(body, dict):
            return
        event = body.get("event") or {}
        event_type = str(event.get("eventtype") or "")
        req_id = str((headers or {}).get("req_id") or body.get("msgid") or "")
        LOGGER.info(
            "[wecom:%s] websocket event=%s req_id=%s",
            self.account_id,
            event_type or "unknown",
            req_id,
        )
        if event_type == "disconnected_event":
            self._stop_requested = True
            if self._ws is not None and not self._ws.closed:
                await self._ws.close()
            return
        # Other events are accepted but do not trigger the RAG handler.

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
        if self.connection_type == "websocket":
            await self._send_websocket_message(message)
            return

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

    async def _send_websocket_message(self, message: OutgoingMessage) -> None:
        if not message.text:
            LOGGER.error("[wecom:%s] empty websocket reply text", self.account_id)
            return
        if self._ws is None or self._ws.closed:
            LOGGER.error("[wecom:%s] websocket is not connected", self.account_id)
            return
        payload = {
            "cmd": "aibot_send_msg",
            "headers": {"req_id": f"req-{time.time_ns()}"},
            "body": {
                "chatid": message.chat_id,
                "msgtype": "markdown",
                "markdown": {"content": message.text},
            },
        }
        try:
            if self._ws_send_lock is None:
                self._ws_send_lock = asyncio.Lock()
            async with self._ws_send_lock:
                await self._ws.send_json(payload)
            LOGGER.info(
                "[wecom:%s] websocket reply sent chat_id=%s",
                self.account_id,
                message.chat_id,
            )
        except Exception:
            LOGGER.error("[wecom:%s] websocket send failed", self.account_id, exc_info=True)


def _build(account_id: str, cfg: dict) -> Channel:
    connection_type = str(cfg.get("connection_type") or "webhook").strip().lower()
    if connection_type == "websocket":
        required = ("bot_id", "secret")
    else:
        required = ("corp_id", "agent_id", "secret", "token", "aes_key")
    missing = [k for k in required if not cfg.get(k)]
    if missing:
        raise ValueError(
            f"wecom account '{account_id}' missing required fields: {missing}"
        )
    agent_id = 0
    aes_key = ""
    corp_id = str(cfg.get("corp_id") or "")
    token = str(cfg.get("token") or "")
    bot_id = str(cfg.get("bot_id") or "")
    if connection_type == "webhook":
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
            connection_type=connection_type,
            corp_id=corp_id,
            agent_id=agent_id,
            secret=str(cfg["secret"]),
            token=token,
            aes_key=aes_key,
            bot_id=bot_id,
            webhook_host=str(cfg.get("webhook_host", "0.0.0.0")),
            webhook_port=int(cfg.get("webhook_port", 3002)),
        )
    )


register_channel("wecom", _build)
