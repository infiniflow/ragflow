import pytest

from common.asyncio_utils import LoopLocalSemaphore


def test_loop_local_semaphore_rejects_zero_capacity():
    with pytest.raises(ValueError, match="must be > 0"):
        LoopLocalSemaphore(0)


def test_loop_local_semaphore_rejects_negative_capacity():
    with pytest.raises(ValueError, match="must be > 0"):
        LoopLocalSemaphore(-3)
