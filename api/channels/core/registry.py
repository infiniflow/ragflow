"""Channel registry for dynamic channel instantiation.

This module provides a registry pattern for channel implementations,
allowing them to be registered by name and instantiated from configuration.
"""

from __future__ import annotations

import logging
from typing import Callable, Dict, List

from .base import Channel

LOGGER = logging.getLogger(__name__)


ChannelBuilder = Callable[[str, dict], Channel]

_BUILDERS: Dict[str, ChannelBuilder] = {}


def register_channel(name: str, builder: ChannelBuilder) -> None:
    """Register a channel builder function by name.

    Args:
        name: The channel type identifier (e.g., "slack", "feishu").
        builder: A function that takes (account_id, config) and returns a Channel.
    """
    _BUILDERS[name] = builder


def registered_channel_ids() -> List[str]:
    """Return a sorted list of registered channel type identifiers.

    Returns:
        List of channel names that have been registered.
    """
    return sorted(_BUILDERS)


def build_channels(config: dict) -> List[Channel]:
    """Walk config.channels.<name>.accounts.<id> and construct one Channel per account."""
    instances: List[Channel] = []
    channels_cfg = config.get("channels") or {}
    for name, raw in channels_cfg.items():
        if not isinstance(raw, dict) or raw.get("enabled") is False:
            continue
        builder = _BUILDERS.get(name)
        if builder is None:
            LOGGER.warning("no builder registered for channel '%s'; skipping", name)
            continue
        accounts = raw.get("accounts") or {}
        if not accounts:
            # Allow a flat single-account config without an `accounts:` block.
            accounts = {"default": {k: v for k, v in raw.items() if k != "accounts"}}
        shared = {k: v for k, v in raw.items() if k not in ("accounts", "default_account")}
        for account_id, account_cfg in accounts.items():
            if not isinstance(account_cfg, dict):
                continue
            if account_cfg.get("enabled") is False:
                continue
            merged = {**shared, **account_cfg}
            instances.append(builder(str(account_id), merged))
    return instances
