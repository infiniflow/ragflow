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


def _make_db():
    """A RedisDB whose connection is replaced by a mock (no live server).

    __open__ is stubbed so a simulated connection error in a method under test
    does not reconnect and replace the mock with a real (dead) client.
    """
    db = RedisDB()
    db.__open__ = MagicMock()
    db.REDIS = MagicMock()
    return db


def _msg(msg_id):
    m = MagicMock()
    m.get_msg_id.return_value = msg_id
    return m


@pytest.mark.p1
class TestGetUnackedIterator:
    def test_non_missing_key_error_does_not_skip_remaining_queues(self):
        """A transient error on the first queue must not abort the second."""
        db = _make_db()

        def xinfo_groups(queue_name):
            if queue_name == "te.1.common":
                # Anything other than an exact "no such key" used to leave
                # group_info unbound -> UnboundLocalError -> whole loop aborts.
                raise valkey.exceptions.ConnectionError("connection reset by peer")
            return [{"name": "grp"}]

        db.REDIS.xinfo_groups.side_effect = xinfo_groups

        # Second queue yields one pending message, then is drained.
        payloads = iter([_msg("5-0"), None])
        db.queue_consumer = MagicMock(side_effect=lambda *a, **k: next(payloads, None))

        out = list(db.get_unacked_iterator(["te.1.common", "te.0.common"], "grp", "c0"))

        assert [m.get_msg_id() for m in out] == ["5-0"]

    def test_missing_stream_is_skipped(self):
        db = _make_db()
        db.REDIS.xinfo_groups.side_effect = valkey.exceptions.ResponseError("no such key")
        db.queue_consumer = MagicMock()

        out = list(db.get_unacked_iterator(["te.0.common"], "grp", "c0"))

        assert out == []
        db.queue_consumer.assert_not_called()

    def test_missing_group_is_skipped(self):
        db = _make_db()
        db.REDIS.xinfo_groups.return_value = [{"name": "other-group"}]
        db.queue_consumer = MagicMock()

        out = list(db.get_unacked_iterator(["te.0.common"], "grp", "c0"))

        assert out == []
        db.queue_consumer.assert_not_called()


@pytest.mark.p1
class TestRequeueMsg:
    def test_requeues_exactly_once(self):
        """The retry loop must stop on success instead of re-adding 3x."""
        db = _make_db()
        db.REDIS.xrange.return_value = [("9-0", {"message": "{}"})]

        assert db.requeue_msg("te.0.common", "grp", "9-0") is True
        assert db.REDIS.xadd.call_count == 1
        db.REDIS.xack.assert_called_once_with("te.0.common", "grp", "9-0")

    def test_acks_even_when_entry_trimmed(self):
        """A PEL entry whose stream payload was trimmed is still drained."""
        db = _make_db()
        db.REDIS.xrange.return_value = []

        assert db.requeue_msg("te.0.common", "grp", "9-0") is True
        db.REDIS.xadd.assert_not_called()
        db.REDIS.xack.assert_called_once_with("te.0.common", "grp", "9-0")


@pytest.mark.p1
class TestGetPendingMsg:
    def test_returns_empty_on_nogroup(self):
        db = _make_db()
        db.REDIS.xpending_range.side_effect = valkey.exceptions.ResponseError(
            "NOGROUP No such key 'te.0.common' or consumer group 'grp'"
        )
        assert db.get_pending_msg("te.0.common", "grp") == []

    def test_paginates_through_full_pel(self):
        db = _make_db()
        page1 = [{"message_id": f"{i}-0", "consumer": "c"} for i in range(256)]
        page2 = [{"message_id": "256-0", "consumer": "c"}]
        db.REDIS.xpending_range.side_effect = [page1, page2]

        out = db.get_pending_msg("te.0.common", "grp")

        assert len(out) == 257
        assert db.REDIS.xpending_range.call_count == 2


@pytest.mark.p1
class TestReclaimPendingMsg:
    def test_reclaims_only_dead_consumer_entries(self):
        db = _make_db()
        db.REDIS.xpending_range.return_value = [
            {"message_id": "1-0", "consumer": "dead_worker"},
            {"message_id": "2-0", "consumer": "live_worker"},
        ]
        db.REDIS.xrange.side_effect = lambda q, a, b: [(a, {"message": "{}"})]

        reclaimed = db.reclaim_pending_msg(
            ["te.0.common"], "grp", live_consumers={"live_worker"}
        )

        assert reclaimed == 1
        # Only the dead worker's entry is acked/requeued.
        db.REDIS.xack.assert_called_once_with("te.0.common", "grp", "1-0")

    def test_noop_when_all_consumers_live(self):
        db = _make_db()
        db.REDIS.xpending_range.return_value = [
            {"message_id": "1-0", "consumer": "live_a"},
            {"message_id": "2-0", "consumer": "live_b"},
        ]

        reclaimed = db.reclaim_pending_msg(
            ["te.0.common"], "grp", live_consumers={"live_a", "live_b"}
        )

        assert reclaimed == 0
        db.REDIS.xack.assert_not_called()
