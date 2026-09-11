from __future__ import annotations

import asyncio
import logging
from dataclasses import dataclass
from typing import Optional

import discord

from ..core.base import Channel, IncomingMessage, OutgoingMessage
from ..core.registry import register_channel

LOGGER = logging.getLogger(__name__)


@dataclass
class DiscordAccount:
    account_id: str
    token: str


def _chat_type(channel: discord.abc.Messageable) -> str:
    if isinstance(channel, discord.DMChannel):
        return "p2p"
    if isinstance(channel, discord.Thread):
        return "thread"
    if isinstance(channel, (discord.TextChannel, discord.VoiceChannel, discord.StageChannel)):
        return "group"
    return type(channel).__name__


class DiscordChannel(Channel):
    channel_id = "discord"

    def __init__(self, account: DiscordAccount) -> None:
        super().__init__()
        self.account = account
        self.account_id = account.account_id
        intents = discord.Intents.default()
        # Message Content is a privileged intent; must also be enabled in the
        # Developer Portal under the application's Bot page.
        intents.message_content = True
        self._client = discord.Client(intents=intents)
        self._run_task: Optional[asyncio.Task] = None
        self._register_handlers()

    def _register_handlers(self) -> None:
        @self._client.event
        async def on_ready() -> None:
            try:
                user = self._client.user
                LOGGER.info(
                    "[discord:%s] connected as %s (id=%s)",
                    self.account_id,
                    user,
                    user.id if user else "unknown",
                )
            except Exception:
                LOGGER.error("[discord:%s] on_ready error", self.account_id, exc_info=True)

        @self._client.event
        async def on_message(message: discord.Message) -> None:
            try:
                if message.author.bot:
                    return
                me = self._client.user
                if me is not None and message.author.id == me.id:
                    return
                incoming = IncomingMessage(
                    channel=self.channel_id,
                    account_id=self.account_id,
                    chat_id=str(message.channel.id),
                    chat_type=_chat_type(message.channel),
                    message_id=str(message.id),
                    sender_id=str(message.author.id),
                    text=message.content or "",
                    raw=message,
                )
                await self._dispatch(incoming)
            except Exception:
                LOGGER.error("[discord:%s] inbound message handling error", self.account_id, exc_info=True)

    async def start(self) -> None:
        LOGGER.info("[discord:%s] starting gateway client", self.account_id)
        self._run_task = asyncio.create_task(self._client.start(self.account.token))

    async def stop(self) -> None:
        if not self._client.is_closed():
            await self._client.close()
        if self._run_task and not self._run_task.done():
            try:
                await self._run_task
            except (asyncio.CancelledError, Exception):
                pass

    async def send(self, message: OutgoingMessage) -> None:
        try:
            channel_id = int(message.chat_id)
        except (TypeError, ValueError):
            LOGGER.error("[discord:%s] invalid chat_id: %r", self.account_id, message.chat_id)
            return

        target = self._client.get_channel(channel_id)
        if target is None:
            try:
                target = await self._client.fetch_channel(channel_id)
            except discord.HTTPException as err:
                LOGGER.error("[discord:%s] fetch_channel failed: %s", self.account_id, err)
                return

        reference = None
        if message.reply_to_message_id:
            try:
                reference = discord.MessageReference(
                    message_id=int(message.reply_to_message_id),
                    channel_id=channel_id,
                    fail_if_not_exists=False,
                )
            except (TypeError, ValueError):
                reference = None

        try:
            await target.send(message.text, reference=reference)
        except discord.HTTPException as err:
            LOGGER.error("[discord:%s] send failed: %s", self.account_id, err)


def _build(account_id: str, cfg: dict) -> Channel:
    token = cfg.get("token")
    if not token:
        raise ValueError(f"discord account '{account_id}' is missing token")
    return DiscordChannel(DiscordAccount(account_id=account_id, token=str(token)))


register_channel("discord", _build)
