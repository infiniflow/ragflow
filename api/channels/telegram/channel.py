from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import Optional

from telegram import ReplyParameters, Update
from telegram.ext import Application, ContextTypes, MessageHandler, filters

from ..core.base import Channel, IncomingMessage, OutgoingMessage
from ..core.registry import register_channel

LOGGER = logging.getLogger(__name__)


@dataclass
class TelegramAccount:
    account_id: str
    token: str


def _chat_type(chat) -> str:
    t = getattr(chat, "type", "")
    if t == "private":
        return "p2p"
    if t in ("group", "supergroup"):
        return "group"
    if t == "channel":
        return "channel"
    return str(t) or "unknown"


class TelegramChannel(Channel):
    channel_id = "telegram"

    def __init__(self, account: TelegramAccount) -> None:
        super().__init__()
        self.account = account
        self.account_id = account.account_id
        self._app: Optional[Application] = None

    async def start(self) -> None:
        self._app = Application.builder().token(self.account.token).build()
        self._app.add_handler(MessageHandler(filters.ALL, self._on_update))
        LOGGER.info("[telegram:%s] starting long-poll", self.account_id)
        await self._app.initialize()
        await self._app.start()
        await self._app.updater.start_polling(drop_pending_updates=True)

    async def stop(self) -> None:
        if self._app is None:
            return
        try:
            if self._app.updater and self._app.updater.running:
                await self._app.updater.stop()
            await self._app.stop()
            await self._app.shutdown()
        except Exception:
            LOGGER.error("[telegram:%s] stop error", self.account_id, exc_info=True)
        finally:
            self._app = None

    async def send(self, message: OutgoingMessage) -> None:
        if self._app is None:
            return
        try:
            chat_id = int(message.chat_id)
        except (TypeError, ValueError):
            LOGGER.error("[telegram:%s] invalid chat_id: %r", self.account_id, message.chat_id)
            return

        reply_parameters = None
        if message.reply_to_message_id:
            try:
                reply_parameters = ReplyParameters(
                    message_id=int(message.reply_to_message_id),
                    allow_sending_without_reply=True,
                )
            except (TypeError, ValueError):
                reply_parameters = None
        try:
            await self._app.bot.send_message(
                chat_id=chat_id,
                text=message.text,
                reply_parameters=reply_parameters,
            )
        except Exception:
            LOGGER.error("[telegram:%s] send failed", self.account_id, exc_info=True)

    async def _on_update(self, update: Update, _ctx: ContextTypes.DEFAULT_TYPE) -> None:
        try:
            msg = update.effective_message
            if msg is None or msg.from_user is None or msg.from_user.is_bot:
                return
            text = msg.text or msg.caption or ""
            incoming = IncomingMessage(
                channel=self.channel_id,
                account_id=self.account_id,
                chat_id=str(msg.chat.id),
                chat_type=_chat_type(msg.chat),
                message_id=str(msg.message_id),
                sender_id=str(msg.from_user.id),
                text=text,
                raw=update,
            )
            await self._dispatch(incoming)
        except Exception:
            LOGGER.error("[telegram:%s] inbound message handling error", self.account_id, exc_info=True)


def _build(account_id: str, cfg: dict) -> Channel:
    token = cfg.get("token")
    if not token:
        raise ValueError(f"telegram account '{account_id}' is missing token")
    return TelegramChannel(TelegramAccount(account_id=account_id, token=str(token)))


register_channel("telegram", _build)
