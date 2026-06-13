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

"""Unit tests for the Redis consumer-group pending/reclaim helpers.

Covers two regressions:
  * get_unacked_iterator must not abort the replay of every remaining queue
    when one queue raises a non-"no such key" error (was an UnboundLocalError).
  * requeue_msg must re-add a message exactly once (the retry loop used to fall
    through and xadd up to three times), and reclaim_pending_msg must requeue
    only the entries owned by dead consumers.
"""

from unittest.mock import MagicMock

import pytest

# common.settings must be imported before rag.utils.redis_conn to resolve the
# settings <-> redis_conn circular import the same way the app does.
import common.settings  # noqa: F401
import valkey

from rag.utils.redis_conn import RedisDB


@pytest.fixture
def db():
    """RedisDB (a @singleton) with its connection mocked. The shared instance is
    restored on teardown to keep tests order-independent, and __open__ is stubbed
    so a simulated connection error doesn't reconnect over the mock.
    """
    inst = RedisDB()
    saved = dict(inst.__dict__)
    inst.__open__ = MagicMock()
    inst.REDIS = MagicMock()
    try:
        yield inst
    finally:
        inst.__dict__.clear()
        inst.__dict__.update(saved)


def _msg(msg_id):
    m = MagicMock()
    m.get_msg_id.return_value = msg_id
    return m


@pytest.mark.p1
class TestGetUnackedIterator:
    def test_non_missing_key_error_does_not_skip_remaining_queues(self, db):
        """A transient error on the first queue must not abort the second."""

        def xinfo_groups(queue_name):
            # A non "no such key" error used to abort the whole replay.
            if queue_name == "te.1.common":
                raise valkey.exceptions.ConnectionError("connection reset by peer")
            return [{"name": "grp"}]

        db.REDIS.xinfo_groups.side_effect = xinfo_groups

        # Second queue yields one pending message, then is drained.
        payloads = iter([_msg("5-0"), None])
        db.queue_consumer = MagicMock(side_effect=lambda *a, **k: next(payloads, None))

        out = list(db.get_unacked_iterator(["te.1.common", "te.0.common"], "grp", "c0"))

        assert [m.get_msg_id() for m in out] == ["5-0"]

    def test_missing_stream_is_skipped(self, db):
        db.REDIS.xinfo_groups.side_effect = valkey.exceptions.ResponseError("no such key")
        db.queue_consumer = MagicMock()

        out = list(db.get_unacked_iterator(["te.0.common"], "grp", "c0"))

        assert out == []
        db.queue_consumer.assert_not_called()

    def test_missing_group_is_skipped(self, db):
        db.REDIS.xinfo_groups.return_value = [{"name": "other-group"}]
        db.queue_consumer = MagicMock()

        out = list(db.get_unacked_iterator(["te.0.common"], "grp", "c0"))

        assert out == []
        db.queue_consumer.assert_not_called()


@pytest.mark.p1
class TestRequeueMsg:
    def test_requeues_via_atomic_script_once(self, db):
        """A successful handoff runs the script once and stops retrying."""
        db.lua_requeue_msg = MagicMock(return_value=1)

        assert db.requeue_msg("te.0.common", "grp", "9-0") is True

        db.lua_requeue_msg.assert_called_once()
        kwargs = db.lua_requeue_msg.call_args.kwargs
        assert kwargs["keys"][0] == "te.0.common"
        # Marker is hash-tagged to the queue's slot so both keys co-locate.
        assert kwargs["keys"][1] == "reclaim:{te.0.common}:grp:9-0"
        assert kwargs["args"][:2] == ["grp", "9-0"]

    def test_returns_false_after_exhausting_retries(self, db):
        db.lua_requeue_msg = MagicMock(side_effect=valkey.exceptions.ConnectionError("boom"))

        assert db.requeue_msg("te.0.common", "grp", "9-0") is False
        assert db.lua_requeue_msg.call_count == 3


@pytest.mark.p1
class TestGetPendingMsg:
    def test_returns_empty_on_nogroup(self, db):
        db.REDIS.xpending_range.side_effect = valkey.exceptions.ResponseError(
            "NOGROUP No such key 'te.0.common' or consumer group 'grp'"
        )
        assert db.get_pending_msg("te.0.common", "grp") == []

    def test_paginates_through_full_pel(self, db):
        page1 = [{"message_id": f"{i}-0", "consumer": "c"} for i in range(256)]
        page2 = [{"message_id": "256-0", "consumer": "c"}]
        db.REDIS.xpending_range.side_effect = [page1, page2]

        out = db.get_pending_msg("te.0.common", "grp")

        assert len(out) == 257
        assert db.REDIS.xpending_range.call_count == 2


@pytest.mark.p1
class TestReclaimPendingMsg:
    def test_reclaims_only_dead_consumer_entries(self, db):
        db.REDIS.xpending_range.return_value = [
            {"message_id": "1-0", "consumer": "dead_worker"},
            {"message_id": "2-0", "consumer": "live_worker"},
        ]
        db.lua_requeue_msg = MagicMock(return_value=1)

        reclaimed = db.reclaim_pending_msg(
            ["te.0.common"], "grp", live_consumers={"live_worker"}
        )

        assert reclaimed == 1
        # Only the dead worker's entry is requeued.
        db.lua_requeue_msg.assert_called_once()
        assert db.lua_requeue_msg.call_args.kwargs["args"][:2] == ["grp", "1-0"]

    def test_noop_when_all_consumers_live(self, db):
        db.REDIS.xpending_range.return_value = [
            {"message_id": "1-0", "consumer": "live_a"},
            {"message_id": "2-0", "consumer": "live_b"},
        ]
        db.lua_requeue_msg = MagicMock(return_value=1)

        reclaimed = db.reclaim_pending_msg(
            ["te.0.common"], "grp", live_consumers={"live_a", "live_b"}
        )

        assert reclaimed == 0
        db.lua_requeue_msg.assert_not_called()
