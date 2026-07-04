from __future__ import annotations

import asyncio
import json
import logging
import time
from dataclasses import dataclass
from typing import Any, Optional

import aiohttp

from ..core.base import Channel, IncomingMessage, OutgoingMessage
from ..core.registry import register_channel

LOGGER = logging.getLogger(__name__)

WHATSAPP_DEFAULT_TIMEOUT_SECS = 30
WHATSAPP_DEFAULT_SESSION_KEY = "default"
WHATSAPP_MESSAGE_TTL_SECS = 3600
WHATSAPP_GATEWAY_START_RETRY_SECS = 30
WHATSAPP_GATEWAY_RECONNECT_SECS = 3


@dataclass
class WhatsAppAccount:
    account_id: str
    gateway_base_url: str
    gateway_token: str = ""
    session_key: str = ""
    timeout_secs: int = WHATSAPP_DEFAULT_TIMEOUT_SECS


_live_channels: dict[str, "WhatsAppChannel"] = {}


def _default_gateway_base_url() -> str:
    return "http://127.0.0.1:3005"


def get_runtime_snapshot(account_id: str) -> dict[str, Any] | None:
    channel = _live_channels.get(account_id)
    if channel is None:
        return None
    return channel.get_status_snapshot()


class WhatsAppChannel(Channel):
    channel_id = "whatsapp"

    def __init__(self, account: WhatsAppAccount) -> None:
        super().__init__()
        self.account = account
        self.account_id = account.account_id
        self._task: Optional[asyncio.Task] = None
        self._stop_event = asyncio.Event()
        self._lifecycle_lock = asyncio.Lock()
        self._http_session: Optional[aiohttp.ClientSession] = None
        self._status: str = "stopped"
        self._last_error: str = ""
        self._qr_data_url: str = ""
        self._qr_updated_at: float = 0.0
        self._connected_at: float = 0.0
        self._last_snapshot_at: float = 0.0
        self._session_id: str = ""
        self._event_cursor: int = 0
        self._seen_message_ids: dict[str, float] = {}

    def _session_key(self) -> str:
        value = str(self.account.session_key or "").strip()
        return value or WHATSAPP_DEFAULT_SESSION_KEY

    def _gateway_base_url(self) -> str:
        return str(self.account.gateway_base_url or "").strip().rstrip("/")

    def _gateway_url(self, path: str) -> str:
        base_url = self._gateway_base_url()
        if not base_url:
            raise ValueError("WhatsApp gateway_base_url is required")
        return f"{base_url}/{path.lstrip('/')}"

    def _events_ws_url(self) -> str:
        base_url = self._gateway_base_url()
        if not base_url:
            raise ValueError("WhatsApp gateway_base_url is required")
        if base_url.startswith("http://"):
            ws_base = f"ws://{base_url.removeprefix('http://')}"
        elif base_url.startswith("https://"):
            ws_base = f"wss://{base_url.removeprefix('https://')}"
        else:
            ws_base = base_url
        return f"{ws_base}/whatsapp/{self._session_key()}/events/ws?after={self._event_cursor}"

    def _gateway_headers(self) -> dict[str, str]:
        token = str(self.account.gateway_token or "").strip()
        if not token:
            return {}
        if token.lower().startswith("bearer "):
            return {"Authorization": token}
        return {"Authorization": f"Bearer {token}"}

    async def _ensure_http_session(self) -> aiohttp.ClientSession:
        if self._http_session is not None and not self._http_session.closed:
            return self._http_session
        timeout = aiohttp.ClientTimeout(total=max(int(self.account.timeout_secs), 1))
        self._http_session = aiohttp.ClientSession(timeout=timeout)
        return self._http_session

    async def _request_json(
        self,
        method: str,
        path: str,
        payload: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        session = await self._ensure_http_session()
        url = self._gateway_url(path)
        async with session.request(
            method,
            url,
            json=payload,
            headers=self._gateway_headers(),
        ) as resp:
            text = await resp.text()
            if resp.status >= 400:
                raise RuntimeError(f"status: {resp.status}, response: {text}")
            if not text.strip():
                return {}
            content_type = resp.headers.get("content-type", "")
            if "application/json" not in content_type.lower():
                raise RuntimeError(f"unexpected response content-type: {content_type or 'unknown'}, response: {text[:200]}")
            try:
                return await resp.json()
            except Exception as ex:
                raise RuntimeError(f"invalid json response: {text[:200]}") from ex

    async def _request_json_with_retry(
        self,
        method: str,
        path: str,
        payload: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        deadline = time.time() + WHATSAPP_GATEWAY_START_RETRY_SECS
        last_error: Exception | None = None
        while time.time() < deadline and not self._stop_event.is_set():
            try:
                return await self._request_json(method, path, payload)
            except (aiohttp.ClientError, asyncio.TimeoutError, OSError) as ex:
                last_error = ex
                await asyncio.sleep(0.5)
        if last_error is not None:
            raise last_error
        return await self._request_json(method, path, payload)

    async def start(self) -> None:
        async with self._lifecycle_lock:
            if self._task and not self._task.done():
                return
            self._stop_event.clear()
            _live_channels[self.account_id] = self
            self._task = asyncio.create_task(self._run(), name=f"whatsapp-{self.account_id}")
            LOGGER.info("[whatsapp:%s] starting gateway client", self.account_id)

    async def stop(self) -> None:
        async with self._lifecycle_lock:
            self._stop_event.set()
            task = self._task
            if task and not task.done():
                task.cancel()
                try:
                    await task
                except (asyncio.CancelledError, Exception):
                    pass
            try:
                await self._request_json(
                    "POST",
                    f"whatsapp/{self._session_key()}/stop",
                    {"account_id": self.account_id, "session_key": self._session_key()},
                )
            except Exception:
                LOGGER.debug("[whatsapp:%s] gateway stop failed", self.account_id, exc_info=True)

            try:
                if self._http_session is not None and not self._http_session.closed:
                    await self._http_session.close()
            finally:
                self._http_session = None

            _live_channels.pop(self.account_id, None)
            self._task = None
            self._status = "stopped"
            self._last_error = ""
            self._qr_data_url = ""
            self._qr_updated_at = 0.0
            self._connected_at = 0.0
            self._last_snapshot_at = 0.0
            self._session_id = ""
            self._event_cursor = 0
            self._seen_message_ids.clear()

    async def send(self, message: OutgoingMessage) -> None:
        if not message.chat_id:
            LOGGER.error("[whatsapp:%s] missing chat_id; cannot send", self.account_id)
            return
        try:
            LOGGER.info(
                "[whatsapp:%s] sending reply chat_id=%s reply_to=%s text_preview=%r",
                self.account_id,
                message.chat_id,
                message.reply_to_message_id,
                message.text[:120],
            )
            await self._request_json(
                "POST",
                f"whatsapp/{self._session_key()}/send",
                {
                    "account_id": self.account_id,
                    "session_key": self._session_key(),
                    "chat_id": message.chat_id,
                    "text": message.text,
                    "reply_to_message_id": message.reply_to_message_id,
                },
            )
            LOGGER.info(
                "[whatsapp:%s] message sent chat_id=%s reply_to=%s",
                self.account_id,
                message.chat_id,
                message.reply_to_message_id,
            )
        except Exception:
            LOGGER.error("[whatsapp:%s] send failed", self.account_id, exc_info=True)

    async def _run(self) -> None:
        self._status = "connecting"
        self._last_error = ""
        try:
            await self._request_json_with_retry(
                "POST",
                f"whatsapp/{self._session_key()}/start",
                {
                    "account_id": self.account_id,
                    "session_key": self._session_key(),
                    "gateway_base_url": self._gateway_base_url(),
                },
            )
            while not self._stop_event.is_set():
                try:
                    await self._run_events_ws()
                except asyncio.CancelledError:
                    raise
                except Exception as ex:
                    self._status = "error"
                    self._last_error = str(ex)
                    LOGGER.error("[whatsapp:%s] gateway event loop error", self.account_id, exc_info=True)
                if not self._stop_event.is_set():
                    await asyncio.sleep(WHATSAPP_GATEWAY_RECONNECT_SECS)
        except asyncio.CancelledError:
            pass
        except Exception as ex:
            self._status = "error"
            self._last_error = str(ex)
            LOGGER.error("[whatsapp:%s] gateway runtime error", self.account_id, exc_info=True)
        finally:
            if not self._stop_event.is_set() and self._status != "error":
                self._status = "disconnected"
                if not self._last_error:
                    self._last_error = "WhatsApp gateway stopped unexpectedly."
            self._last_snapshot_at = time.time()

    def _apply_snapshot(self, snapshot: dict[str, Any]) -> None:
        self._last_snapshot_at = time.time()
        self._status = str(snapshot.get("status") or "connecting")
        self._last_error = str(snapshot.get("last_error") or "")
        self._qr_data_url = str(snapshot.get("qr_data_url") or "")
        self._qr_updated_at = float(snapshot.get("qr_updated_at") or 0.0)
        self._connected_at = float(snapshot.get("connected_at") or 0.0)
        self._session_id = str(snapshot.get("session_id") or self._session_id or "")
        self._event_cursor = max(self._event_cursor, int(snapshot.get("event_cursor") or 0))
        if self._status == "connected":
            self._last_error = ""

    async def _run_events_ws(self) -> None:
        session = await self._ensure_http_session()
        url = self._events_ws_url()
        async with session.ws_connect(url, headers=self._gateway_headers(), heartbeat=None) as ws:
            async for msg in ws:
                if msg.type == aiohttp.WSMsgType.TEXT:
                    await self._handle_ws_payload(msg.data)
                elif msg.type == aiohttp.WSMsgType.ERROR:
                    raise RuntimeError(f"whatsapp events websocket error: {ws.exception()}")
            LOGGER.warning("[whatsapp:%s] events websocket closed", self.account_id)

    async def _handle_ws_payload(self, payload: str) -> None:
        try:
            obj = json.loads(payload)
        except json.JSONDecodeError:
            LOGGER.warning(
                "[whatsapp:%s] ignored invalid websocket payload: %r",
                self.account_id,
                payload[:200],
            )
            return

        kind = str(obj.get("type") or "").strip()
        data = obj.get("data")
        if kind == "snapshot" and isinstance(data, dict):
            self._apply_snapshot(data)
            return
        if kind == "event" and isinstance(data, dict):
            await self._handle_event_item(data)
            return

    async def _handle_event_item(self, item: dict[str, Any]) -> None:
        seq = int(item.get("seq") or 0)
        if seq > self._event_cursor:
            self._event_cursor = seq
        if item.get("kind") != "message":
            return
        message_id = str(item.get("message_id") or "").strip()
        if not message_id:
            return
        self._prune_seen_message_ids()
        if self._is_seen_message(message_id):
            return
        self._seen_message_ids[message_id] = time.time()
        incoming = IncomingMessage(
            channel=self.channel_id,
            account_id=self.account_id,
            chat_id=str(item.get("chat_id") or ""),
            chat_type=str(item.get("chat_type") or "p2p"),
            message_id=message_id,
            sender_id=str(item.get("sender_id") or ""),
            text=str(item.get("text") or ""),
            raw=item.get("raw"),
        )
        LOGGER.info(
            "[whatsapp:%s] inbound message_id=%s chat_id=%s",
            self.account_id,
            message_id,
            incoming.chat_id,
        )
        await self._dispatch(incoming)

    def _is_seen_message(self, message_id: str) -> bool:
        return message_id in self._seen_message_ids

    def _prune_seen_message_ids(self) -> None:
        cutoff = time.time() - WHATSAPP_MESSAGE_TTL_SECS
        stale = [key for key, ts in self._seen_message_ids.items() if ts < cutoff]
        for key in stale:
            self._seen_message_ids.pop(key, None)

    def get_status_snapshot(self) -> dict[str, Any]:
        return {
            "account_id": self.account_id,
            "session_key": self._session_key(),
            "status": self._status,
            "connected_at": self._connected_at or None,
            "qr_updated_at": self._qr_updated_at or None,
            "qr_data_url": self._qr_data_url or None,
            "last_error": self._last_error or None,
            "session_id": self._session_id or None,
            "last_snapshot_at": self._last_snapshot_at or None,
            "gateway_base_url": self._gateway_base_url() or None,
            "event_cursor": self._event_cursor,
        }


def _build(account_id: str, cfg: dict) -> Channel:
    gateway_base_url = str(cfg.get("gateway_base_url") or cfg.get("gateway_url") or cfg.get("control_url") or _default_gateway_base_url())
    gateway_token = str(cfg.get("gateway_token") or cfg.get("token") or "")
    session_key = str(cfg.get("session_key") or cfg.get("session_id") or account_id)
    timeout_secs = int(cfg.get("timeout_secs") or WHATSAPP_DEFAULT_TIMEOUT_SECS)
    return WhatsAppChannel(
        WhatsAppAccount(
            account_id=account_id,
            gateway_base_url=gateway_base_url,
            gateway_token=gateway_token,
            session_key=session_key,
            timeout_secs=timeout_secs,
        )
    )


register_channel("whatsapp", _build)
