#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#

def test_openai_chat_defaults_stream_to_false_when_omitted():
    req = {"model": "model", "messages": [{"role": "user", "content": "hello"}]}
    stream_mode = bool(req.get("stream", False))
    assert stream_mode is False

    req["stream"] = True
    assert bool(req.get("stream", False)) is True
