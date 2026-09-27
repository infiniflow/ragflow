import asyncio
import json
import sys
from types import ModuleType, SimpleNamespace

import pytest

from api.channels import bootstrap
from api.channels.core.base import IncomingMessage


def test_prepare_agent_turn_resumes_waiting_user_fillup_with_fresh_form_value():
    assert hasattr(bootstrap, "_prepare_agent_turn"), "Agent channel turn preparation is missing"

    dsl = {
        "path": ["UserFillUp:Clarification"],
        "components": {
            "UserFillUp:Clarification": {
                "obj": {
                    "component_name": "UserFillUp",
                    "params": {
                        "inputs": {
                            "clarification": {
                                "name": "补充信息",
                                "optional": False,
                                "options": [],
                                "type": "line",
                            }
                        }
                    },
                }
            }
        },
    }

    query, inputs = bootstrap._prepare_agent_turn("TESLA-M 后面板工位", json.dumps(dsl))

    assert query == ""
    assert inputs == {
        "clarification": {
            "name": "补充信息",
            "optional": False,
            "options": [],
            "type": "line",
            "value": "TESLA-M 后面板工位",
        }
    }


def test_prepare_agent_turn_treats_regular_agent_session_as_a_new_question():
    assert hasattr(bootstrap, "_prepare_agent_turn"), "Agent channel turn preparation is missing"

    query, inputs = bootstrap._prepare_agent_turn(
        "同轴线怎么安装",
        {"path": ["Retrieval:Search"], "components": {}},
    )

    assert query == "同轴线怎么安装"
    assert inputs == {}


def test_channel_agent_user_id_isolates_people_in_the_same_feishu_group():
    assert hasattr(bootstrap, "_channel_agent_user_id"), "Agent channel session identity is missing"

    first = bootstrap._channel_agent_user_id("channel-1", "group-1", "user-a")
    second = bootstrap._channel_agent_user_id("channel-1", "group-1", "user-b")

    assert first == "channel:channel-1:group-1:user-a"
    assert second == "channel:channel-1:group-1:user-b"
    assert first != second


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)


def _run_agent_turn(monkeypatch, frames):
    """Send one inbound message to an Agent-connected channel and return what the bot replied."""
    sent = []

    class _Channel:
        channel_id = "feishu"
        account_id = "channel-1"

        async def send(self, message):
            sent.append(message)

    async def _completion(**_kwargs):
        for frame in frames:
            yield frame

    connection = SimpleNamespace(chat_id=None, agent_id="agent-1", tenant_id="tenant-1")
    _stub(monkeypatch, "api.db.services.api_service", API4ConversationService=SimpleNamespace(get_latest_agent_channel_session=lambda *_a: None))
    _stub(monkeypatch, "api.db.services.canvas_service", completion=_completion)
    _stub(monkeypatch, "api.db.services.chat_channel_service", ChatChannelService=SimpleNamespace(get_by_id=lambda _id: (True, connection)))
    _stub(monkeypatch, "api.db.services.conversation_service", ConversationService=SimpleNamespace(), structure_answer=None)
    _stub(monkeypatch, "api.db.services.dialog_service", DialogService=SimpleNamespace(), async_chat=None)
    _stub(monkeypatch, "common.misc_utils", get_uuid=lambda: "uuid")
    monkeypatch.setattr(bootstrap, "validate_agent_target", lambda *_a: None)

    handle = bootstrap._make_chat_handler(_Channel())
    message = IncomingMessage(
        channel="feishu",
        account_id="channel-1",
        chat_id="group-1",
        chat_type="group",
        message_id="message-1",
        sender_id="user-a",
        text="同轴线怎么安装",
    )
    asyncio.run(handle(message))
    return sent


@pytest.mark.parametrize("separator", ["\u2028", "\u2029", "\x85"])
def test_agent_reply_keeps_frames_with_unicode_line_separators(monkeypatch, separator):
    # canvas_service.completion serializes frames with ensure_ascii=False, so these
    # separators reach the channel raw inside the JSON payload.
    frames = [
        "data:" + json.dumps({"event": "message", "data": {"content": f"先拧紧{separator}再测试"}}, ensure_ascii=False) + "\n\n",
        "data:" + json.dumps({"event": "message_end", "data": {}}, ensure_ascii=False) + "\n\n",
    ]

    sent = _run_agent_turn(monkeypatch, frames)

    assert [message.text for message in sent] == [f"先拧紧{separator}再测试"]
