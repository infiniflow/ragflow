#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

"""Legacy channel abstraction for messaging platform integrations.

This module provides the base classes for channel implementations.
Note: New code should use api/channels/core/base.py instead.
"""

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Any


@dataclass
class IncomingMessage:
    """A message received from an external channel."""
    content: str
    sender_id: str
    channel_id: str
    raw: Any = field(default=None)


@dataclass
class OutgoingMessage:
    """A message to be sent to an external channel."""
    content: str
    recipient_id: str
    channel_id: str
    raw: Any = field(default=None)


class Channel(ABC):
    """Abstract base class for all messaging channel integrations."""

    def __init__(self, tenant_id: str, config: dict):
        """Initialize the channel with tenant ID and configuration.

        Args:
            tenant_id: The tenant ID for this channel instance.
            config: Channel-specific configuration dictionary.
        """
        self.tenant_id = tenant_id
        self.config = config

    @abstractmethod
    async def start(self):
        """Start the channel (open connections, register webhooks, etc.)."""

    @abstractmethod
    async def stop(self):
        """Stop the channel and release all resources."""

    @abstractmethod
    async def send(self, outgoing: OutgoingMessage):
        """Deliver a message to the external channel."""

    @abstractmethod
    async def _dispatch(self, incoming: IncomingMessage) -> "OutgoingMessage | None":
        """Process an incoming message and return an optional reply."""
