from __future__ import annotations

import asyncio
import json
import logging
from dataclasses import dataclass
from typing import Any, Dict, Optional
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit

import aiohttp

from ..core.base import Channel, IncomingMessage, OutgoingMessage
from ..core.registry import register_channel

LOGGER = logging.getLogger(__name__)

DINGTALK_API_BASE = "https://api.dingtalk.com"
DINGTALK_WS_FALLBACK = "wss://wss-open-connection.dingtalk.com:443/connect"
DINGTALK_STREAM_TOPIC = "/v1.0/im/bot/messages/get"


@dataclass
class DingTalkAccount:
    account_id: str
    client_id: str
    client_secret: str


class DingTalkChannel(Channel):
    channel_id = "dingtalk"

    def __init__(self, account: DingTalkAccount) -> None:
        super().__init__()
        self.account = account
        self.account_id = account.account_id
        self._stream_task: Optional[asyncio.Task] = None
        self._ws: Optional[aiohttp.ClientWebSocketResponse] = None
        self._stop_requested = False
        self._session_webhooks: Dict[str, str] = {}

    async def start(self) -> None:
        self._stop_requested = False
        if self._stream_task and not self._stream_task.done():
            return
        LOGGER.info(
            "[dingtalk:%s] starting stream client (client_id=%s)",
            self.account_id,
            self._mask(self.account.client_id),
        )
        self._stream_task = asyncio.create_task(
            self._run_stream(),
            name=f"dingtalk-stream-{self.account_id}",
        )

    async def stop(self) -> None:
        self._stop_requested = True
        if self._ws is not None and not self._ws.closed:
            try:
                await self._ws.close()
            except Exception:
                LOGGER.debug("[dingtalk:%s] websocket close error", self.account_id, exc_info=True)
        if self._stream_task and not self._stream_task.done():
            self._stream_task.cancel()
            try:
                await self._stream_task
            except BaseException:
                pass
        self._ws = None
        self._stream_task = None
        self._session_webhooks.clear()

    async def send(self, message: OutgoingMessage) -> None:
        session_webhook = self._session_webhooks.get(message.chat_id)
        if not session_webhook:
            LOGGER.warning(
                "[dingtalk:%s] no sessionWebhook cached for chat_id=%s; dropping reply",
                self.account_id,
                message.chat_id,
            )
            return

        payload = {
            "msgtype": "markdown",
            "markdown": {
                "title": "RAGFlow",
                "text": message.text,
            },
        }
        try:
            async with aiohttp.ClientSession() as session:
                async with session.post(session_webhook, json=payload) as resp:
                    body = await resp.text()
                    if resp.status >= 400:
                        LOGGER.error(
                            "[dingtalk:%s] reply failed: status=%s body=%s",
                            self.account_id,
                            resp.status,
                            body[:500],
                        )
                    else:
                        LOGGER.debug(
                            "[dingtalk:%s] reply sent: status=%s chat_id=%s",
                            self.account_id,
                            resp.status,
                            message.chat_id,
                        )
        except Exception:
            LOGGER.error("[dingtalk:%s] send failed", self.account_id, exc_info=True)

    async def _run_stream(self) -> None:
        backoff = 3
        while not self._stop_requested:
            try:
                endpoint, ticket = await self._open_connection()
                async with aiohttp.ClientSession() as session:
                    ws = await self._connect_websocket(session, endpoint, ticket)
                    self._ws = ws
                    LOGGER.info(
                        "[dingtalk:%s] websocket connected endpoint=%s",
                        self.account_id,
                        endpoint,
                    )
                    await self._subscribe(ws)
                    async for msg in ws:
                        if self._stop_requested:
                            break
                        if msg.type == aiohttp.WSMsgType.TEXT:
                            await self._handle_ws_payload(msg.data)
                        elif msg.type == aiohttp.WSMsgType.BINARY:
                            await self._handle_ws_payload(msg.data.decode("utf-8", "ignore"))
                        elif msg.type in (
                            aiohttp.WSMsgType.CLOSE,
                            aiohttp.WSMsgType.CLOSED,
                            aiohttp.WSMsgType.ERROR,
                        ):
                            break
                backoff = 3
            except asyncio.CancelledError:
                break
            except Exception:
                LOGGER.error("[dingtalk:%s] websocket loop error", self.account_id, exc_info=True)
            finally:
                if self._ws is not None and not self._ws.closed:
                    try:
                        await self._ws.close()
                    except Exception:
                        pass
                self._ws = None

            if not self._stop_requested:
                await asyncio.sleep(backoff)
                backoff = min(backoff * 2, 30)

    async def _connect_websocket(
        self,
        session: aiohttp.ClientSession,
        endpoint: str,
        ticket: str,
    ) -> aiohttp.ClientWebSocketResponse:
        candidates = [
            ("query", self._build_ws_url(endpoint, ticket), {}),
            ("header", endpoint, {"ticket": ticket}),
            ("bare", endpoint, {}),
        ]
        last_error: Exception | None = None
        for mode, url, headers in candidates:
            try:
                LOGGER.info(
                    "[dingtalk:%s] trying websocket connect mode=%s url=%s",
                    self.account_id,
                    mode,
                    url,
                )
                return await session.ws_connect(url, heartbeat=30, headers=headers or None)
            except Exception as exc:
                last_error = exc
                LOGGER.warning(
                    "[dingtalk:%s] websocket connect failed mode=%s: %s",
                    self.account_id,
                    mode,
                    exc,
                )
        if last_error is not None:
            raise last_error
        raise RuntimeError("websocket connect failed")

    async def _open_connection(self) -> tuple[str, str]:
        payload = {
            "clientId": self.account.client_id,
            "clientSecret": self.account.client_secret,
            "subscriptions": [
                {
                    "type": "CALLBACK",
                    "topic": DINGTALK_STREAM_TOPIC,
                }
            ],
        }
        async with aiohttp.ClientSession() as session:
            async with session.post(
                f"{DINGTALK_API_BASE}/v1.0/gateway/connections/open",
                json=payload,
                headers={"Accept": "application/json"},
            ) as resp:
                text = await resp.text()
                if resp.status != 200:
                    raise RuntimeError(f"status: {resp.status}, response: {text}")
                try:
                    data = json.loads(text)
                except json.JSONDecodeError as exc:
                    raise RuntimeError(f"invalid open response: {text[:500]}") from exc

        endpoint = str(data.get("endpoint") or "").strip() or DINGTALK_WS_FALLBACK
        ticket = str(data.get("ticket") or "").strip()
        if not ticket:
            raise RuntimeError(f"connections/open response missing ticket: {text[:500]}")
        LOGGER.info(
            "[dingtalk:%s] connections/open ok endpoint=%s ticket=%s",
            self.account_id,
            endpoint,
            self._mask(ticket),
        )
        return endpoint, ticket

    async def _subscribe(self, ws: aiohttp.ClientWebSocketResponse) -> None:
        # DingTalk Stream uses the `connections/open` ticket to authorize the
        # websocket connection. We keep this hook for future protocol-specific
        # frames, but the current minimal integration does not emit anything.
        _ = ws

    async def _handle_ws_payload(self, payload: str) -> None:
        if not payload:
            return
        try:
            obj = json.loads(payload)
        except json.JSONDecodeError:
            LOGGER.error(
                "[dingtalk:%s] invalid websocket payload: %r",
                self.account_id,
                payload[:200],
            )
            return

        data: Any = obj
        if isinstance(obj, dict):
            if isinstance(obj.get("data"), str):
                try:
                    data = json.loads(obj["data"])
                except json.JSONDecodeError:
                    data = obj["data"]
            elif isinstance(obj.get("data"), dict):
                data = obj["data"]

        if not isinstance(data, dict):
            return

        session_webhook = str(data.get("sessionWebhook") or obj.get("sessionWebhook") or "").strip()
        chat_id = str(
            data.get("conversationId")
            or data.get("chatId")
            or data.get("openConversationId")
            or data.get("msgId")
            or ""
        ).strip()
        sender_id = str(
            data.get("senderId")
            or data.get("senderStaffId")
            or data.get("userId")
            or ""
        ).strip()
        message_id = str(data.get("msgId") or obj.get("msgId") or "").strip()
        chat_type = str(data.get("conversationType") or "").strip()
        text = self._extract_text(data)

        if session_webhook and chat_id:
            self._session_webhooks[chat_id] = session_webhook

        if not text.strip() or not chat_id or not sender_id:
            return

        incoming = IncomingMessage(
            channel=self.channel_id,
            account_id=self.account_id,
            chat_id=chat_id,
            chat_type=chat_type,
            message_id=message_id,
            sender_id=sender_id,
            text=text,
            raw=data,
        )
        await self._dispatch(incoming)

    @staticmethod
    def _extract_text(data: Dict[str, Any]) -> str:
        text = data.get("text")
        if isinstance(text, dict):
            content = text.get("content")
            if isinstance(content, str):
                return content
        if isinstance(text, str):
            return text
        content = data.get("content")
        if isinstance(content, dict):
            for key in ("text", "content", "recognition"):
                value = content.get(key)
                if isinstance(value, str) and value.strip():
                    return value
        if isinstance(content, str):
            return content
        return ""

    @staticmethod
    def _build_ws_url(endpoint: str, ticket: str) -> str:
        parts = urlsplit(endpoint)
        query = dict(parse_qsl(parts.query, keep_blank_values=True))
        query.setdefault("ticket", ticket)
        return urlunsplit(
            (parts.scheme, parts.netloc, parts.path, urlencode(query), parts.fragment)
        )

    @staticmethod
    def _mask(value: str) -> str:
        if len(value) <= 8:
            return "***"
        return f"{value[:4]}...{value[-4:]}"


def _build(account_id: str, cfg: dict) -> Channel:
    credential = cfg.get("credential") or cfg
    client_id = credential.get("client_id") or credential.get("clientId")
    client_secret = credential.get("client_secret") or credential.get("clientSecret")
    if not client_id or not client_secret:
        raise ValueError(f"dingtalk account '{account_id}' is missing client_id or client_secret")
    return DingTalkChannel(
        DingTalkAccount(
            account_id=account_id,
            client_id=str(client_id),
            client_secret=str(client_secret),
        )
    )


register_channel("dingtalk", _build)
