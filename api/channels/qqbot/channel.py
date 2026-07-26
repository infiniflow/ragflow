from __future__ import annotations

import asyncio
import json
import logging
import random
import time
import threading
from dataclasses import dataclass
from typing import Any, Optional

import aiohttp

from ..core.base import Channel, IncomingMessage, OutgoingMessage
from ..core.registry import register_channel

LOGGER = logging.getLogger(__name__)

DEFAULT_BASE_URL = "https://api.sgroup.qq.com"
DEFAULT_TOKEN_URL = "https://bots.qq.com/app/getAppAccessToken"


@dataclass
class QQBotAccount:
    account_id: str
    app_id: str
    client_secret: str
    base_url: str = DEFAULT_BASE_URL


def _msg_seq() -> int:
    return int((time.time() * 1000) % 65535) or random.randint(1, 65535)


def _normalize_target(chat_id: str) -> tuple[str, str]:
    raw = (chat_id or "").strip()
    if raw.startswith("qqbot:"):
        raw = raw[len("qqbot:") :]
    if raw.startswith("group:"):
        return "group", raw[len("group:") :]
    if raw.startswith("channel:"):
        return "channel", raw[len("channel:") :]
    if raw.startswith("dm:"):
        return "dm", raw[len("dm:") :]
    if raw.startswith("c2c:"):
        return "c2c", raw[len("c2c:") :]
    return "c2c", raw


def _incoming_chat_id(chat_type: str, peer_id: str) -> str:
    if chat_type == "group":
        return f"qqbot:group:{peer_id}"
    if chat_type == "channel":
        return f"qqbot:channel:{peer_id}"
    if chat_type == "dm":
        return f"qqbot:dm:{peer_id}"
    return f"qqbot:c2c:{peer_id}"


class QQBotChannel(Channel):
    channel_id = "qqbot"

    def __init__(self, account: QQBotAccount) -> None:
        super().__init__()
        self.account = account
        self.account_id = account.account_id
        self._task: Optional[asyncio.Task] = None
        self._stop_event = asyncio.Event()
        self._session_id: Optional[str] = None
        self._seq: Optional[int] = None
        self._heartbeat_task: Optional[asyncio.Task] = None
        self._ws_thread: Optional[threading.Thread] = None
        self._ws_loop: Optional[asyncio.AbstractEventLoop] = None
        self._ws_session: Optional[aiohttp.ClientSession] = None

    async def start(self) -> None:
        if self._ws_thread and self._ws_thread.is_alive():
            return
        self._stop_event.clear()
        self._ws_thread = threading.Thread(
            target=self._run_ws_thread,
            name=f"qqbot-ws-{self.account_id}",
            daemon=True,
        )
        self._ws_thread.start()
        LOGGER.info("[qqbot:%s] starting gateway client", self.account_id)

    async def stop(self) -> None:
        self._stop_event.set()
        if self._heartbeat_task and not self._heartbeat_task.done():
            self._heartbeat_task.cancel()
        if self._task and not self._task.done():
            self._task.cancel()
            try:
                await self._task
            except (asyncio.CancelledError, Exception):
                pass
        if self._ws_loop and self._ws_loop.is_running() and self._ws_session is not None:
            self._ws_loop.call_soon_threadsafe(lambda: asyncio.create_task(self._ws_session.close()))
        if self._ws_thread and self._ws_thread.is_alive():
            await asyncio.to_thread(self._ws_thread.join, 5)
        self._heartbeat_task = None
        self._task = None
        self._ws_thread = None

    def _run_ws_thread(self) -> None:
        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)
        self._ws_loop = loop
        try:
            loop.run_until_complete(self._run())
        except Exception:
            LOGGER.error("[qqbot:%s] websocket thread crashed", self.account_id, exc_info=True)
        finally:
            try:
                loop.close()
            except Exception:
                pass
            self._ws_loop = None

    async def send(self, message: OutgoingMessage) -> None:
        if not message.chat_id:
            LOGGER.error("[qqbot:%s] missing chat_id; cannot send", self.account_id)
            return
        chat_type, peer_id = _normalize_target(message.chat_id)
        if not peer_id:
            LOGGER.error("[qqbot:%s] invalid chat_id: %r", self.account_id, message.chat_id)
            return
        if chat_type not in ("c2c", "group", "dm", "channel"):
            LOGGER.error(
                "[qqbot:%s] unsupported outbound chat_type=%s chat_id=%r",
                self.account_id,
                chat_type,
                message.chat_id,
            )
            return
        try:
            token = await self._get_access_token()
            if chat_type == "c2c":
                path = f"/v2/users/{peer_id}/messages"
            elif chat_type == "group":
                path = f"/v2/groups/{peer_id}/messages"
            elif chat_type == "dm":
                path = f"/dms/{peer_id}/messages"
            else:
                path = f"/channels/{peer_id}/messages"
            body = {
                "content": message.text,
                "msg_type": 0,
                "msg_seq": _msg_seq(),
            }
            if message.reply_to_message_id:
                body["msg_id"] = message.reply_to_message_id
            await self._request_json(token, "POST", path, body)
        except Exception:
            LOGGER.error("[qqbot:%s] send failed", self.account_id, exc_info=True)

    async def _run(self) -> None:
        while not self._stop_event.is_set():
            try:
                token = await self._get_access_token()
                gateway_url = await self._get_gateway_url(token)
                async with aiohttp.ClientSession() as session:
                    self._ws_session = session
                    async with session.ws_connect(gateway_url, heartbeat=None) as ws:
                        LOGGER.info("[qqbot:%s] connected to gateway", self.account_id)
                        await self._run_ws_session(ws, token)
            except asyncio.CancelledError:
                break
            except Exception:
                LOGGER.error("[qqbot:%s] gateway loop error", self.account_id, exc_info=True)
            finally:
                self._ws_session = None
            if not self._stop_event.is_set():
                await asyncio.sleep(3)

    async def _run_ws_session(self, ws: aiohttp.ClientWebSocketResponse, token: str) -> None:
        heartbeat_interval = 30
        try:
            async for msg in ws:
                if self._stop_event.is_set():
                    break
                if msg.type == aiohttp.WSMsgType.TEXT:
                    await self._handle_ws_payload(ws, msg.data, token, heartbeat_interval)
                elif msg.type == aiohttp.WSMsgType.BINARY:
                    await self._handle_ws_payload(ws, msg.data.decode("utf-8", "ignore"), token, heartbeat_interval)
                elif msg.type in (aiohttp.WSMsgType.CLOSED, aiohttp.WSMsgType.CLOSE, aiohttp.WSMsgType.ERROR):
                    break
        finally:
            if self._heartbeat_task and not self._heartbeat_task.done():
                self._heartbeat_task.cancel()
            self._heartbeat_task = None

    async def _handle_ws_payload(
        self,
        ws: aiohttp.ClientWebSocketResponse,
        payload: str,
        token: str,
        heartbeat_interval: int,
    ) -> None:
        if not payload:
            return
        try:
            obj = json.loads(payload)
        except json.JSONDecodeError:
            LOGGER.error("[qqbot:%s] invalid gateway payload: %r", self.account_id, payload[:200])
            return

        op = obj.get("op")
        data = obj.get("d")
        seq = obj.get("s")
        event_type = obj.get("t")
        if isinstance(seq, int):
            self._seq = seq

        if op == 10:
            interval = int((data or {}).get("heartbeat_interval", heartbeat_interval))
            heartbeat_interval = interval
            await self._send_identify_or_resume(ws, token)
            if self._heartbeat_task and not self._heartbeat_task.done():
                self._heartbeat_task.cancel()
            self._heartbeat_task = asyncio.create_task(self._heartbeat_loop(ws, interval))
            return

        if op == 11:
            return

        if op == 7:
            LOGGER.info("[qqbot:%s] gateway requested reconnect", self.account_id)
            await ws.close()
            return

        if op == 9:
            can_resume = bool(data)
            LOGGER.info("[qqbot:%s] invalid session (can_resume=%s)", self.account_id, can_resume)
            if not can_resume:
                self._session_id = None
                self._seq = None
                await asyncio.sleep(3)
            await ws.close()
            return

        if op != 0 or not event_type:
            return

        if event_type == "READY":
            self._session_id = (data or {}).get("session_id")
            LOGGER.info("[qqbot:%s] ready session_id=%s", self.account_id, self._session_id)
            return
        if event_type == "RESUMED":
            LOGGER.info("[qqbot:%s] resumed session", self.account_id)
            return

        incoming = self._normalize_incoming_event(event_type, data)
        if incoming is None:
            return
        asyncio.create_task(self._dispatch(incoming))

    async def _heartbeat_loop(self, ws: aiohttp.ClientWebSocketResponse, interval_ms: int) -> None:
        delay = max(interval_ms / 1000.0, 1.0)
        try:
            while not self._stop_event.is_set() and not ws.closed:
                await asyncio.sleep(delay)
                await ws.send_json({"op": 1, "d": self._seq})
        except asyncio.CancelledError:
            pass
        except Exception:
            LOGGER.error("[qqbot:%s] heartbeat failed", self.account_id, exc_info=True)

    async def _send_identify_or_resume(self, ws: aiohttp.ClientWebSocketResponse, token: str) -> None:
        if self._session_id and self._seq is not None:
            await ws.send_json(
                {
                    "op": 6,
                    "d": {
                        "token": f"QQBot {token}",
                        "session_id": self._session_id,
                        "seq": self._seq,
                    },
                }
            )
            return
        await ws.send_json(
            {
                "op": 2,
                "d": {
                    "token": f"QQBot {token}",
                    "intents": (1 << 30) | (1 << 12) | (1 << 25) | (1 << 26),
                    "shard": [0, 1],
                },
            }
        )

    def _normalize_incoming_event(self, event_type: str, data: Any) -> Optional[IncomingMessage]:
        if not isinstance(data, dict):
            return None

        if event_type == "C2C_MESSAGE_CREATE":
            author = data.get("author") or {}
            sender_id = str(author.get("user_openid") or "")
            if not sender_id:
                return None
            return IncomingMessage(
                channel=self.channel_id,
                account_id=self.account_id,
                chat_id=_incoming_chat_id("c2c", sender_id),
                chat_type="p2p",
                message_id=str(data.get("id") or ""),
                sender_id=sender_id,
                text=str(data.get("content") or ""),
                raw=data,
            )

        if event_type in ("GROUP_AT_MESSAGE_CREATE", "GROUP_MESSAGE_CREATE"):
            author = data.get("author") or {}
            group_openid = str(data.get("group_openid") or "")
            sender_id = str(author.get("member_openid") or "")
            if not group_openid or not sender_id:
                return None
            return IncomingMessage(
                channel=self.channel_id,
                account_id=self.account_id,
                chat_id=_incoming_chat_id("group", group_openid),
                chat_type="group",
                message_id=str(data.get("id") or ""),
                sender_id=sender_id,
                text=str(data.get("content") or ""),
                raw=data,
            )

        if event_type == "DIRECT_MESSAGE_CREATE":
            author = data.get("author") or {}
            sender_id = str(author.get("id") or "")
            guild_id = str(data.get("guild_id") or "")
            if not sender_id:
                return None
            chat_id = guild_id or sender_id
            return IncomingMessage(
                channel=self.channel_id,
                account_id=self.account_id,
                chat_id=_incoming_chat_id("dm", chat_id),
                chat_type="dm",
                message_id=str(data.get("id") or ""),
                sender_id=sender_id,
                text=str(data.get("content") or ""),
                raw=data,
            )

        if event_type == "AT_MESSAGE_CREATE":
            author = data.get("author") or {}
            sender_id = str(author.get("id") or "")
            channel_id = str(data.get("channel_id") or "")
            if not sender_id or not channel_id:
                return None
            return IncomingMessage(
                channel=self.channel_id,
                account_id=self.account_id,
                chat_id=_incoming_chat_id("channel", channel_id),
                chat_type="channel",
                message_id=str(data.get("id") or ""),
                sender_id=sender_id,
                text=str(data.get("content") or ""),
                raw=data,
            )

        return None

    async def _get_access_token(self) -> str:
        body = {"appId": self.account.app_id, "clientSecret": self.account.client_secret}
        data = await self._request_json(
            None,
            "POST",
            DEFAULT_TOKEN_URL,
            body,
            auth=False,
            absolute_url=True,
        )
        token = data.get("access_token") or data.get("accessToken")
        if not token:
            raise RuntimeError(f"qqbot token response missing access_token: {data}")
        return str(token)

    async def _get_gateway_url(self, access_token: str) -> str:
        data = await self._request_json(access_token, "GET", "/gateway")
        url = data.get("url")
        if not url:
            raise RuntimeError(f"qqbot gateway response missing url: {data}")
        return str(url)

    async def _request_json(
        self,
        access_token: Optional[str],
        method: str,
        path: str,
        body: Optional[dict[str, Any]] = None,
        auth: bool = True,
        absolute_url: bool = False,
    ) -> dict[str, Any]:
        headers = {"Content-Type": "application/json"}
        if auth and access_token:
            headers["Authorization"] = f"QQBot {access_token}"
        url = path if absolute_url else f"{self.account.base_url.rstrip('/')}{path}"
        timeout = aiohttp.ClientTimeout(total=60)
        async with aiohttp.ClientSession(timeout=timeout) as session:
            async with session.request(method, url, headers=headers, json=body) as resp:
                text = await resp.text()
                if resp.status >= 400:
                    raise RuntimeError(f"status: {resp.status}, response: {text}")
                try:
                    return json.loads(text) if text else {}
                except json.JSONDecodeError as err:
                    raise RuntimeError(f"invalid json response from {path}: {text[:200]}") from err


def _build(account_id: str, cfg: dict) -> Channel:
    app_id = cfg.get("app_id")
    client_secret = cfg.get("client_secret")
    if not app_id or not client_secret:
        raise ValueError(f"qqbot account '{account_id}' is missing app_id or client_secret")
    return QQBotChannel(
        QQBotAccount(
            account_id=account_id,
            app_id=str(app_id),
            client_secret=str(client_secret),
            base_url=str(cfg.get("base_url") or DEFAULT_BASE_URL),
        )
    )


register_channel("qqbot", _build)
