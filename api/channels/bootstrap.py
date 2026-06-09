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

import asyncio
import logging

from api.channels.base import Channel
from api.channels.registry import CHANNEL_REGISTRY

logger = logging.getLogger(__name__)

# Maps channel_id -> running Channel instance
_running: dict[str, Channel] = {}


def _load_enabled_channels() -> list[dict]:
    """Read all enabled rows from the chat_channel table.

    Returns a list of plain dicts with at minimum:
      id, channel_type, tenant_id, config
    """
    try:
        from api.db.db_models import ChatChannel
        rows = ChatChannel.select().where(ChatChannel.status == "enabled")
        return [
            {
                "id": str(row.id),
                "channel_type": row.channel,
                "tenant_id": str(row.tenant_id),
                "config": row.config if isinstance(row.config, dict) else {},
            }
            for row in rows
        ]
    except Exception:
        logger.exception("Failed to load enabled channels from database")
        return []


async def _start_channel(row: dict):
    """Instantiate and start a single channel."""
    channel_id = row["id"]
    channel_type = row["channel_type"]
    cls = CHANNEL_REGISTRY.get(channel_type)
    if cls is None:
        logger.warning("No registered class for channel_type=%r (channel_id=%s)", channel_type, channel_id)
        return
    try:
        instance: Channel = cls(tenant_id=row["tenant_id"], config=row["config"])
        await instance.start()
        _running[channel_id] = instance
        logger.info("Started channel channel_id=%s type=%s", channel_id, channel_type)
    except Exception:
        logger.exception("Failed to start channel channel_id=%s type=%s", channel_id, channel_type)


async def _stop_channel(channel_id: str):
    """Stop and remove a running channel."""
    instance = _running.pop(channel_id, None)
    if instance is None:
        return
    try:
        await instance.stop()
        logger.info("Stopped channel channel_id=%s", channel_id)
    except Exception:
        logger.exception("Failed to stop channel channel_id=%s", channel_id)


async def reconcile_channels():
    """Continuously reconcile running channels against the database every 30 seconds.

    - Starts channels that are enabled but not yet running.
    - Stops channels that are no longer enabled or have been removed.
    """
    # Ensure all channel implementations are imported so they self-register
    _import_channel_modules()

    while True:
        try:
            enabled_rows = _load_enabled_channels()
            enabled_ids = {row["id"] for row in enabled_rows}

            # Stop channels that are no longer enabled
            stale_ids = set(_running) - enabled_ids
            for channel_id in stale_ids:
                await _stop_channel(channel_id)

            # Start channels that are not yet running
            for row in enabled_rows:
                if row["id"] not in _running:
                    await _start_channel(row)

        except Exception:
            logger.exception("reconcile_channels encountered an unexpected error")

        await asyncio.sleep(30)


def _import_channel_modules():
    """Import channel implementation modules so their @channel decorators run."""
    import importlib
    for mod in ("api.channels.feishu", "api.channels.slack"):
        try:
            importlib.import_module(mod)
        except ImportError:
            logger.debug("Optional channel module not available: %s", mod)
        except Exception:
            logger.exception("Error importing channel module: %s", mod)
