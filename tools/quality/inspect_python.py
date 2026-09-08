"""Observe Python imports and complexity signals; never import or modify application code."""

from __future__ import annotations

import argparse
import ast
from collections import deque
import hashlib
import importlib.util
import json
from pathlib import Path
import platform
import subprocess
import sys
import tokenize
from io import BytesIO

import yaml

from capture_inventory import capture, git, paths, safe_path


VERSION = "0.5.0"
SOURCE_ROOTS = ("api", "admin", "agent", "business_documents", "common", "rag", "deepdoc")
MARKERS = {
    "bootstrap": ("api.apps", "api.ragflow_server"),
    "orm": ("peewee", "playhouse", "api.db.db_models"),
    "http_framework": ("quart", "flask", "fastapi"),
    "network_client": ("requests", "httpx", "aiohttp", "redis", "elasticsearch", "minio", "openai"),
}
LIMITS = [
    "Observations only: no approved layer allowlist, baseline or architecture acceptance.",
    "Potential dependencies include function bodies and conditional branches; they are not an execution trace.",
    "External libraries, custom import hooks, sys.path mutation and reflection are not resolved transitively.",
    "A possible from-import submodule may be shadowed by a package export; its edge is conservative.",
    "No dead-code verdict, deletion, call graph, runtime benchmark or asymptotic-complexity proof.",
    "No direct static consumer does not mean dead code; registries, persisted DSL, reflection and external contracts need separate evidence.",
    "Root Python profile only; secondary source roots, frontend and Go require separate analyzers.",
]


def module_index(sources, import_roots=frozenset()):
    index = {}
    packages = set()
    normalized_roots = sorted({root.rstrip("/") + "/" for root in import_roots}, key=len, reverse=True)
    for path in sorted(sources):
        matching_root = next((root for root in normalized_roots if path.startswith(root)), None)
        module_path = path[len(matching_root) :] if matching_root else path
        parts = list(Path(module_path).with_suffix("").parts)
        if parts[-1] == "__init__":
            parts.pop()
        if not parts or not all(part.isidentifier() for part in parts):
            raise ValueError(f"Unsupported Python module path: {path}")
        name = ".".join(parts)
        if name in index:
            raise ValueError(f"Ambiguous module {name}: {index[name]}, {path}")
        index[name] = path
        packages.update(".".join(parts[:i]) for i in range(1, len(parts)))
        if path.endswith("/__init__.py"):
            packages.add(name)
    return index, packages


class Imports(ast.NodeVisitor):
    def __init__(self, name, path, tree, index, packages):
        self.name, self.path, self.index, self.packages = name, path, index, packages
        self.package = name if path.endswith("/__init__.py") else name.rpartition(".")[0]
        self.local_roots = {module.split(".")[0] for module in index}
        self.deferred_annotations = any(isinstance(node, ast.ImportFrom) and node.module == "__future__" and any(alias.name == "annotations" for alias in node.names) for node in tree.body)
        self.edges, self.issues = [], []
        self.phase, self.conditional, self.type_only = "module", False, False
        self.scope = "module"
        self.aliases = {}
        self.shadowed = set()
        self.dynamic_aliases = set()

        def bind(alias, target):
            if alias in self.aliases and self.aliases[alias] != target:
                self.shadowed.add(alias)
            if target in {"importlib.import_module", "__import__"}:
                self.dynamic_aliases.add(alias)
            self.aliases[alias] = target

        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                for alias in node.names:
                    bind(alias.asname or alias.name.split(".")[0], alias.name if alias.asname else alias.name.split(".")[0])
            elif isinstance(node, ast.ImportFrom) and node.level == 0:
                for alias in node.names:
                    bind(alias.asname or alias.name, f"{node.module}.{alias.name}")
            elif isinstance(node, ast.Name) and isinstance(node.ctx, ast.Store):
                self.shadowed.add(node.id)
            elif isinstance(node, ast.arg):
                self.shadowed.add(node.arg)

    def issue(self, node, kind, detail):
        self.issues.append({"source": self.name, "path": self.path, "line": getattr(node, "lineno", 0), "kind": kind, "detail": detail, "type_only": self.type_only})

    def edge(self, target, node, kind="import", symbols=None):
        if target in self.index:
            resolution = "local"
        elif target in self.packages:
            resolution = "namespace"
        elif target.split(".")[0] in self.local_roots:
            resolution = "unresolved_local"
            self.issue(node, resolution, target)
        else:
            resolution = "stdlib" if target.split(".")[0] in sys.stdlib_module_names else "external"
        self.edges.append(
            {
                "source": self.name,
                "target": target,
                "path": self.path,
                "line": getattr(node, "lineno", 0),
                "kind": kind,
                "phase": self.phase,
                "conditional": self.conditional,
                "type_only": self.type_only,
                "resolution": resolution,
                "symbols": sorted(set(symbols or [])),
            }
        )
        # Importing a dotted local target also initializes its parent packages.
        for i in range(1, len(target.split("."))):
            parent = ".".join(target.split(".")[:i])
            if parent != self.name and self.index.get(parent, "").endswith("/__init__.py"):
                self.edges.append({**self.edges[-1], "target": parent, "kind": "parent_init", "resolution": "local", "symbols": []})

    def visit_Import(self, node):
        for alias in node.names:
            self.edge(alias.name, node)

    def visit_ImportFrom(self, node):
        try:
            target = importlib.util.resolve_name("." * node.level + (node.module or ""), self.package) if node.level else node.module
        except (ImportError, ValueError):
            self.issue(node, "invalid_relative_import", "Relative import cannot be resolved in this package")
            return
        self.edge(target, node, symbols=[alias.name for alias in node.names])
        for alias in node.names:
            if alias.name == "*":
                self.issue(node, "star_exports", target)
            elif f"{target}.{alias.name}" in self.index or f"{target}.{alias.name}" in self.packages:
                self.edge(f"{target}.{alias.name}", node, "possible_submodule")

    def dotted(self, node):
        if isinstance(node, ast.Name):
            if node.id in self.shadowed:
                return ""
            return self.aliases.get(node.id, node.id)
        if isinstance(node, ast.Attribute):
            prefix = self.dotted(node.value)
            return f"{prefix}.{node.attr}" if prefix else ""
        return ""

    def visit_Call(self, node):
        name = self.dotted(node.func)
        dynamic = {"importlib.import_module", "__import__"}
        if name in dynamic:
            value = node.args[0].value if node.args and isinstance(node.args[0], ast.Constant) else None
            level = next((keyword.value for keyword in node.keywords if keyword.arg == "level"), node.args[4] if len(node.args) > 4 else ast.Constant(0))
            relative_builtin = name == "__import__" and (not isinstance(level, ast.Constant) or level.value != 0)
            if isinstance(value, str) and value and not value.startswith(".") and not relative_builtin:
                self.edge(value, node, "literal_dynamic_import")
            else:
                self.issue(node, "dynamic_import", "Computed/relative target needs runtime registration evidence")
        elif name in {"exec", "eval", "importlib.util.spec_from_file_location", "importlib.util.module_from_spec"} or isinstance(node.func, ast.Attribute) and node.func.attr == "exec_module":
            self.issue(node, "dynamic_loading", name or "exec_module")
        elif isinstance(node.func, ast.Name) and node.func.id in self.shadowed and node.func.id in self.dynamic_aliases:
            self.issue(node, "shadowed_import_alias", node.func.id)
        elif isinstance(node.func, ast.Attribute) and node.func.attr == "import_module" and not name:
            self.issue(node, "shadowed_import_alias", "import_module receiver")
        self.generic_visit(node)

    def visit_Assign(self, node):
        if self.dotted(node.value) in {"importlib.import_module", "__import__"}:
            self.issue(node, "import_callable_escape", "Import callable assigned to another binding")
        self.generic_visit(node)

    def visit_AnnAssign(self, node):
        if node.value is not None:
            if self.dotted(node.value) in {"importlib.import_module", "__import__"}:
                self.issue(node, "import_callable_escape", "Import callable assigned to annotated binding")
            self.visit(node.value)
        previous = self.type_only
        self.type_only = previous or self.deferred_annotations
        self.visit(node.annotation)
        self.type_only = previous

    def visit_NamedExpr(self, node):
        if self.dotted(node.value) in {"importlib.import_module", "__import__"}:
            self.issue(node, "import_callable_escape", "Import callable assigned by expression")
        self.generic_visit(node)

    def visit_If(self, node):
        previous = self.conditional, self.type_only
        flag = self.dotted(node.test) == "typing.TYPE_CHECKING"
        negative = isinstance(node.test, ast.UnaryOp) and isinstance(node.test.op, ast.Not) and self.dotted(node.test.operand) == "typing.TYPE_CHECKING"
        self.visit(node.test)
        self.conditional = True
        self.type_only = previous[1] or flag
        for child in node.body:
            self.visit(child)
        self.type_only = previous[1] or negative
        for child in node.orelse:
            self.visit(child)
        self.conditional, self.type_only = previous

    def visit_Try(self, node):
        previous = self.conditional
        self.conditional = True
        self.generic_visit(node)
        self.conditional = previous

    visit_TryStar = visit_Try

    def visit_FunctionDef(self, node):
        if node.name == "__getattr__" and self.scope == "module":
            self.issue(node, "dynamic_exports", "Module-level __getattr__")
        previous = self.phase
        previous_scope = self.scope
        # Defaults and decorators execute when a definition is evaluated.
        for child in [*node.decorator_list, *node.args.defaults, *filter(None, node.args.kw_defaults)]:
            self.visit(child)
        type_only = self.type_only
        self.type_only = type_only or self.deferred_annotations
        arguments = [*node.args.posonlyargs, *node.args.args, *node.args.kwonlyargs, node.args.vararg, node.args.kwarg]
        for annotation in [*(arg.annotation for arg in arguments if arg is not None and arg.annotation is not None), node.returns]:
            if annotation is not None:
                self.visit(annotation)
        self.type_only = type_only
        self.phase = "call"
        self.scope = "function"
        for child in node.body:
            self.visit(child)
        self.phase = previous
        self.scope = previous_scope

    visit_AsyncFunctionDef = visit_FunctionDef

    def visit_ClassDef(self, node):
        for child in [*node.bases, *node.decorator_list, *(keyword.value for keyword in node.keywords)]:
            self.visit(child)
        previous = self.scope
        self.scope = "class"
        for child in node.body:
            self.visit(child)
        self.scope = previous

    def visit_Lambda(self, node):
        for child in [*node.args.defaults, *filter(None, node.args.kw_defaults)]:
            self.visit(child)
        previous = self.phase
        self.phase = "call"
        self.visit(node.body)
        self.phase = previous


def function_metrics(tree, path):
    results = []

    def inspect(node, prefix=""):
        for child in ast.iter_child_nodes(node):
            if isinstance(child, (ast.FunctionDef, ast.AsyncFunctionDef)):
                name = prefix + child.name
                counts = {"branch_points": 0, "max_nesting": 0, "max_loop_nesting": 0, "statements": 0}

                def measure(part, depth=0, loops=0):
                    if isinstance(part, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef, ast.Lambda)):
                        return
                    counts["statements"] += int(isinstance(part, ast.stmt))
                    counts["branch_points"] += int(isinstance(part, (ast.If, ast.IfExp, ast.For, ast.AsyncFor, ast.While, ast.ExceptHandler, ast.comprehension)))
                    if isinstance(part, ast.BoolOp):
                        counts["branch_points"] += len(part.values) - 1
                    if isinstance(part, ast.Match):
                        counts["branch_points"] += sum(not isinstance(case.pattern, ast.MatchAs) or case.pattern.pattern is not None for case in part.cases)
                        counts["branch_points"] += sum(case.guard is not None for case in part.cases)
                    if isinstance(part, ast.comprehension):
                        counts["branch_points"] += len(part.ifs)
                    if isinstance(part, (ast.If, ast.For, ast.AsyncFor, ast.While, ast.Try, ast.TryStar, ast.With, ast.AsyncWith, ast.Match)):
                        depth += 1
                    if isinstance(part, (ast.For, ast.AsyncFor, ast.While, ast.comprehension)):
                        loops += 1
                    counts["max_nesting"] = max(counts["max_nesting"], depth)
                    counts["max_loop_nesting"] = max(counts["max_loop_nesting"], loops)
                    if isinstance(part, (ast.ListComp, ast.SetComp, ast.DictComp, ast.GeneratorExp)):
                        for i, generator in enumerate(part.generators):
                            measure(generator, depth, loops + i)
                        elements = [part.key, part.value] if isinstance(part, ast.DictComp) else [part.elt]
                        for element in elements:
                            measure(element, depth, loops + len(part.generators))
                        return
                    for nested in ast.iter_child_nodes(part):
                        measure(nested, depth, loops)

                for statement in child.body:
                    measure(statement)
                signals = [key for key, threshold in (("branch_points", 20), ("max_nesting", 5), ("max_loop_nesting", 2)) if counts[key] >= threshold]
                results.append({"path": path, "symbol": name, "line": child.lineno, "end_line": child.end_lineno, **counts, "signals": signals})
                inspect(child, name + ".")
            else:
                inspect(child, prefix + child.name + "." if isinstance(child, ast.ClassDef) else prefix)

    inspect(tree)
    return results


def collect_import_graph(sources, selected=frozenset(), import_roots=frozenset()):
    index, packages = module_index(sources, import_roots)
    edges, issues, metrics = [], [], []
    for name, path in index.items():
        try:
            tree = ast.parse(sources[path], filename=path)
        except SyntaxError as error:
            issues.append({"source": name, "path": path, "line": error.lineno, "kind": "syntax_error", "detail": error.msg, "type_only": False})
            continue
        visitor = Imports(name, path, tree, index, packages)
        for i in range(1, len(name.split("."))):
            parent = ".".join(name.split(".")[:i])
            if index.get(parent, "").endswith("/__init__.py"):
                visitor.edge(parent, tree, "parent_init")
        visitor.visit(tree)
        edges.extend(visitor.edges)
        issues.extend(visitor.issues)
        if path in selected:
            metrics.extend(function_metrics(tree, path))
    edges = list({json.dumps(edge, sort_keys=True): edge for edge in edges}.values())
    return {"index": index, "packages": packages, "edges": edges, "issues": issues, "functions": metrics}


def analyze(sources, selected, ownership, import_roots=frozenset()):
    graph = collect_import_graph(sources, selected, import_roots)
    index, edges, issues, metrics = graph["index"], graph["edges"], graph["issues"], graph["functions"]
    outgoing = {}
    for edge in edges:
        if not edge["type_only"]:
            outgoing.setdefault(edge["source"], []).append(edge)
    observed, reached = [], set()
    for name, path in index.items():
        if path not in selected:
            continue
        queue, previous = deque([name]), {name: None}
        while queue:
            current = queue.popleft()
            reached.add(current)
            for group, targets in MARKERS.items():
                if any(current == target or group != "bootstrap" and current.startswith(target + ".") for target in targets):
                    chain, cursor = [], current
                    while previous[cursor] is not None:
                        edge = previous[cursor]
                        chain.append(edge)
                        cursor = edge["source"]
                    observed.append({"source": name, "path": path, "category": group, "target": current, "chain": list(reversed(chain)), "interpretation": "potential coupling; review required"})
            for edge in outgoing.get(current, []):
                if edge["target"] not in previous:
                    previous[edge["target"]] = edge
                    queue.append(edge["target"])
    relevant_issues = [issue for issue in issues if issue["source"] in reached and not issue["type_only"]]
    selected_modules = {name: path for name, path in index.items() if path in selected}
    incoming_by_target = {module: [] for module in selected_modules}
    for edge in edges:
        if edge["target"] not in incoming_by_target or edge["type_only"] or edge["source"] == edge["target"]:
            continue
        source_path = index.get(edge["source"])
        incoming_by_target[edge["target"]].append(
            {
                **edge,
                "source_path": source_path,
                "source_origin": ownership.get(source_path, {}).get("origin") if source_path else None,
                "source_owner": ownership.get(source_path, {}).get("owner") if source_path else None,
            }
        )
    reverse_imports = []
    for module, path in sorted(selected_modules.items()):
        incoming = incoming_by_target[module]
        incoming.sort(key=lambda edge: (edge["source"], edge["kind"], edge["line"], edge["path"]))
        reverse_imports.append(
            {
                "module": module,
                "path": path,
                "consumer_modules": sorted({edge["source"] for edge in incoming}),
                "incoming_edges": incoming,
                "interpretation": "direct potential static import consumers only; absence is not a dead-code verdict",
            }
        )
    return {
        "analysis_status": "INCOMPLETE" if relevant_issues else "OBSERVED",
        "policy_status": "NOT_EVALUATED",
        "dead_code_status": "NOT_ANALYZED",
        "scope": {
            "selected_paths": sorted(selected),
            "indexed_files": len(index),
            "reachable_modules": len(reached),
            "direct_static_consumer_edges": sum(len(item["incoming_edges"]) for item in reverse_imports),
            "selected_without_static_consumers": sum(not item["incoming_edges"] for item in reverse_imports),
        },
        "nodes": [{"module": name, "path": path, **ownership.get(path, {"origin": "unknown", "owner": None})} for name, path in index.items() if name in reached],
        "edges": [edge for edge in edges if edge["source"] in reached],
        "reverse_imports": reverse_imports,
        "coupling_paths": observed,
        "incomplete_reasons": relevant_issues,
        "other_parse_errors": [issue for issue in issues if issue["kind"] == "syntax_error" and issue not in relevant_issues],
        "functions": metrics,
        "limits": LIMITS,
    }


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2])
    parser.add_argument("--module", action="append", required=True, dest="modules", help="Exact owner module ID from module-map.yaml; repeat to select more")
    parser.add_argument("--output", type=Path, required=True, help="Report path outside tracked/nonignored source files")
    args = parser.parse_args(argv)
    root, output = args.root.resolve(), args.output.resolve()
    try:
        if output.is_relative_to(root):
            git(root, "check-ignore", "--no-index", "-q", output.relative_to(root).as_posix())
        config = json.loads((root / "tools/quality/upstream-base.json").read_text(encoding="utf-8"))
        mapping = yaml.safe_load((root / "tools/quality/module-map.yaml").read_text(encoding="utf-8"))
        modules = {module["id"]: module for module in mapping["modules"]}
        if set(args.modules) - modules.keys():
            raise ValueError("Unknown owner module ID")
        before = capture(root, config, mapping)
        files = paths(git(root, "ls-files", "--cached", "--others", "--exclude-standard", "-z"))
        if output.is_relative_to(root) and output.relative_to(root).as_posix() in files:
            raise ValueError("Report must not overwrite source files, including tracked files under ignore rules")
        profile = {path for path in files if path.endswith(".py") and path.split("/")[0] in SOURCE_ROOTS and (root / path).exists()}
        chosen = {path for module in args.modules for path in modules[module]["paths"]}
        selected = chosen & profile
        if not selected:
            raise ValueError("Selected module has no Python runtime files in this profile")
        raw = {path: safe_path(root, path).read_bytes() for path in sorted(profile)}
        sources = {path: content.decode(tokenize.detect_encoding(BytesIO(content).readline)[0]) for path, content in raw.items()}
        owners = {record["path"]: {"origin": record["origin"], "owner": record["module"]} for record in before["records"]}
        for path in sources:
            owners.setdefault(path, {"origin": "upstream", "owner": "upstream"})
        report = analyze(sources, selected, owners)
        for path, content in raw.items():
            if safe_path(root, path).read_bytes() != content:
                raise ValueError(f"Source changed during analysis: {path}")
        after = capture(root, config, mapping)
        if before["snapshot_sha256"] != after["snapshot_sha256"] or before["head"] != after["head"]:
            raise ValueError("Provenance snapshot changed during analysis")
        if report["other_parse_errors"]:
            report["analysis_status"] = "INCOMPLETE"
        report.update(
            {
                "schema_version": 1,
                "tool": {"name": "inspect_python", "version": VERSION, "python": platform.python_version(), "os": platform.system()},
                "profile": {"name": "root-python-api-worker", "source_roots": SOURCE_ROOTS, "selected_owners": args.modules, "excluded_owned_paths": sorted(chosen - selected)},
                "input": {
                    "head": before["head"],
                    "upstream_base": before["upstream_base"],
                    "snapshot_sha256": before["snapshot_sha256"],
                    "source_sha256": {path: hashlib.sha256(content).hexdigest() for path, content in raw.items()},
                    "tool_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
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
                    "selected_files": len(selected),
                    "coupling_paths": len(report["coupling_paths"]),
                    "incomplete_reasons": len(report["incomplete_reasons"]),
                    "output": str(output),
                }
            )
        )
        return code
    except (ValueError, OSError, SyntaxError, UnicodeError, subprocess.CalledProcessError) as error:
        print(f"INCOMPLETE: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
