"""Observe Go imports, build profiles and package reachability without compiling code."""

from __future__ import annotations

import argparse
from collections import deque
import hashlib
import json
from pathlib import Path, PurePosixPath
import platform
import re
import subprocess
import sys

import yaml

from capture_inventory import capture, git, paths, safe_path


VERSION = "0.1.0"
GOOS = {
    "aix",
    "android",
    "darwin",
    "dragonfly",
    "freebsd",
    "illumos",
    "ios",
    "js",
    "linux",
    "netbsd",
    "openbsd",
    "plan9",
    "solaris",
    "wasip1",
    "windows",
}
GOARCH = {
    "386",
    "amd64",
    "amd64p32",
    "arm",
    "arm64",
    "loong64",
    "mips",
    "mips64",
    "mips64le",
    "mipsle",
    "ppc64",
    "ppc64le",
    "riscv64",
    "s390x",
    "sparc64",
    "wasm",
}
LIMITS = [
    "Observations only: package reachability and cycles are not an architecture or dead-code verdict.",
    "The built-in reader parses package/import headers and build constraints, not complete Go syntax or types.",
    "Tests and inactive build-tag variants are indexed but excluded from runtime reachability for each profile.",
    "External modules, generated files outside the repository inventory, cgo headers and linker targets are not traversed.",
    "Entrypoint reachability follows static local imports and is not an execution trace or proof that every symbol runs.",
    "Embed and linkname directives are recorded as registration evidence but their assets and symbols are not validated.",
    "Compilation, native linkage and package tests remain the responsibility of build.sh and the Go build-profile planner.",
]


def _tokenize(source: str) -> tuple[list[dict], list[dict]]:
    tokens = []
    issues = []
    index = 0
    line = 1
    length = len(source)
    while index < length:
        char = source[index]
        if char in " \t\r\f":
            index += 1
            continue
        if char == "\n":
            tokens.append({"kind": "newline", "value": "\n", "line": line})
            line += 1
            index += 1
            continue
        if source.startswith("//", index):
            end = source.find("\n", index)
            index = length if end < 0 else end
            continue
        if source.startswith("/*", index):
            start_line = line
            end = source.find("*/", index + 2)
            if end < 0:
                issues.append({"line": start_line, "kind": "unterminated_block_comment", "detail": "/*"})
                break
            block = source[index : end + 2]
            for _ in range(block.count("\n")):
                tokens.append({"kind": "newline", "value": "\n", "line": line})
                line += 1
            index = end + 2
            continue
        if char in {'"', "`", "'"}:
            delimiter = char
            start = index
            start_line = line
            index += 1
            escaped = False
            while index < length:
                current = source[index]
                if current == "\n":
                    line += 1
                    if delimiter != "`":
                        break
                if delimiter != "`" and current == "\\" and not escaped:
                    escaped = True
                    index += 1
                    continue
                if current == delimiter and not escaped:
                    index += 1
                    break
                escaped = False
                index += 1
            else:
                issues.append({"line": start_line, "kind": "unterminated_literal", "detail": delimiter})
            tokens.append({"kind": "string" if delimiter != "'" else "rune", "value": source[start:index], "line": start_line})
            continue
        if char == "_" or char.isalpha():
            start = index
            index += 1
            while index < length and (source[index] == "_" or source[index].isalnum()):
                index += 1
            tokens.append({"kind": "ident", "value": source[start:index], "line": line})
            continue
        tokens.append({"kind": "symbol", "value": char, "line": line})
        index += 1
    return tokens, issues


def _unquote_import(token: dict, issues: list[dict]) -> str | None:
    raw = token["value"]
    if raw.startswith("`") and raw.endswith("`"):
        value = raw[1:-1]
        if "\n" in value or "\r" in value:
            issues.append({"line": token["line"], "kind": "invalid_import_literal", "detail": raw})
            return None
        return value
    try:
        value = json.loads(raw)
    except (json.JSONDecodeError, TypeError):
        issues.append({"line": token["line"], "kind": "unsupported_import_escape", "detail": raw})
        return None
    return value if isinstance(value, str) else None


def _skip_separators(tokens: list[dict], index: int) -> int:
    while index < len(tokens) and (tokens[index]["kind"] == "newline" or tokens[index]["value"] == ";"):
        index += 1
    return index


def _import_spec(tokens: list[dict], index: int, issues: list[dict]) -> tuple[dict | None, int]:
    if index >= len(tokens):
        return None, index
    alias = None
    start = tokens[index]
    if start["kind"] == "string":
        literal = start
        index += 1
    elif start["kind"] == "ident" or start["value"] == ".":
        alias = start["value"]
        index += 1
        if index >= len(tokens) or tokens[index]["kind"] != "string":
            issues.append({"line": start["line"], "kind": "invalid_import_spec", "detail": alias})
            return None, index
        literal = tokens[index]
        index += 1
    else:
        issues.append({"line": start["line"], "kind": "invalid_import_spec", "detail": start["value"]})
        return None, index + 1
    target = _unquote_import(literal, issues)
    if target is None:
        return None, index
    return {"path": target, "alias": alias, "blank": alias == "_", "dot": alias == ".", "line": literal["line"]}, index


def parse_go_source(path: str, source: str) -> dict:
    tokens, issues = _tokenize(source)
    index = _skip_separators(tokens, 0)
    if index >= len(tokens) or tokens[index]["kind"] != "ident" or tokens[index]["value"] != "package":
        issues.append({"line": 1, "kind": "missing_package_clause", "detail": path})
        package = None
        package_line = 0
    else:
        index += 1
        if index >= len(tokens) or tokens[index]["kind"] != "ident":
            issues.append({"line": tokens[index - 1]["line"], "kind": "invalid_package_clause", "detail": path})
            package = None
            package_line = tokens[index - 1]["line"]
        else:
            package = tokens[index]["value"]
            package_line = tokens[index]["line"]
            index += 1
    imports = []
    index = _skip_separators(tokens, index)
    while index < len(tokens) and tokens[index]["kind"] == "ident" and tokens[index]["value"] == "import":
        declaration = tokens[index]
        index += 1
        if index < len(tokens) and tokens[index]["value"] == "(":
            index += 1
            while True:
                index = _skip_separators(tokens, index)
                if index >= len(tokens):
                    issues.append({"line": declaration["line"], "kind": "unterminated_import_group", "detail": path})
                    break
                if tokens[index]["value"] == ")":
                    index += 1
                    break
                spec, index = _import_spec(tokens, index, issues)
                if spec:
                    imports.append(spec)
                while index < len(tokens) and tokens[index]["kind"] != "newline" and tokens[index]["value"] not in {";", ")"}:
                    issues.append({"line": tokens[index]["line"], "kind": "unexpected_import_token", "detail": tokens[index]["value"]})
                    index += 1
        else:
            spec, index = _import_spec(tokens, index, issues)
            if spec:
                imports.append(spec)
            while index < len(tokens) and tokens[index]["kind"] != "newline" and tokens[index]["value"] != ";":
                issues.append({"line": tokens[index]["line"], "kind": "unexpected_import_token", "detail": tokens[index]["value"]})
                index += 1
        index = _skip_separators(tokens, index)

    header = source.splitlines()[: max(package_line - 1, 0)]
    go_build = []
    plus_build = []
    for line_number, text in enumerate(header, 1):
        match = re.fullmatch(r"\s*//go:build\s+(.+?)\s*", text)
        if match:
            go_build.append({"line": line_number, "expression": match.group(1)})
        match = re.fullmatch(r"\s*//\s*\+build\s+(.+?)\s*", text)
        if match:
            plus_build.append({"line": line_number, "expression": match.group(1)})
    if len(go_build) > 1:
        issues.append({"line": go_build[1]["line"], "kind": "multiple_go_build_directives", "detail": path})

    directives = []
    for line_number, text in enumerate(source.splitlines(), 1):
        match = re.fullmatch(r"\s*//go:([A-Za-z0-9_]+)(?:\s+(.*?))?\s*", text)
        if match and match.group(1) != "build":
            directives.append({"kind": match.group(1), "arguments": match.group(2) or "", "line": line_number})
    return {
        "path": path,
        "directory": PurePosixPath(path).parent.as_posix(),
        "package": package,
        "is_test": path.endswith("_test.go"),
        "build_expression": go_build[0]["expression"] if go_build else None,
        "legacy_build_expressions": [item["expression"] for item in plus_build],
        "imports": imports,
        "directives": directives,
        "issues": [{"source": path, **issue} for issue in issues],
    }


class _BuildExpression:
    def __init__(self, expression: str, tags: set[str]):
        compact = re.sub(r"\s+", "", expression)
        self.tokens = re.findall(r"&&|\|\||!|\(|\)|[A-Za-z0-9_.]+", compact)
        if "".join(self.tokens) != compact:
            raise ValueError(f"Unsupported build expression: {expression}")
        self.index = 0
        self.tags = tags

    def parse(self) -> bool:
        result = self._or()
        if self.index != len(self.tokens):
            raise ValueError("Trailing build expression tokens")
        return result

    def _or(self) -> bool:
        result = self._and()
        while self._take("||"):
            right = self._and()
            result = result or right
        return result

    def _and(self) -> bool:
        result = self._unary()
        while self._take("&&"):
            right = self._unary()
            result = result and right
        return result

    def _unary(self) -> bool:
        if self._take("!"):
            return not self._unary()
        if self._take("("):
            result = self._or()
            if not self._take(")"):
                raise ValueError("Unclosed build expression group")
            return result
        if self.index >= len(self.tokens) or self.tokens[self.index] in {"&&", "||", ")"}:
            raise ValueError("Missing build expression operand")
        token = self.tokens[self.index]
        self.index += 1
        return token in self.tags

    def _take(self, token: str) -> bool:
        if self.index < len(self.tokens) and self.tokens[self.index] == token:
            self.index += 1
            return True
        return False


def _profile_tags(profile: dict) -> set[str]:
    tags = set(profile.get("tags", [])) | {profile["goos"], profile["goarch"]}
    if profile["cgo"]:
        tags.add("cgo")
    major, minor = (int(item) for item in profile["go_version"].split(".", 1))
    if major == 1:
        tags.update(f"go1.{item}" for item in range(1, minor + 1))
    return tags


def _filename_active(path: str, profile: dict) -> bool:
    name = PurePosixPath(path).name.removesuffix(".go").removesuffix("_test")
    parts = name.split("_")
    if len(parts) >= 3 and parts[-2] in GOOS and parts[-1] in GOARCH:
        return parts[-2] == profile["goos"] and parts[-1] == profile["goarch"]
    if parts[-1] in GOOS:
        return parts[-1] == profile["goos"]
    if parts[-1] in GOARCH:
        return parts[-1] == profile["goarch"]
    return True


def _legacy_active(expressions: list[str], tags: set[str]) -> bool:
    for expression in expressions:
        options = expression.split()
        if not options:
            raise ValueError("Empty legacy build expression")
        option_results = []
        for option in options:
            terms = option.split(",")
            if any(not term for term in terms):
                raise ValueError(f"Invalid legacy build option: {option}")
            term_results = []
            for term in terms:
                negated = term.startswith("!")
                tag = term[1:] if negated else term
                if not tag or not re.fullmatch(r"[A-Za-z0-9_.]+", tag):
                    raise ValueError(f"Invalid legacy build term: {term}")
                active = tag in tags
                term_results.append(not active if negated else active)
            option_results.append(all(term_results))
        if not any(option_results):
            return False
    return True


def _file_active(node: dict, profile: dict) -> tuple[bool, str | None]:
    if node["is_test"] or not _filename_active(node["path"], profile):
        return False, None
    tags = _profile_tags(profile)
    try:
        if node["build_expression"] is not None:
            return _BuildExpression(node["build_expression"], tags).parse(), None
        return _legacy_active(node["legacy_build_expressions"], tags), None
    except ValueError as error:
        return False, str(error)


def _cycles(nodes: set[str], edges: list[dict]) -> list[list[str]]:
    outgoing = {node: [] for node in nodes}
    for edge in edges:
        if edge["source_package"] in nodes and edge["target_package"] in nodes:
            outgoing[edge["source_package"]].append(edge["target_package"])
    index = 0
    stack = []
    on_stack = set()
    indices = {}
    lowlinks = {}
    components = []

    def connect(node):
        nonlocal index
        indices[node] = lowlinks[node] = index
        index += 1
        stack.append(node)
        on_stack.add(node)
        for target in outgoing[node]:
            if target not in indices:
                connect(target)
                lowlinks[node] = min(lowlinks[node], lowlinks[target])
            elif target in on_stack:
                lowlinks[node] = min(lowlinks[node], indices[target])
        if lowlinks[node] != indices[node]:
            return
        component = []
        while True:
            member = stack.pop()
            on_stack.remove(member)
            component.append(member)
            if member == node:
                break
        if len(component) > 1 or node in outgoing[node]:
            components.append(sorted(component))

    for node in sorted(nodes):
        if node not in indices:
            connect(node)
    return sorted(components, key=lambda component: (component[0], len(component)))


def _module_identity(source: str) -> tuple[str, str]:
    module = re.search(r"(?m)^module\s+(\S+)\s*$", source)
    version = re.search(r"(?m)^go\s+(\S+)\s*$", source)
    if not module or not version:
        raise ValueError("go.mod must declare module and Go version")
    return module.group(1), version.group(1)


def analyze(parsed: list[dict], config: dict, ownership: dict[str, dict], module_path: str) -> dict:
    by_path = {node["path"]: node for node in parsed}
    by_directory = {}
    for node in parsed:
        by_directory.setdefault(node["directory"], []).append(node)
    all_edges = []
    global_issues = [issue for node in parsed for issue in node["issues"]]
    for node in parsed:
        for item in node["imports"]:
            specifier = item["path"]
            edge = {
                "source": node["path"],
                "source_package": node["directory"],
                "specifier": specifier,
                "alias": item["alias"],
                "blank": item["blank"],
                "dot": item["dot"],
                "line": item["line"],
            }
            if specifier == "C":
                edge.update(resolution="cgo", target_package=None)
            elif specifier.startswith("."):
                edge.update(resolution="invalid_relative", target_package=None)
                global_issues.append({"source": node["path"], "line": item["line"], "kind": "relative_go_import", "detail": specifier})
            elif specifier == module_path or specifier.startswith(module_path + "/"):
                target = "." if specifier == module_path else specifier[len(module_path) + 1 :]
                if target in by_directory:
                    edge.update(resolution="local", target_package=target)
                else:
                    edge.update(resolution="unresolved_local", target_package=target)
                    global_issues.append({"source": node["path"], "line": item["line"], "kind": "unresolved_local_import", "detail": specifier})
            else:
                edge.update(resolution="external", target_package=None)
            all_edges.append(edge)

    profile_reports = []
    for profile in config["profiles"]:
        profile_issues = []
        active = set()
        for node in parsed:
            selected, error = _file_active(node, profile)
            if error:
                profile_issues.append({"source": node["path"], "line": 1, "kind": "invalid_build_expression", "detail": error})
            if selected:
                active.add(node["path"])
        active_by_directory = {}
        for path in active:
            active_by_directory.setdefault(by_path[path]["directory"], []).append(by_path[path])
        for directory, nodes in active_by_directory.items():
            names = sorted({node["package"] for node in nodes if node["package"]})
            if len(names) != 1:
                profile_issues.append({"source": directory, "line": 0, "kind": "mixed_active_packages", "detail": names})

        profile_edges = []
        for edge in all_edges:
            if edge["source"] not in active or edge["resolution"] != "local":
                continue
            target_active = active_by_directory.get(edge["target_package"], [])
            if not target_active:
                profile_issues.append(
                    {
                        "source": edge["source"],
                        "line": edge["line"],
                        "kind": "inactive_local_target",
                        "detail": edge["specifier"],
                    }
                )
                continue
            profile_edges.append(edge)

        package_edges = []
        keys = sorted({(edge["source_package"], edge["target_package"]) for edge in profile_edges})
        for source, target in keys:
            contributors = [edge for edge in profile_edges if edge["source_package"] == source and edge["target_package"] == target]
            package_edges.append(
                {
                    "source_package": source,
                    "target_package": target,
                    "source_files": sorted({edge["source"] for edge in contributors}),
                    "blank_import": any(edge["blank"] for edge in contributors),
                }
            )
        outgoing = {}
        for edge in package_edges:
            outgoing.setdefault(edge["source_package"], []).append(edge["target_package"])

        entrypoints = []
        reachable = set()
        root_packages = set()
        queue = deque()
        for declaration in profile["entrypoints"]:
            path = declaration["path"]
            node = by_path.get(path)
            if node is None:
                profile_issues.append({"source": path, "line": 0, "kind": "missing_entrypoint", "detail": path})
                continue
            override = bool(declaration.get("ignore_build_constraints"))
            if path not in active and not override:
                profile_issues.append({"source": path, "line": 1, "kind": "inactive_entrypoint", "detail": path})
                continue
            root_packages.add(node["directory"])
            root_targets = []
            for edge in all_edges:
                if edge["source"] != path or edge["resolution"] != "local":
                    continue
                if edge["target_package"] not in active_by_directory:
                    profile_issues.append({"source": path, "line": edge["line"], "kind": "inactive_entrypoint_target", "detail": edge["specifier"]})
                    continue
                root_targets.append(edge["target_package"])
                queue.append(edge["target_package"])
            entrypoints.append(
                {
                    **declaration,
                    "constraint_override_used": path not in active and override,
                    "direct_local_packages": sorted(set(root_targets)),
                }
            )
        while queue:
            current = queue.popleft()
            if current in reachable:
                continue
            reachable.add(current)
            queue.extend(target for target in outgoing.get(current, []) if target not in reachable)
        reachable.update(root_packages)
        package_nodes = set(active_by_directory)
        cycles = []
        for members in _cycles(package_nodes, package_edges):
            cycles.append(
                {
                    "members": members,
                    "reachable_from_entrypoint": bool(set(members) & reachable),
                    "interpretation": "profile-specific package-cycle signal; policy is not evaluated",
                }
            )
        profile_reports.append(
            {
                "id": profile["id"],
                "goos": profile["goos"],
                "goarch": profile["goarch"],
                "cgo": profile["cgo"],
                "tags": sorted(_profile_tags(profile)),
                "entrypoints": entrypoints,
                "active_runtime_files": len(active),
                "active_packages": len(package_nodes),
                "local_package_edges": len(package_edges),
                "reachable_packages": sorted(reachable),
                "unreachable_packages": sorted(package_nodes - reachable),
                "cycles": cycles,
                "incomplete_reasons": profile_issues,
            }
        )

    owned_files = {path for path in by_path if ownership.get(path, {}).get("origin") in {"extension", "core_change"}}
    owned_packages = {by_path[path]["directory"] for path in owned_files}
    package_nodes = []
    for directory, nodes in sorted(by_directory.items()):
        package_nodes.append(
            {
                "path": directory,
                "package_names": sorted({node["package"] for node in nodes if node["package"]}),
                "files": sorted(node["path"] for node in nodes),
                "runtime_files": sum(not node["is_test"] for node in nodes),
                "test_files": sum(node["is_test"] for node in nodes),
                "owned_files": sorted(node["path"] for node in nodes if node["path"] in owned_files),
                "owners": sorted({ownership[node["path"]]["owner"] for node in nodes if node["path"] in ownership and node["path"] in owned_files}),
            }
        )
    reverse_consumers = []
    for target in sorted(owned_packages):
        incoming = [edge for edge in all_edges if edge["resolution"] == "local" and edge["target_package"] == target and edge["source_package"] != target]
        reverse_consumers.append(
            {
                "package": target,
                "consumer_packages": sorted({edge["source_package"] for edge in incoming}),
                "consumer_files": sorted({edge["source"] for edge in incoming}),
                "incoming_edges": incoming,
                "interpretation": "direct static package imports only; absence is not a dead-code verdict",
            }
        )
    primary_reachable = set(profile_reports[0]["reachable_packages"]) if profile_reports else set()
    all_issues = global_issues + [issue for profile in profile_reports for issue in profile["incomplete_reasons"]]
    files = []
    for node in parsed:
        files.append(
            {
                **{key: value for key, value in node.items() if key != "issues"},
                **ownership.get(node["path"], {"origin": "upstream", "owner": "upstream"}),
            }
        )
    return {
        "analysis_status": "INCOMPLETE" if all_issues else "OBSERVED",
        "policy_status": "NOT_EVALUATED",
        "dead_code_status": "NOT_ANALYZED",
        "scope": {
            "indexed_files": len(parsed),
            "runtime_files": sum(not node["is_test"] for node in parsed),
            "test_files": sum(node["is_test"] for node in parsed),
            "packages": len(by_directory),
            "all_import_edges": len(all_edges),
            "local_import_edges": sum(edge["resolution"] == "local" for edge in all_edges),
            "cgo_imports": sum(edge["resolution"] == "cgo" for edge in all_edges),
            "owned_or_core_files": len(owned_files),
            "owned_packages": len(owned_packages),
            "owned_packages_unreachable_in_primary_profile": len(owned_packages - primary_reachable),
        },
        "files": files,
        "packages": package_nodes,
        "edges": all_edges,
        "profiles": profile_reports,
        "reverse_consumers": reverse_consumers,
        "unreachable_owned_packages_in_primary_profile": sorted(owned_packages - primary_reachable),
        "directives": [{"source": node["path"], **directive} for node in parsed for directive in node["directives"]],
        "incomplete_reasons": all_issues,
        "limits": LIMITS,
    }


def _normalize_config(raw: object) -> dict:
    if not isinstance(raw, dict) or raw.get("schema_version") != 1:
        raise ValueError("Unsupported Go analysis schema")
    if raw.get("source_root") != "." or raw.get("module_file") != "go.mod":
        raise ValueError("Go analysis must use the repository module root")
    profiles = raw.get("profiles")
    if not isinstance(profiles, list) or not profiles:
        raise ValueError("Go analysis must declare build profiles")
    identifiers = set()
    normalized = []
    for profile in profiles:
        if not isinstance(profile, dict):
            raise ValueError("Go profile must be an object")
        profile_id = profile.get("id")
        if not isinstance(profile_id, str) or not profile_id or profile_id in identifiers:
            raise ValueError("Invalid or duplicate Go profile ID")
        identifiers.add(profile_id)
        if profile.get("goos") not in GOOS or profile.get("goarch") not in GOARCH or type(profile.get("cgo")) is not bool:
            raise ValueError(f"Invalid platform for Go profile {profile_id}")
        if not re.fullmatch(r"1\.\d+", str(profile.get("go_version", ""))):
            raise ValueError(f"Invalid Go version for profile {profile_id}")
        tags = profile.get("tags", [])
        if not isinstance(tags, list) or any(not isinstance(tag, str) or not tag for tag in tags):
            raise ValueError(f"Invalid tags for Go profile {profile_id}")
        entrypoints = profile.get("entrypoints")
        if not isinstance(entrypoints, list) or not entrypoints:
            raise ValueError(f"Go profile {profile_id} must declare entrypoints")
        entries = []
        for entry in entrypoints:
            if not isinstance(entry, dict) or not isinstance(entry.get("path"), str):
                raise ValueError(f"Invalid entrypoint in Go profile {profile_id}")
            path = PurePosixPath(entry["path"])
            if path.is_absolute() or ".." in path.parts or path.suffix != ".go" or "\\" in entry["path"]:
                raise ValueError(f"Unsafe Go entrypoint: {entry['path']}")
            if entry.get("ignore_build_constraints") not in {None, True, False}:
                raise ValueError(f"Invalid constraint override for {entry['path']}")
            if entry.get("ignore_build_constraints") and not entry.get("rationale"):
                raise ValueError(f"Constraint override requires rationale: {entry['path']}")
            entries.append({**entry, "path": path.as_posix()})
        normalized.append({**profile, "tags": list(dict.fromkeys(tags)), "entrypoints": entries})
    return {**raw, "profiles": normalized}


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2])
    parser.add_argument("--profile", action="append", help="Exact build profile ID; repeat to select multiple profiles")
    parser.add_argument("--output", type=Path, required=True, help="Report path outside tracked/nonignored source files")
    args = parser.parse_args(argv)
    root, output = args.root.resolve(), args.output.resolve()
    try:
        if output.is_relative_to(root):
            git(root, "check-ignore", "--no-index", "-q", output.relative_to(root).as_posix())
        base_config = json.loads(safe_path(root, "tools/quality/upstream-base.json").read_text(encoding="utf-8"))
        mapping = yaml.safe_load(safe_path(root, "tools/quality/module-map.yaml").read_text(encoding="utf-8"))
        policy_path = safe_path(root, "tools/quality/go-analysis.yaml")
        config = _normalize_config(yaml.safe_load(policy_path.read_text(encoding="utf-8")))
        if args.profile:
            selected = set(args.profile)
            known = {profile["id"] for profile in config["profiles"]}
            if selected - known:
                raise ValueError("Unknown Go analysis profile: " + ", ".join(sorted(selected - known)))
            config = {**config, "profiles": [profile for profile in config["profiles"] if profile["id"] in selected]}
        before = capture(root, base_config, mapping)
        repository_files = paths(git(root, "ls-files", "--cached", "--others", "--exclude-standard", "-z"))
        if output.is_relative_to(root) and output.relative_to(root).as_posix() in repository_files:
            raise ValueError("Report must not overwrite source files, including tracked files under ignore rules")
        source_paths = sorted(path for path in repository_files if path.endswith(".go") and safe_path(root, path).is_file())
        if not source_paths:
            raise ValueError("Go source inventory is empty")
        raw = {path: safe_path(root, path).read_bytes() for path in source_paths}
        module_path_file = safe_path(root, config["module_file"])
        module_source = module_path_file.read_text(encoding="utf-8")
        module_path, go_version = _module_identity(module_source)
        ownership = {record["path"]: {"origin": record["origin"], "owner": record["module"]} for record in before["records"]}
        parsed = [parse_go_source(path, raw[path].decode("utf-8")) for path in source_paths]
        report = analyze(parsed, config, ownership, module_path)
        for path, content in raw.items():
            if safe_path(root, path).read_bytes() != content:
                raise ValueError(f"Source changed during analysis: {path}")
        after = capture(root, base_config, mapping)
        if before["snapshot_sha256"] != after["snapshot_sha256"] or before["head"] != after["head"]:
            raise ValueError("Provenance snapshot changed during analysis")
        report.update(
            {
                "schema_version": 1,
                "tool": {"name": "inspect_go", "version": VERSION, "python": platform.python_version(), "os": platform.system()},
                "module": {"path": module_path, "declared_go_version": go_version},
                "input": {
                    "head": before["head"],
                    "upstream_base": before["upstream_base"],
                    "snapshot_sha256": before["snapshot_sha256"],
                    "source_sha256": {path: hashlib.sha256(content).hexdigest() for path, content in raw.items()},
                    "tool_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                    "policy_sha256": hashlib.sha256(policy_path.read_bytes()).hexdigest(),
                    "module_file_sha256": hashlib.sha256(module_path_file.read_bytes()).hexdigest(),
                },
                "command": sys.argv if argv is None else argv,
            }
        )
        code = 2 if report["analysis_status"] == "INCOMPLETE" else 0
        report["exit_code"] = code
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print(
            json.dumps(
                {
                    "analysis_status": report["analysis_status"],
                    "policy_status": report["policy_status"],
                    "indexed_files": report["scope"]["indexed_files"],
                    "packages": report["scope"]["packages"],
                    "profiles": len(report["profiles"]),
                    "incomplete_reasons": len(report["incomplete_reasons"]),
                    "output": str(output),
                }
            )
        )
        return code
    except (KeyError, TypeError, ValueError, OSError, UnicodeError, subprocess.CalledProcessError) as error:
        print(f"INCOMPLETE: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
