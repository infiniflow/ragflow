"""Isolated worker for configured Python import and registration probes.

The parent architecture checker owns policy validation and process timeouts.
This worker intentionally accepts one JSON request on stdin and emits one JSON
result on stdout so imported application code cannot corrupt the protocol.
"""

from __future__ import annotations

import contextlib
import importlib
from importlib.util import module_from_spec, spec_from_file_location
import io
import json
from pathlib import Path
import sys
import threading
from types import ModuleType, SimpleNamespace


DEFAULT_BLOCKED_AUDIT_EVENTS = (
    "os.posix_spawn",
    "os.spawn",
    "os.system",
    "socket.bind",
    "socket.connect",
    "subprocess.Popen",
)


class BlockedRuntimeSideEffect(RuntimeError):
    """Raised when import-time code attempts a forbidden external effect."""


def _inside(path: Path, root: Path) -> bool:
    try:
        path.resolve().relative_to(root)
    except (OSError, ValueError):
        return False
    return True


def _module_file(module: object) -> Path | None:
    value = getattr(module, "__file__", None)
    if not isinstance(value, str):
        return None
    try:
        return Path(value).resolve()
    except OSError:
        return None


def _repo_modules(root: Path, before: set[str], source_roots: set[str]) -> list[dict[str, str]]:
    loaded = []
    for name in sorted(set(sys.modules) - before):
        path = _module_file(sys.modules.get(name))
        if path is not None and _inside(path, root):
            relative = path.relative_to(root)
            if source_roots and relative.parts[0] not in source_roots:
                continue
            if not source_roots and relative.parts[0].startswith("."):
                continue
            loaded.append({"module": name, "path": relative.as_posix()})
    return loaded


def _prefix_matches(module: str, prefixes: list[str]) -> bool:
    return any(module == prefix or module.startswith(prefix + ".") for prefix in prefixes)


def _target_observation(root: Path, module: object, target: dict) -> tuple[dict, list[dict]]:
    module_name = target["module"]
    expected = (root / target["path"]).resolve()
    actual = _module_file(module)
    missing_symbols = sorted(symbol for symbol in target.get("required_symbols", []) if not hasattr(module, symbol))
    findings = []
    if actual != expected:
        findings.append(
            {
                "kind": "unexpected_module_path",
                "module": module_name,
                "expected": expected.relative_to(root).as_posix(),
                "actual": actual.relative_to(root).as_posix() if actual is not None and _inside(actual, root) else str(actual),
                "message": f"{module_name} loaded from an unexpected path",
            }
        )
    for symbol in missing_symbols:
        findings.append(
            {
                "kind": "missing_runtime_symbol",
                "module": module_name,
                "symbol": symbol,
                "message": f"{module_name} does not expose required symbol {symbol}",
            }
        )
    return (
        {
            "module": module_name,
            "path": actual.relative_to(root).as_posix() if actual is not None and _inside(actual, root) else str(actual),
            "required_symbols": target.get("required_symbols", []),
            "missing_symbols": missing_symbols,
        },
        findings,
    )


def _run_isolated_import(root: Path, request: dict) -> tuple[list[dict], list[dict]]:
    observations = []
    findings = []
    for target in request["targets"]:
        module = importlib.import_module(target["module"])
        observation, target_findings = _target_observation(root, module, target)
        observations.append(observation)
        findings.extend(target_findings)
    return observations, findings


def _ensure_package(root: Path, name: str) -> ModuleType:
    existing = sys.modules.get(name)
    if isinstance(existing, ModuleType):
        return existing
    module = ModuleType(name)
    package_path = root.joinpath(*name.split("."))
    module.__path__ = [str(package_path)] if package_path.is_dir() else []
    sys.modules[name] = module
    if "." in name:
        parent_name, child_name = name.rsplit(".", 1)
        setattr(_ensure_package(root, parent_name), child_name, module)
    return module


def _stub_function(name: str):
    def function(*_args, **_kwargs):
        return None

    function.__name__ = name
    return function


def _install_stub_modules(root: Path, specifications: list[dict]) -> None:
    for specification in specifications:
        name = specification["module"]
        if "." in name:
            _ensure_package(root, name.rsplit(".", 1)[0])
        module = ModuleType(name)
        for symbol, kind in specification["symbols"].items():
            if kind == "exception":
                value = type(symbol, (Exception,), {})
            elif kind == "class":
                value = type(symbol, (), {})
            else:
                value = _stub_function(symbol)
            setattr(module, symbol, value)
        sys.modules[name] = module
        if "." in name:
            parent_name, child_name = name.rsplit(".", 1)
            setattr(sys.modules[parent_name], child_name, module)


def _run_ragflow_quart_blueprint(root: Path, request: dict) -> tuple[list[dict], list[dict]]:
    # Quart is infrastructure needed by the loader itself. Import it before the
    # observed section so the probe reports only application-entrypoint effects.
    from quart import Blueprint, Quart

    parent = request["parent_stub"]
    parent_module = ModuleType(parent["module"])
    parent_module.__path__ = [str((root / parent["path"]).resolve())]
    parent_module.current_user = SimpleNamespace(id="architecture-runtime-probe", is_superuser=False)
    parent_module.login_required = lambda function: function
    sys.modules[parent["module"]] = parent_module
    if "." in parent["module"]:
        parent_name, child_name = parent["module"].rsplit(".", 1)
        setattr(_ensure_package(root, parent_name), child_name, parent_module)
    _install_stub_modules(root, request.get("stub_modules", []))

    target = request["target"]
    source_path = (root / target["path"]).resolve()
    spec = spec_from_file_location(target["module"], source_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"Could not create module spec for {target['path']}")
    module = module_from_spec(spec)
    module.manager = Blueprint(request["blueprint_name"], target["module"])
    sys.modules[target["module"]] = module
    spec.loader.exec_module(module)

    stubbed_imports = []
    stub_findings = []
    for stub in request.get("stub_modules", []):
        stub_module = sys.modules[stub["module"]]
        for symbol in sorted(stub["symbols"]):
            used = getattr(module, symbol, None) is getattr(stub_module, symbol)
            stubbed_imports.append({"source": stub["module"], "symbol": symbol, "used": used})
            if not used:
                stub_findings.append(
                    {
                        "kind": "stale_runtime_stub",
                        "module": stub["module"],
                        "symbol": symbol,
                        "message": f"Configured runtime stub is no longer imported: {stub['module']}:{symbol}",
                    }
                )

    app = Quart("architecture-runtime-probe")
    app.register_blueprint(module.manager, url_prefix=request["url_prefix"])
    actual_routes = []
    for rule in app.url_map.iter_rules():
        if rule.endpoint == "static":
            continue
        endpoint = rule.endpoint.rsplit(".", 1)[-1]
        methods = sorted(set(rule.methods or ()) - {"HEAD", "OPTIONS"})
        actual_routes.append({"path": rule.rule, "methods": methods, "endpoint": endpoint})
    actual_routes.sort(key=lambda item: (item["path"], item["methods"], item["endpoint"]))
    expected_routes = sorted(request["expected_routes"], key=lambda item: (item["path"], item["methods"], item["endpoint"]))
    findings = list(stub_findings)
    actual_keys = {(item["path"], tuple(item["methods"]), item["endpoint"]) for item in actual_routes}
    expected_keys = {(item["path"], tuple(item["methods"]), item["endpoint"]) for item in expected_routes}
    for path, methods, endpoint in sorted(expected_keys - actual_keys):
        findings.append(
            {
                "kind": "missing_runtime_route",
                "path": path,
                "methods": list(methods),
                "endpoint": endpoint,
                "message": f"Configured route was not registered: {','.join(methods)} {path} ({endpoint})",
            }
        )
    for path, methods, endpoint in sorted(actual_keys - expected_keys):
        findings.append(
            {
                "kind": "unexpected_runtime_route",
                "path": path,
                "methods": list(methods),
                "endpoint": endpoint,
                "message": f"Unapproved route was registered: {','.join(methods)} {path} ({endpoint})",
            }
        )
    target_observation, target_findings = _target_observation(root, module, target)
    findings.extend(target_findings)
    return [{**target_observation, "stubbed_imports": stubbed_imports, "routes": actual_routes}], findings


def run(request: dict) -> dict:
    root = Path(request["root"]).resolve()
    sys.path.insert(0, str(root))
    python_paths = request.get("python_paths", [])
    if not isinstance(python_paths, list):
        raise ValueError("python_paths must be a list")
    resolved_python_paths = []
    for value in python_paths:
        if not isinstance(value, str) or not value or "\\" in value:
            raise ValueError("python_paths must contain repository-relative POSIX directories")
        path = (root / value).resolve()
        if not _inside(path, root) or not path.is_dir():
            raise ValueError(f"Unsafe or missing python_path: {value}")
        resolved_python_paths.append(path)
    for path in reversed(resolved_python_paths):
        sys.path.insert(0, str(path))
    before_modules = set(sys.modules)
    before_threads = {thread.ident for thread in threading.enumerate() if thread.is_alive() and not thread.daemon}
    audit_events = []
    blocked_events = tuple(request.get("blocked_audit_events") or DEFAULT_BLOCKED_AUDIT_EVENTS)

    def audit(event, _arguments):
        if any(event == blocked or event.startswith(blocked + ".") for blocked in blocked_events):
            audit_events.append(event)
            raise BlockedRuntimeSideEffect(f"Blocked import-time audit event: {event}")

    output = io.StringIO()
    errors = io.StringIO()
    observations = []
    findings = []
    incomplete = []
    interrupted_by_block = False
    try:
        if request["kind"] == "ragflow_quart_blueprint":
            # The framework import happens inside the function before the hook.
            runner = _run_ragflow_quart_blueprint
        else:
            runner = _run_isolated_import
        if request["kind"] == "ragflow_quart_blueprint":
            from quart import Blueprint as _Blueprint  # noqa: F401

        sys.addaudithook(audit)
        with contextlib.redirect_stdout(output), contextlib.redirect_stderr(errors):
            observations, findings = runner(root, request)
    except BlockedRuntimeSideEffect:
        interrupted_by_block = True
    except Exception as exc:  # An incomplete probe must preserve its exact failure class.
        incomplete.append({"kind": "runtime_probe_error", "exception": type(exc).__name__, "message": str(exc)})

    if request.get("audit_events_are_findings", True):
        for event in sorted(set(audit_events)):
            findings.append(
                {
                    "kind": "blocked_import_side_effect",
                    "event": event,
                    "message": f"Import attempted forbidden runtime side effect {event}",
                }
            )
    elif interrupted_by_block:
        incomplete.append(
            {
                "kind": "runtime_probe_blocked",
                "events": sorted(set(audit_events)),
                "message": "A blocked effect interrupted the runtime probe before it completed",
            }
        )
    stdout = output.getvalue()
    stderr = errors.getvalue()
    if not request.get("allow_output", False):
        if stdout:
            findings.append({"kind": "import_stdout", "message": "Import wrote to stdout", "output": stdout[-2000:]})
        if stderr:
            findings.append({"kind": "import_stderr", "message": "Import wrote to stderr", "output": stderr[-2000:]})

    new_threads = sorted(thread.name for thread in threading.enumerate() if thread.is_alive() and not thread.daemon and thread.ident not in before_threads)
    for name in new_threads:
        findings.append({"kind": "import_thread", "thread": name, "message": f"Import left non-daemon thread running: {name}"})

    new_modules = set(sys.modules) - before_modules
    loaded_repo_modules = _repo_modules(root, before_modules, set(request.get("observed_source_roots", [])))
    forbidden = request.get("forbidden_module_prefixes", [])
    forbidden_loaded = sorted(module for module in new_modules if _prefix_matches(module, forbidden))
    for module in forbidden_loaded:
        findings.append(
            {
                "kind": "forbidden_runtime_module",
                "module": module,
                "message": f"Import loaded forbidden module {module}",
            }
        )

    status = "INCOMPLETE" if incomplete else "FAIL" if findings else "PASS"
    return {
        "status": status,
        "observations": observations,
        "loaded_repo_modules": loaded_repo_modules,
        "forbidden_loaded_modules": forbidden_loaded,
        "audit_events": sorted(set(audit_events)),
        "new_non_daemon_threads": new_threads,
        "captured_stdout": stdout[-2000:],
        "captured_stderr": stderr[-2000:],
        "findings": findings,
        "incomplete_reasons": incomplete,
    }


def main() -> int:
    try:
        request = json.loads(sys.stdin.read())
        result = run(request)
    except Exception as exc:
        result = {
            "status": "INCOMPLETE",
            "observations": [],
            "loaded_repo_modules": [],
            "forbidden_loaded_modules": [],
            "audit_events": [],
            "new_non_daemon_threads": [],
            "captured_stdout": "",
            "captured_stderr": "",
            "findings": [],
            "incomplete_reasons": [{"kind": "worker_error", "exception": type(exc).__name__, "message": str(exc)}],
        }
    sys.stdout.write(json.dumps(result, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
