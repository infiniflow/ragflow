import asyncio
import unittest
from unittest.mock import patch

from services import execution


class ArtifactPromotionTest(unittest.TestCase):
    def test_promote_root_artifacts_moves_task_output(self):
        calls = []

        async def run_command(*args, **kwargs):
            calls.append(args)
            if args[-7:] == ("find", "/workspace/task-id", "-maxdepth", "1", "-type", "f", "-print0"):
                return 0, "/workspace/task-id/chart.png\0", ""
            return 0, "", ""

        with patch.object(execution, "async_run_command", run_command):
            asyncio.run(execution._promote_root_artifacts("runner", "task-id"))

        self.assertIn(
            (
                "docker",
                "exec",
                "runner",
                "mv",
                "/workspace/task-id/chart.png",
                "/workspace/task-id/artifacts/chart.png",
            ),
            calls,
        )
        self.assertNotIn(("docker", "exec", "runner", "find", "/workspace", "-maxdepth", "1", "-type", "f", "-print0"), calls)

    def test_promote_root_artifacts_skips_control_characters(self):
        calls = []

        async def run_command(*args, **kwargs):
            calls.append(args)
            if args[-7:] == ("find", "/workspace/task-id", "-maxdepth", "1", "-type", "f", "-print0"):
                return 0, "/workspace/task-id/chart\ncopy.png\0", ""
            return 0, "", ""

        with patch.object(execution, "async_run_command", run_command):
            asyncio.run(execution._promote_root_artifacts("runner", "task-id"))

        self.assertFalse(any(call[3] == "mv" for call in calls))
