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

from api.apps import current_user, login_required
from api.db import CanvasCategory
from api.db.services.canvas_service import UserCanvasService
from api.db.services.chat_channel_service import ChatChannelService
from api.db.services.dialog_service import DialogService
from api.utils.api_utils import get_data_error_result, get_json_result, get_request_json, validate_request
from common.constants import RetCode
from common.misc_utils import get_uuid

LOGGER = logging.getLogger(__name__)


def _validate_channel_target(req: dict, tenant_id: str):
    """Validate the connected dialog/agent (if any) and return an error message or None."""
    chat_id = req.get("chat_id") or None
    agent_id = req.get("agent_id") or None
    if chat_id and agent_id:
        return "chat_id and agent_id are mutually exclusive"
    if chat_id:
        e, dia = DialogService.get_by_id(chat_id)
        if not e:
            return "Can't find this chat assistant!"
        if dia.tenant_id != tenant_id:
            return "no authorization"
    if agent_id:
        if not UserCanvasService.accessible(agent_id, tenant_id):
            return "no authorization"
        e, canvas = UserCanvasService.get_by_id(agent_id)
        if not e or canvas.canvas_category != CanvasCategory.Agent.value:
            return "Only flow agents can be bound to a chat channel"
    return None


def _chat_channel_auth_error(channel_id: str, user_id: str):
    """Return the chat channel authorization failure response and log the denial."""
    LOGGER.warning("chat channel access denied: channel_id=%s user_id=%s", channel_id, user_id)
    return get_json_result(data=False, message="no authorization", code=RetCode.AUTHENTICATION_ERROR)


@manager.route("/chat-channels", methods=["POST"])  # noqa: F821
@login_required
@validate_request("name", "channel")
async def create_chat_channel():
    """Create a chat channel bot owned by the current tenant."""
    req = await get_request_json()
    error = _validate_channel_target(req, current_user.id)
    if error == "no authorization":
        return _chat_channel_auth_error(req.get("id") or "", current_user.id)
    if error:
        return get_data_error_result(message=error)
    channel = {
        "id": get_uuid(),
        "tenant_id": current_user.id,
        "name": req["name"],
        "channel": req["channel"],
        "config": req.get("config") or {},
        "chat_id": req.get("chat_id") or None,
        "agent_id": req.get("agent_id") or None,
    }
    ChatChannelService.insert(**channel)

    e, conn = ChatChannelService.get_by_id(channel["id"])
    if not e:
        return get_data_error_result(message="Failed to create chat channel!")
    return get_json_result(data=conn.to_dict())


@manager.route("/chat-channels", methods=["GET"])  # noqa: F821
@login_required
def list_chat_channel():
    """List chat channel bots owned by the current tenant."""
    return get_json_result(data=ChatChannelService.list(current_user.id))


@manager.route("/chat-channels/<channel_id>", methods=["GET"])  # noqa: F821
@login_required
def get_chat_channel(channel_id):
    """Return a chat channel bot's details when the current user can access it."""
    if not ChatChannelService.accessible(channel_id, current_user.id):
        return _chat_channel_auth_error(channel_id, current_user.id)

    e, conn = ChatChannelService.get_by_id(channel_id)
    if not e:
        return get_data_error_result(message="Can't find this chat channel!")
    return get_json_result(data=conn.to_dict())


@manager.route("/chat-channels/<channel_id>", methods=["PATCH"])  # noqa: F821
@login_required
async def update_chat_channel(channel_id):
    """Update an accessible chat channel bot's name/config/status."""
    if not ChatChannelService.accessible(channel_id, current_user.id):
        return _chat_channel_auth_error(channel_id, current_user.id)

    e, conn = ChatChannelService.get_by_id(channel_id)
    if not e:
        return get_data_error_result(message="Can't find this chat channel!")

    req = await get_request_json()
    if isinstance(req, dict) and isinstance(req.get("data"), dict):
        req = req["data"]

    # Validate the connected dialog/agent (if provided) belongs to the channel's tenant.
    error = _validate_channel_target(req, conn.tenant_id)
    if error == "no authorization":
        return _chat_channel_auth_error(channel_id, current_user.id)
    if error:
        return get_data_error_result(message=error)

    update_fields = {fld: req[fld] for fld in ["name", "config", "chat_id", "agent_id"] if fld in req}
    # A channel links to exactly one target: setting one clears the other.
    if update_fields.get("chat_id"):
        update_fields["agent_id"] = None
    if update_fields.get("agent_id"):
        update_fields["chat_id"] = None
    if update_fields:
        ChatChannelService.update_by_id(channel_id, update_fields)

    e, conn = ChatChannelService.get_by_id(channel_id)
    if not e:
        return get_data_error_result(message="Can't find this chat channel!")
    return get_json_result(data=conn.to_dict())


@manager.route("/chat-channels/<channel_id>", methods=["DELETE"])  # noqa: F821
@login_required
def rm_chat_channel(channel_id):
    """Delete an accessible chat channel bot."""
    if not ChatChannelService.accessible(channel_id, current_user.id):
        return _chat_channel_auth_error(channel_id, current_user.id)

    ChatChannelService.delete_by_id(channel_id)
    return get_json_result(data=True)


@manager.route("/chat-channels/<channel_id>/runtime", methods=["GET"])  # noqa: F821
@login_required
def get_chat_channel_runtime(channel_id):
    """Return live runtime metadata for a running chat channel."""
    if not ChatChannelService.accessible(channel_id, current_user.id):
        return _chat_channel_auth_error(channel_id, current_user.id)

    e, conn = ChatChannelService.get_by_id(channel_id)
    if not e:
        return get_data_error_result(message="Can't find this chat channel!")

    if conn.channel != "whatsapp":
        return get_data_error_result(message="Runtime snapshot is only available for WhatsApp.")

    try:
        from api.channels.whatsapp.channel import get_runtime_snapshot
    except Exception as ex:
        LOGGER.error("failed to load whatsapp runtime helper: %s", ex, exc_info=True)
        return get_data_error_result(message="WhatsApp runtime is unavailable.")

    snapshot = get_runtime_snapshot(channel_id)
    if snapshot is None:
        return get_json_result(
            data={
                "account_id": channel_id,
                "session_key": channel_id,
                "status": "waiting",
                "connected_at": None,
                "qr_updated_at": None,
                "qr_data_url": None,
                "last_error": None,
                "session_id": None,
                "last_snapshot_at": None,
                "gateway_base_url": None,
                "event_cursor": 0,
            }
        )
    return get_json_result(data=snapshot)
