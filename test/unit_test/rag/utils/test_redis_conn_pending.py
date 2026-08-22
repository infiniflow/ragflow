#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

from unittest.mock import MagicMock

import pytest
import valkey

from rag.utils.redis_conn import RedisDB


@pytest.fixture
def redis_db():
    db = RedisDB()
    original_state = dict(db.__dict__)
    db.__open__ = MagicMock()
    db.REDIS = MagicMock()
    try:
        yield db
    finally:
        db.__dict__.clear()
        db.__dict__.update(original_state)


def pending_message(message_id):
    message = MagicMock()
    message.get_msg_id.return_value = message_id
    return message


@pytest.mark.p1
class TestPendingReplay:
    def test_queue_failure_does_not_abort_later_queues(self, redis_db):
        def group_info(queue_name):
            if queue_name == "te.1.common":
                raise valkey.exceptions.ConnectionError("connection reset")
            return [{"name": "group"}]

        redis_db.REDIS.xinfo_groups.side_effect = group_info
        redis_db.queue_consumer = MagicMock(side_effect=[pending_message("7-0"), None])

        messages = list(redis_db.get_unacked_iterator(["te.1.common", "te.0.common"], "group", "worker"))

        assert [message.get_msg_id() for message in messages] == ["7-0"]

    def test_missing_stream_is_skipped_case_insensitively(self, redis_db):
        redis_db.REDIS.xinfo_groups.side_effect = valkey.exceptions.ResponseError("NOGROUP No such key 'te.0.common'")
        redis_db.queue_consumer = MagicMock()

        assert list(redis_db.get_unacked_iterator(["te.0.common"], "group", "worker")) == []
        redis_db.queue_consumer.assert_not_called()


@pytest.mark.p1
class TestPendingInspection:
    def test_paginates_over_the_whole_pending_list(self, redis_db):
        first_page = [{"message_id": f"{index}-0", "consumer": "worker-a"} for index in range(128)]
        second_page = [{"message_id": "128-0", "consumer": "worker-b"}]
        redis_db.REDIS.xpending_range.side_effect = [first_page, second_page]

        messages = redis_db.get_pending_msg("te.0.common", "group", count=128)

        assert len(messages) == 129
        assert redis_db.REDIS.xpending_range.call_args_list[1].kwargs["min"] == "(127-0"

    def test_missing_group_has_no_pending_messages(self, redis_db):
        redis_db.REDIS.xpending_range.side_effect = valkey.exceptions.ResponseError("NOGROUP group does not exist")

        assert redis_db.get_pending_msg("te.0.common", "group") == []


@pytest.mark.p1
class TestPendingReclaim:
    def test_atomic_requeue_stops_after_success(self, redis_db):
        redis_db.lua_requeue_msg = MagicMock(return_value=1)

        assert redis_db.requeue_msg("te.0.common", "group", "3-0") is True
        redis_db.lua_requeue_msg.assert_called_once()
        assert redis_db.lua_requeue_msg.call_args.kwargs["keys"][0] == "te.0.common"
        assert redis_db.lua_requeue_msg.call_args.kwargs["keys"][1] == "te.0.common:reclaim:group:3-0"
        assert redis_db.lua_requeue_msg.call_args.kwargs["args"][:2] == ["group", "3-0"]

    def test_reclaims_only_idle_messages_from_dead_consumers(self, redis_db):
        redis_db.REDIS.xpending_range.return_value = [
            {"message_id": "1-0", "consumer": "dead-worker"},
            {"message_id": "2-0", "consumer": "live-worker"},
        ]
        redis_db.lua_requeue_msg = MagicMock(return_value=1)

        reclaimed = redis_db.reclaim_pending_msg(
            ["te.0.common"],
            "group",
            live_consumers={"live-worker"},
            min_idle_ms=120_000,
        )

        assert reclaimed == 1
        assert redis_db.REDIS.xpending_range.call_args.kwargs["idle"] == 120_000
        assert redis_db.lua_requeue_msg.call_args.kwargs["args"][:2] == ["group", "1-0"]
