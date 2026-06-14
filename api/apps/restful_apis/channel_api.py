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

import logging

from quart import request

from api.apps import login_required, current_user
from api.channels.bootstrap import _running, _stop_channel
from api.channels.registry import CHANNEL_REGISTRY
from api.db.db_models import ChatChannel
from api.utils.api_utils import (
    get_error_data_result,
    get_json_result,
    get_request_json,
)
from common.constants import RetCode
from common.misc_utils import get_uuid
from common.time_utils import current_timestamp

logger = logging.getLogger(__name__)

# --------------------------------------------------------------------------- #
# Helpers
# --------------------------------------------------------------------------- #

_CHANNEL_INFO: dict[str, list[dict]] = {
    "slack": [
        {"name": "bot_token", "label": "Bot Token", "type": "password", "required": True},
        {"name": "app_token", "label": "App Token (Socket Mode)", "type": "password", "required": True},
    ],
    "feishu": [
        {"name": "app_id", "label": "App ID", "type": "text", "required": True},
        {"name": "app_secret", "label": "App Secret", "type": "password", "required": True},
        {"name": "encrypt_key", "label": "Encrypt Key", "type": "password", "required": False},
    ],
}


def _row_to_dict(row: ChatChannel) -> dict:
    d = row.to_dict()
    d.pop("config", None)
    return d


def _owns(row: ChatChannel) -> bool:
    return row.tenant_id == current_user.id


# --------------------------------------------------------------------------- #
# Routes
# --------------------------------------------------------------------------- #

@manager.route("/channels", methods=["GET"])  # noqa: F821
@login_required
async def list_channels():
    rows = ChatChannel.select().where(
        ChatChannel.tenant_id == current_user.id
    ).order_by(ChatChannel.create_time.desc())
    return get_json_result(data=[_row_to_dict(r) for r in rows])


@manager.route("/channels/info", methods=["GET"])  # noqa: F821
@login_required
async def channel_info():
    channel_type = request.args.get("type", "")
    if not channel_type:
        return get_error_data_result(message="`type` query parameter is required")
    fields = _CHANNEL_INFO.get(channel_type)
    if fields is None:
        registered = list(CHANNEL_REGISTRY.keys())
        return get_error_data_result(
            message=f"Unknown channel type {channel_type!r}. Registered: {registered}"
        )
    return get_json_result(data=fields)


@manager.route("/channels", methods=["POST"])  # noqa: F821
@login_required
async def create_channel():
    req = await get_request_json()
    if not req:
        return get_error_data_result(message="Request body is required")

    name = (req.get("name") or "").strip()
    channel_type = (req.get("channel") or "").strip()
    dialog_id = (req.get("dialog_id") or "").strip()
    config = req.get("config") or {}

    if not name:
        return get_error_data_result(message="`name` is required")
    if not channel_type:
        return get_error_data_result(message="`channel` is required")
    if not dialog_id:
        return get_error_data_result(message="`dialog_id` is required")
    if channel_type not in CHANNEL_REGISTRY and channel_type not in _CHANNEL_INFO:
        return get_error_data_result(message=f"Unknown channel type: {channel_type!r}")

    now = current_timestamp()
    row = ChatChannel.create(
        id=get_uuid(),
        tenant_id=current_user.id,
        name=name,
        channel=channel_type,
        config=config,
        dialog_id=dialog_id,
        status=req.get("status", "enabled"),
        create_time=now,
        update_time=now,
    )
    return get_json_result(data=_row_to_dict(row))


@manager.route("/channels/<channel_id>", methods=["PUT"])  # noqa: F821
@login_required
async def update_channel(channel_id):
    try:
        row = ChatChannel.get_by_id(channel_id)
    except ChatChannel.DoesNotExist:
        return get_error_data_result(message="Channel not found")
    if not _owns(row):
        return get_json_result(
            data=False, message="No authorization.", code=RetCode.AUTHENTICATION_ERROR
        )

    req = await get_request_json()
    if not req:
        return get_error_data_result(message="Request body is required")

    updates: dict = {"update_time": current_timestamp()}
    for field in ("name", "channel", "dialog_id", "config", "status"):
        if field in req:
            updates[field] = req[field]

    ChatChannel.update(updates).where(ChatChannel.id == channel_id).execute()
    row = ChatChannel.get_by_id(channel_id)
    return get_json_result(data=_row_to_dict(row))


@manager.route("/channels/<channel_id>", methods=["DELETE"])  # noqa: F821
@login_required
async def delete_channel(channel_id):
    try:
        row = ChatChannel.get_by_id(channel_id)
    except ChatChannel.DoesNotExist:
        return get_error_data_result(message="Channel not found")
    if not _owns(row):
        return get_json_result(
            data=False, message="No authorization.", code=RetCode.AUTHENTICATION_ERROR
        )

    # Stop the running instance if any
    if channel_id in _running:
        try:
            import asyncio
            asyncio.create_task(_stop_channel(channel_id))
        except Exception:
            logger.exception("Error stopping channel %s before deletion", channel_id)

    ChatChannel.delete_by_id(channel_id)
    return get_json_result(data=True)


@manager.route("/channels/<channel_id>/webhook", methods=["POST"])  # noqa: F821
async def feishu_webhook(channel_id):
    """Webhook endpoint for Feishu event callbacks.

    Feishu pushes events here; the channel instance handles verification and dispatch.
    No authentication header — Feishu signs the payload instead.
    """
    from api.channels.feishu import FeishuChannel

    instance = _running.get(channel_id)
    if not isinstance(instance, FeishuChannel):
        return get_json_result(
            data=False, message="Channel not found or not running", code=RetCode.DATA_ERROR
        )

    payload = await get_request_json() or {}
    result = await instance.handle_webhook(payload)
    return get_json_result(data=result)
