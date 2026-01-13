import json
import sys
import time
from pathlib import Path


def eprint(*args, **kwargs):
    print(*args, file=sys.stderr, **kwargs)


def load_json_arg(value, name):
    if value is None:
        return None
    if isinstance(value, dict):
        return value
    if isinstance(value, str) and value.startswith("@"):
        path = Path(value[1:])
        try:
            return json.loads(path.read_text(encoding="utf-8"))
        except Exception as exc:
            raise ValueError(f"Failed to read {name} from {path}: {exc}") from exc
    try:
        return json.loads(value)
    except Exception as exc:
        raise ValueError(f"Invalid JSON for {name}: {exc}") from exc


def split_csv(value):
    if value is None:
        return None
    if isinstance(value, list):
        return value
    if isinstance(value, str):
        items = [item.strip() for item in value.split(",")]
        return [item for item in items if item]
    return [value]


def unique_name(prefix):
    return f"{prefix}_{int(time.time() * 1000)}"

