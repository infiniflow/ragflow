import json

from api.channels import bootstrap


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
