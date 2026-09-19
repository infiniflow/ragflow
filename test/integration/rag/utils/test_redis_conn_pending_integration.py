#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

import os
import time
import uuid

import pytest
import valkey

from rag.utils.redis_conn import RedisDB


@pytest.fixture
def redis_client():
    url = os.environ.get("RAGFLOW_REDIS_TEST_URL")
    if not url:
        pytest.skip("set RAGFLOW_REDIS_TEST_URL to run against Redis or Valkey")

    client = valkey.from_url(url, decode_responses=True)
    try:
        client.ping()
    except valkey.exceptions.RedisError as error:
        pytest.fail(f"RAGFLOW_REDIS_TEST_URL is unavailable: {error}")
    try:
        yield client
    finally:
        client.close()


@pytest.fixture
def redis_db(redis_client):
    db = RedisDB()
    original_state = dict(db.__dict__)
    db.REDIS = redis_client
    try:
        yield db
    finally:
        db.__dict__.clear()
        db.__dict__.update(original_state)


def read_one(client, stream, group, consumer):
    messages = client.xreadgroup(group, consumer, {stream: ">"}, count=1)
    return messages[0][1][0][0]


@pytest.mark.p1
def test_xclaim_transfers_only_dead_consumer_entries(redis_db, redis_client):
    stream = f"test:pending:{uuid.uuid4()}"
    group = "task-executor"
    try:
        live_id = redis_client.xadd(stream, {"message": '{"id": "live"}'})
        dead_id = redis_client.xadd(stream, {"message": '{"id": "dead"}'})
        acked_id = redis_client.xadd(stream, {"message": '{"id": "acked"}'})
        deleted_id = redis_client.xadd(stream, {"message": '{"id": "deleted"}'})
        redis_client.xgroup_create(stream, group, id="0-0")

        assert read_one(redis_client, stream, group, "live-worker") == live_id
        assert read_one(redis_client, stream, group, "dead-worker") == dead_id
        assert read_one(redis_client, stream, group, "acked-worker") == acked_id
        assert read_one(redis_client, stream, group, "deleted-worker") == deleted_id
        assert redis_client.xack(stream, group, acked_id) == 1
        assert redis_client.xdel(stream, deleted_id) == 1
        time.sleep(0.01)

        claimed = redis_db.reclaim_pending_msg(
            [stream],
            group,
            "recovery-worker",
            live_consumers={"live-worker"},
            min_idle_ms=0,
        )

        assert [message.get_msg_id() for message in claimed] == [dead_id]
        assert claimed[0].get_message() == {"id": "dead"}
        pending = {entry["message_id"]: entry["consumer"] for entry in redis_client.xpending_range(stream, group, "-", "+", 10)}
        assert pending[live_id] == "live-worker"
        assert pending[dead_id] == "recovery-worker"
        assert acked_id not in pending
        if deleted_id in pending:
            assert pending[deleted_id] == "recovery-worker"
        assert redis_client.xlen(stream) == 3

        assert claimed[0].ack() is True
        pending = {entry["message_id"]: entry["consumer"] for entry in redis_client.xpending_range(stream, group, "-", "+", 10)}
        assert pending[live_id] == "live-worker"
        assert dead_id not in pending
    finally:
        redis_client.delete(stream)


@pytest.mark.p1
def test_xclaim_ignores_an_entry_acked_after_pending_inspection(redis_db, redis_client):
    stream = f"test:pending:{uuid.uuid4()}"
    group = "task-executor"
    try:
        message_id = redis_client.xadd(stream, {"message": '{"id": "acked"}'})
        redis_client.xgroup_create(stream, group, id="0-0")
        assert read_one(redis_client, stream, group, "worker-a") == message_id

        stale_pending = redis_db.get_pending_msg(stream, group)
        assert [entry["message_id"] for entry in stale_pending] == [message_id]
        assert redis_client.xack(stream, group, message_id) == 1

        claimed = redis_db.claim_pending_msg(stream, group, "worker-b", [message_id], min_idle_ms=0)

        assert claimed == []
        assert redis_client.xlen(stream) == 1
        assert redis_client.xpending_range(stream, group, "-", "+", 10) == []
    finally:
        redis_client.delete(stream)
