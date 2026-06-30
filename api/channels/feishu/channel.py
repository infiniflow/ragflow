from __future__ import annotations

import asyncio
import json
import logging
import threading
from dataclasses import dataclass
from typing import Optional

import lark_oapi as lark
import lark_oapi.ws.client as lark_ws_client
from websockets.exceptions import ConnectionClosedOK
from lark_oapi.api.im.v1 import (
    CreateMessageRequest,
    CreateMessageRequestBody,
    P2ImMessageReceiveV1,
    ReplyMessageRequest,
    ReplyMessageRequestBody,
)

from ..core.base import Channel, IncomingMessage, OutgoingMessage
from ..core.registry import register_channel

LOGGER = logging.getLogger(__name__)


@dataclass
class FeishuAccount:
    account_id: str
    app_id: str
    app_secret: str
    domain: str = "feishu"  # "feishu" or "lark"


def _lark_domain(domain: str) -> str:
    return lark.FEISHU_DOMAIN if domain != "lark" else lark.LARK_DOMAIN


class FeishuChannel(Channel):
    channel_id = "feishu"

    def __init__(self, account: FeishuAccount) -> None:
        super().__init__()
        self.account = account
        self.account_id = account.account_id
        self._loop: Optional[asyncio.AbstractEventLoop] = None
        self._ws_client = None
        self._ws_thread: Optional[threading.Thread] = None
        self._rest = (
            lark.Client.builder()
            .app_id(account.app_id)
            .app_secret(account.app_secret)
            .domain(_lark_domain(account.domain))
            .log_level(lark.LogLevel.DEBUG)
            .build()
        )

    async def start(self) -> None:
        # The channel loop is the cross-thread dispatch target for inbound events.
        self._loop = asyncio.get_running_loop()
        LOGGER.info("[feishu:%s] starting WebSocket client", self.account_id)
        self._ws_thread = threading.Thread(
            target=self._run_ws,
            name=f"feishu-ws-{self.account_id}",
            daemon=True,
        )
        self._ws_thread.start()

    def _run_ws(self) -> None:
        # Everything lark touches must be created and run on THIS thread with its
        # own event loop. lark captures the running loop when the handler/client
        # are built and when start() runs; building them on the channel daemon
        # loop made lark schedule its WebSocket onto that loop, colliding with
        # run_channels() ("Leaving task ... does not match" / "cannot enter
        # context: already entered"). A dedicated isolated loop avoids that.
        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)
        loop.set_exception_handler(self._handle_loop_exception)
        # lark_oapi.ws.client stores a module-level `loop` at import time and all
        # websocket task scheduling goes through that object. Rebind it here so
        # this Feishu channel uses the thread-local loop instead of the API
        # server's main loop.
        lark_ws_client.loop = loop
        try:
            handler = (
                lark.EventDispatcherHandler.builder("", "")
                .register_p2_im_message_receive_v1(self._on_message_receive)
                .build()
            )
            self._ws_client = lark.ws.Client(
                self.account.app_id,
                self.account.app_secret,
                domain=_lark_domain(self.account.domain),
                event_handler=handler,
                auto_reconnect=False,
                log_level=lark.LogLevel.DEBUG,
            )
            # Blocks, running lark's own connect/reconnect loop on this thread.
            self._ws_client.start()
        except Exception:
            LOGGER.error("[feishu:%s] WebSocket client crashed", self.account_id, exc_info=True)
        finally:
            try:
                loop.close()
            except Exception:
                pass

    def _handle_loop_exception(self, loop, context) -> None:
        exc = context.get("exception")
        if isinstance(exc, ConnectionClosedOK):
            return
        loop.default_exception_handler(context)

    async def stop(self) -> None:
        # lark's ws client exposes no clean public stop; disconnect best-effort.
        client = self._ws_client
        if client is not None:
            for attr in ("stop", "_disconnect", "disconnect"):
                fn = getattr(client, attr, None)
                if callable(fn):
                    try:
                        result = fn()
                        if asyncio.iscoroutine(result):
                            ws_loop = lark_ws_client.loop
                            if ws_loop and not ws_loop.is_closed():
                                await asyncio.wrap_future(
                                    asyncio.run_coroutine_threadsafe(result, ws_loop)
                                )
                            else:
                                await result
                    except Exception:
                        LOGGER.error("[feishu:%s] ws stop error", self.account_id, exc_info=True)
                    break
        self._ws_client = None
        if self._ws_thread and self._ws_thread.is_alive():
            await asyncio.to_thread(self._ws_thread.join, 5)
        self._ws_thread = None

    async def send(self, message: OutgoingMessage) -> None:
        content = json.dumps({"text": message.text}, ensure_ascii=False)
        if message.reply_to_message_id:
            req = (
                ReplyMessageRequest.builder()
                .message_id(message.reply_to_message_id)
                .request_body(
                    ReplyMessageRequestBody.builder()
                    .content(content)
                    .msg_type("text")
                    .build()
                )
                .build()
            )
            resp = await asyncio.to_thread(self._rest.im.v1.message.reply, req)
        else:
            req = (
                CreateMessageRequest.builder()
                .receive_id_type("chat_id")
                .request_body(
                    CreateMessageRequestBody.builder()
                    .receive_id(message.chat_id)
                    .content(content)
                    .msg_type("text")
                    .build()
                )
                .build()
            )
            resp = await asyncio.to_thread(self._rest.im.v1.message.create, req)
        if not resp.success():
            LOGGER.error(
                "[feishu:%s] send failed: code=%s msg=%s",
                self.account_id,
                resp.code,
                resp.msg,
            )

    def _on_message_receive(self, data: P2ImMessageReceiveV1) -> None:
        # Runs on the lark-oapi WS thread; bounce into asyncio for downstream handlers.
        try:
            incoming = self._normalize(data)
            if self._loop and not self._loop.is_closed():
                future = asyncio.run_coroutine_threadsafe(self._dispatch(incoming), self._loop)
                future.add_done_callback(self._log_dispatch_result)
        except Exception:
            LOGGER.error("[feishu:%s] inbound message handling error", self.account_id, exc_info=True)

    def _log_dispatch_result(self, future) -> None:
        try:
            future.result()
        except Exception:
            LOGGER.error("[feishu:%s] dispatch error", self.account_id, exc_info=True)

    def _normalize(self, data: P2ImMessageReceiveV1) -> IncomingMessage:
        event = data.event
        msg = event.message
        sender = event.sender
        text = ""
        if msg.content:
            try:
                payload = json.loads(msg.content)
                text = payload.get("text", "") if isinstance(payload, dict) else ""
            except (json.JSONDecodeError, TypeError):
                text = msg.content
        sender_id = ""
        if sender and getattr(sender, "sender_id", None):
            sender_id = getattr(sender.sender_id, "open_id", "") or ""
        return IncomingMessage(
            channel=self.channel_id,
            account_id=self.account_id,
            chat_id=msg.chat_id or "",
            chat_type=msg.chat_type or "",
            message_id=msg.message_id or "",
            sender_id=sender_id,
            text=text,
            raw=data,
        )


def _build(account_id: str, cfg: dict) -> Channel:
    app_id = cfg.get("app_id")
    app_secret = cfg.get("app_secret")
    if not app_id or not app_secret:
        raise ValueError(
            f"feishu account '{account_id}' is missing app_id or app_secret"
        )
    return FeishuChannel(
        FeishuAccount(
            account_id=account_id,
            app_id=str(app_id),
            app_secret=str(app_secret),
            domain=str(cfg.get("domain", "feishu")),
        )
    )


register_channel("feishu", _build)
