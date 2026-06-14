"""Core channel abstraction for messaging platform integrations.

This module defines the base classes and data structures for implementing
chat channel integrations (Slack, Feishu, Telegram, etc.).
"""

from __future__ import annotations

import logging
from abc import ABC, abstractmethod
from dataclasses import dataclass
from typing import Any, Awaitable, Callable, ClassVar, Optional

LOGGER = logging.getLogger(__name__)


@dataclass
class IncomingMessage:
    """A message received from a messaging platform.

    Attributes:
        channel: The channel type (e.g., "slack", "feishu").
        account_id: The bot account ID on the platform.
        chat_id: The conversation/channel ID.
        chat_type: The type of chat (e.g., "dm", "channel", "group").
        message_id: The platform-specific message ID.
        sender_id: The user ID who sent the message.
        text: The message text content.
        raw: The raw platform-specific message object.
    """
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
    """A message to send to a messaging platform.

    Attributes:
        chat_id: The conversation/channel ID to send to.
        text: The message text content.
        reply_to_message_id: Optional message ID to reply to.
    """
    chat_id: str
    text: str
    reply_to_message_id: Optional[str] = None


MessageHandler = Callable[[IncomingMessage], Awaitable[None]]


class Channel(ABC):
    """One configured bot identity on one messaging platform."""

    channel_id: ClassVar[str]
    account_id: str

    def __init__(self) -> None:
        """Initialize the channel with no message handler set."""
        self._handler: Optional[MessageHandler] = None

    def set_message_handler(self, handler: MessageHandler) -> None:
        """Set the callback for handling incoming messages.

        Args:
            handler: Async function that processes IncomingMessage.
        """
        self._handler = handler

    async def _dispatch(self, message: IncomingMessage) -> None:
        """Dispatch an incoming message to the registered handler.

        Catches and logs exceptions to prevent one bad message from
        crashing the entire channel.

        Args:
            message: The incoming message to dispatch.
        """
        if self._handler is None:
            return
        try:
            await self._handler(message)
        except Exception:  # framework boundary — keep one bad msg from killing the channel
            LOGGER.error("[%s:%s] handler error", self.channel_id, self.account_id, exc_info=True)

    @abstractmethod
    async def start(self) -> None:
        """Start the channel and begin listening for messages."""

    @abstractmethod
    async def stop(self) -> None:
        """Stop the channel and clean up resources."""

    @abstractmethod
    async def send(self, message: OutgoingMessage) -> None:
        """Send a message to the platform.

        Args:
            message: The outgoing message to send.
        """
