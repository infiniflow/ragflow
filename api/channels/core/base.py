from __future__ import annotations

import logging
from abc import ABC, abstractmethod
from dataclasses import dataclass
from typing import Any, Awaitable, Callable, ClassVar, Optional

LOGGER = logging.getLogger(__name__)


@dataclass
class IncomingMessage:
    channel: str
    account_id: str
    chat_id: str
    chat_type: str
    message_id: str
    sender_id: str
    text: str
    raw: Any = None


@dataclass
class OutgoingMessage:
    chat_id: str
    text: str
    reply_to_message_id: Optional[str] = None


MessageHandler = Callable[[IncomingMessage], Awaitable[None]]


class Channel(ABC):
    """One configured bot identity on one messaging platform."""

    channel_id: ClassVar[str]
    account_id: str

    def __init__(self) -> None:
        self._handler: Optional[MessageHandler] = None

    def set_message_handler(self, handler: MessageHandler) -> None:
        self._handler = handler

    async def _dispatch(self, message: IncomingMessage) -> None:
        if self._handler is None:
            return
        try:
            await self._handler(message)
        except Exception:  # framework boundary — keep one bad msg from killing the channel
            LOGGER.error("[%s:%s] handler error", self.channel_id, self.account_id, exc_info=True)

    @abstractmethod
    async def start(self) -> None: ...

    @abstractmethod
    async def stop(self) -> None: ...

    @abstractmethod
    async def send(self, message: OutgoingMessage) -> None: ...
