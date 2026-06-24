# Stub common.file_utils for Docker deepdoc service.

import os

_PROJECT_BASE = None


def get_project_base_directory(*args):
    global _PROJECT_BASE
    if _PROJECT_BASE is None:
        # In Docker, the project root is /app
        _PROJECT_BASE = os.environ.get("RAGFLOW_PROJECT_BASE", "/app")
    if args:
        return os.path.join(_PROJECT_BASE, *args)
    return _PROJECT_BASE
