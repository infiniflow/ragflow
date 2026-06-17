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

"""Legacy channel registry for channel type registration.

This module provides a registry pattern for registering and retrieving
channel implementations by type name. Note: New code should use
api/channels/core/registry.py instead.
"""

from typing import Type

from api.channels.base import Channel

CHANNEL_REGISTRY: dict[str, Type[Channel]] = {}


def register_channel(channel_type: str, channel_class: Type[Channel]):
    """Register a channel class under the given type name."""
    CHANNEL_REGISTRY[channel_type] = channel_class


def get_channel(channel_type: str) -> Type[Channel]:
    """Return the channel class for the given type name.

    Raises KeyError if the type is not registered.
    """
    if channel_type not in CHANNEL_REGISTRY:
        raise KeyError(f"Unknown channel type: {channel_type!r}. Registered: {list(CHANNEL_REGISTRY)}")
    return CHANNEL_REGISTRY[channel_type]


def channel(channel_type: str):
    """Class decorator that registers the decorated Channel subclass.

    Args:
        channel_type: The channel type identifier to register under.

    Returns:
        A decorator function that registers the class.
    """
    def decorator(cls: Type[Channel]) -> Type[Channel]:
        register_channel(channel_type, cls)
        return cls
    return decorator
