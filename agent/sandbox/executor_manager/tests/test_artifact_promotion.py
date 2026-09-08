import asyncio
import unittest
from unittest.mock import patch

from services import execution


class ArtifactPromotionTest(unittest.TestCase):
    def test_promote_root_artifacts_moves_workspace_output(self):
        calls = []

        async def run_command(*args, **kwargs):
            calls.append(args)
            if args[-6:] == ("find", "/workspace", "-maxdepth", "1", "-type", "f"):
                return 0, "/workspace/chart.png\n", ""
            return 0, "", ""

        with patch.object(execution, "async_run_command", run_command):
            asyncio.run(execution._promote_root_artifacts("runner", "task-id"))

        self.assertIn(
            (
                "docker",
                "exec",
                "runner",
                "mv",
                "/workspace/chart.png",
                "/workspace/task-id/artifacts/chart.png",
            ),
            calls,
        )
