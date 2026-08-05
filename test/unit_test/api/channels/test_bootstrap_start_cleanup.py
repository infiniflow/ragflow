import pytest

from api.channels import bootstrap


class _PartiallyStartedChannel:
    channel_id = "fake"

    def __init__(self, stop_error=False):
        self.stop_calls = 0
        self.stop_error = stop_error
        self.handler = None

    def set_message_handler(self, handler):
        self.handler = handler

    async def start(self):
        raise RuntimeError("start failed after allocating resources")

    async def stop(self):
        self.stop_calls += 1
        if self.stop_error:
            raise RuntimeError("cleanup failed")


@pytest.mark.asyncio
async def test_start_channel_cleans_up_partial_start(monkeypatch):
    channel = _PartiallyStartedChannel()
    running = {}

    monkeypatch.setattr(bootstrap, "_build_one", lambda *_args: channel)
    monkeypatch.setattr(bootstrap, "_make_chat_handler", lambda _channel: object())

    started = await bootstrap._start_channel(
        running,
        account_id="account-1",
        channel="fake",
        credential={},
        fp="fingerprint",
    )

    assert started is False
    assert channel.stop_calls == 1
    assert running == {}


@pytest.mark.asyncio
async def test_start_channel_contains_cleanup_failure(monkeypatch):
    channel = _PartiallyStartedChannel(stop_error=True)
    running = {}

    monkeypatch.setattr(bootstrap, "_build_one", lambda *_args: channel)
    monkeypatch.setattr(bootstrap, "_make_chat_handler", lambda _channel: object())

    started = await bootstrap._start_channel(
        running,
        account_id="account-1",
        channel="fake",
        credential={},
        fp="fingerprint",
    )

    assert started is False
    assert channel.stop_calls == 1
    assert running == {}
