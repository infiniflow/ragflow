"""Require the independently versioned mandatory browser cases, not just a count.

Update the checked-in inventory only with review of the changed test contract.
Never regenerate the expectation from the runner's current selection in CI.
"""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import xml.etree.ElementTree as ET


INVENTORY = Path(__file__).resolve().parents[2] / "test/playwright/mandatory-browser-cases.json"


def expected_inventory(path: Path = INVENTORY) -> tuple[set[str], str]:
    raw = path.read_bytes()
    data = json.loads(raw)
    nodes = data["nodeids"]
    if type(data["schema"]) is not int or data["schema"] != 1 or data["purpose"] != "mandatory-isolated-browser-regression" or not isinstance(nodes, list) or not nodes:
        raise ValueError("invalid mandatory browser inventory")
    if not all(isinstance(node, str) and node.startswith("test/playwright/e2e/") and ".py::test_" in node for node in nodes):
        raise ValueError("invalid mandatory browser nodeid")
    if len(set(nodes)) != len(nodes) or type(data["expected_count"]) is not int or data["expected_count"] != len(nodes):
        raise ValueError("duplicate browser inventory or inconsistent expected count")
    return set(nodes), hashlib.sha256(raw).hexdigest()


def verify_junit(report: Path, inventory: Path = INVENTORY) -> dict:
    expected, digest = expected_inventory(inventory)
    root = ET.parse(report).getroot()
    if root.tag not in {"testsuite", "testsuites"}:
        raise ValueError("invalid browser JUnit root")
    if any(root.find(".//" + tag) is not None for tag in ("skipped", "failure", "error")):
        raise ValueError("mandatory browser case skipped, failed, or errored")
    cases = list(root.iter("testcase"))
    actual = []
    for case in cases:
        classname, name = case.get("classname"), case.get("name")
        if not classname or not name:
            raise ValueError("browser testcase lacks identity")
        actual.append(classname.replace(".", "/") + ".py::" + name)
    if len(actual) != len(set(actual)):
        raise ValueError("duplicate browser testcase")
    if set(actual) != expected:
        raise ValueError(f"mandatory browser inventory mismatch: missing={len(expected - set(actual))}, extra={len(set(actual) - expected)}")
    for suite in [root, *root.findall(".//testsuite")]:
        if any(int(suite.get(name, "0")) != 0 for name in ("failures", "errors", "skipped")):
            raise ValueError("browser suite reports non-successful cases")
        if "tests" in suite.attrib and int(suite.attrib["tests"]) != len(list(suite.iter("testcase"))):
            raise ValueError("browser suite count is inconsistent")
    return {"tests": len(actual), "inventory_sha256": digest}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("report", type=Path)
    parser.add_argument("--inventory", type=Path, default=INVENTORY)
    args = parser.parse_args()
    try:
        print(json.dumps(verify_junit(args.report, args.inventory)))
        return 0
    except (OSError, ValueError, KeyError, TypeError, ET.ParseError) as exc:
        print(f"INCOMPLETE: {exc}")
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
