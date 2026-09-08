from __future__ import annotations

from collections import Counter
import json
from pathlib import Path
import shutil
import subprocess

import pytest


ROOT = Path(__file__).resolve().parents[4]
TYPESCRIPT = ROOT / "web/node_modules/typescript/lib/typescript.js"
ROUTES = ROOT / "web/src/routes.tsx"
NAVIGATION = ROOT / "web/src/constants/navigation.ts"


_PARSE_TYPESCRIPT = r"""
const fs = require('fs');
const ts = require(process.argv[1]);

function source(path, kind) {
  return ts.createSourceFile(path, fs.readFileSync(path, 'utf8'), ts.ScriptTarget.Latest, true, kind);
}

const routes = source(process.argv[2], ts.ScriptKind.TSX);
const entries = [];

function namedProperty(node, name) {
  return node.properties.find((item) =>
    ts.isPropertyAssignment(item) && item.name.getText(routes) === name
  );
}

function importedTarget(node) {
  let target = null;
  function visit(item) {
    if (
      ts.isCallExpression(item) &&
      item.expression.kind === ts.SyntaxKind.ImportKeyword &&
      item.arguments.length === 1 &&
      ts.isStringLiteral(item.arguments[0])
    ) {
      target = item.arguments[0].text;
    }
    ts.forEachChild(item, visit);
  }
  visit(node);
  return target;
}

function visitRoutes(node) {
  if (ts.isObjectLiteralExpression(node)) {
    const path = namedProperty(node, 'path');
    const component = namedProperty(node, 'Component');
    if (path && component) {
      const target = importedTarget(component.initializer);
      if (target) entries.push({path: path.initializer.getText(routes), target});
    }
  }
  ts.forEachChild(node, visitRoutes);
}
visitRoutes(routes);

const navigationSource = source(process.argv[3], ts.ScriptKind.TS);
const navigation = {};
function visitNavigation(node) {
  if (
    ts.isVariableDeclaration(node) &&
    node.name.getText(navigationSource) === 'NavigationSectionPaths' &&
    ts.isObjectLiteralExpression(node.initializer)
  ) {
    for (const property of node.initializer.properties) {
      if (ts.isPropertyAssignment(property) && ts.isStringLiteral(property.initializer)) {
        navigation[property.name.getText(navigationSource)] = property.initializer.text;
      }
    }
  }
  ts.forEachChild(node, visitNavigation);
}
visitNavigation(navigationSource);

process.stdout.write(JSON.stringify({entries, navigation}));
"""


EXPECTED_ROUTES = {
    ("Routes.OpenMetadata", "@/pages/openmetadata"),
    ("Routes.BusinessDocuments", "@/pages/business-documents"),
    ("`${Routes.BusinessDocuments}/:id`", "@/pages/business-documents"),
    ("`${Routes.BusinessDocuments}/eva/:changeId`", "@/pages/business-documents"),
    ("Routes.AdminNavigationVisibility", "@/pages/admin/navigation-visibility"),
    ("Routes.AdminAccessGroups", "@/pages/admin/access-groups"),
    ("Routes.AdminBusinessDocumentsSettings", "@/pages/admin/business-documents-settings"),
    ("Routes.AdminAudit", "@/pages/admin/audit"),
}
EXPECTED_NAVIGATION = {
    "catalog": "/openmetadata",
    "business_documents": "/business-documents",
}


def _inventory(routes: Path = ROUTES, navigation: Path = NAVIGATION) -> dict:
    node = shutil.which("node")
    if node is None:
        raise RuntimeError("Node.js is required to parse frontend entrypoints")
    if not TYPESCRIPT.is_file():
        raise RuntimeError("Install the locked frontend dependencies before checking entrypoints")
    completed = subprocess.run(
        [node, "-e", _PARSE_TYPESCRIPT, str(TYPESCRIPT), str(routes), str(navigation)],
        cwd=ROOT,
        capture_output=True,
        text=True,
        timeout=20,
        check=False,
    )
    if completed.returncode:
        raise RuntimeError(f"TypeScript entrypoint parser failed: {completed.stderr[-2000:]}")
    return json.loads(completed.stdout)


def _target_exists(target: str) -> bool:
    relative = target.removeprefix("@/")
    path = ROOT / "web/src" / relative
    candidates = [path, path.with_suffix(".ts"), path.with_suffix(".tsx"), path / "index.ts", path / "index.tsx"]
    return any(candidate.is_file() for candidate in candidates)


def _assert_owned_entrypoints(inventory: dict) -> None:
    actual = Counter((entry["path"], entry["target"]) for entry in inventory["entries"])
    assert {entry: actual[entry] for entry in EXPECTED_ROUTES} == {entry: 1 for entry in EXPECTED_ROUTES}
    assert all(_target_exists(target) for _, target in EXPECTED_ROUTES)
    assert {key: inventory["navigation"].get(key) for key in EXPECTED_NAVIGATION} == EXPECTED_NAVIGATION


def test_owned_frontend_route_entrypoints_are_registered_and_resolvable():
    _assert_owned_entrypoints(_inventory())


def test_frontend_entrypoint_contract_rejects_changed_lazy_target(tmp_path):
    source = ROUTES.read_text(encoding="utf-8")
    changed = source.replace("import('@/pages/openmetadata')", "import('@/pages/home')", 1)
    assert changed != source
    routes = tmp_path / "routes.tsx"
    routes.write_text(changed, encoding="utf-8")

    with pytest.raises(AssertionError):
        _assert_owned_entrypoints(_inventory(routes=routes))
