from pathlib import Path
import sys


def pytest_sessionstart(session):
    root = Path(__file__).resolve().parents[3]
    sys.path.insert(0, str(root))
    common_mod = sys.modules.get("common")
    if common_mod is not None and not hasattr(common_mod, "__path__"):
        del sys.modules["common"]
