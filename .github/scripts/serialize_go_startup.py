#!/usr/bin/env python3
"""Serialize Go role startup in the CI-only entrypoint copy."""

from pathlib import Path
import sys


ADMIN_BLOCK = """    if [[ \"${API_PROXY_SCHEME}\" == \"hybrid\" ]] || [[ \"${API_PROXY_SCHEME}\" == \"go\" ]]; then
        while true; do
            echo \"Starting Admin go server...\"
            bin/ragflow_server --admin
            echo \"Admin go server started.\"
            sleep 1;
        done &
    fi
"""

API_BLOCK = """    if [[ \"${API_PROXY_SCHEME}\" == \"hybrid\" ]] || [[ \"${API_PROXY_SCHEME}\" == \"go\" ]]; then
        while true; do
            echo \"Starting RAGFlow go server...\"
            bin/ragflow_server --api
            echo \"RAGFlow go server started.\"
            sleep 1;
        done &
    fi
"""


def replace_once(source: str, old: str, new: str) -> str:
    if source.count(old) != 1:
        raise RuntimeError("expected exactly one CI entrypoint startup block")
    return source.replace(old, new)


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit(f"usage: {Path(sys.argv[0]).name} <entrypoint>")

    path = Path(sys.argv[1])
    source = path.read_text()
    helper = """\nwait_for_go_listener() {
    local port=$1
    local role=$2
    for _ in $(seq 1 60); do
        if (echo > \"/dev/tcp/127.0.0.1/${port}\") 2>/dev/null; then
            return 0
        fi
        sleep 1
    done
    echo \"Timed out waiting for Go ${role} server on port ${port}\" >&2
    exit 1
}
"""
    marker = 'ensure_db_init\n\nif [[ "${INIT_MODEL_PROVIDER_TABLES}" -eq 1 ]]; then'
    source = replace_once(source, marker, f'ensure_db_init{helper}\nif [[ "${{INIT_MODEL_PROVIDER_TABLES}}" -eq 1 ]]; then')
    source = replace_once(source, ADMIN_BLOCK, ADMIN_BLOCK.replace("    fi\n", "        wait_for_go_listener 9383 admin\n    fi\n"))
    source = replace_once(source, API_BLOCK, API_BLOCK.replace("    fi\n", "        wait_for_go_listener 9384 api\n    fi\n"))
    path.write_text(source)


if __name__ == "__main__":
    main()
