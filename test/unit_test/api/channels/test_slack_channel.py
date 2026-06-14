import pytest
from api.channels.slack import SlackChannel

@pytest.mark.asyncio
async def test_slack_channel_initialisation():
    config = {
        "bot_token": "xoxb-test-token",
        "app_token": "xapp-test-token",
        "dialog_id": "test-dialog-id"
    }
    channel = SlackChannel("tenant1", config)
    assert channel._bot_token == "xoxb-test-token"
    assert channel._app_token == "xapp-test-token"
    assert channel._dialog_id == "test-dialog-id"
