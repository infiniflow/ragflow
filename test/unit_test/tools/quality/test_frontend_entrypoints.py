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
MAIN = ROOT / "web/src/main.tsx"
APP = ROOT / "web/src/app.tsx"
REGISTER_SERVER = ROOT / "web/src/utils/register-server.ts"


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


_CHECK_FRONTEND_REGISTRATIONS = r"""
const fs = require('fs');
const vm = require('vm');
const ts = require(process.argv[1]);

function source(path, kind) {
  return ts.createSourceFile(path, fs.readFileSync(path, 'utf8'), ts.ScriptTarget.Latest, true, kind);
}

function walk(node, visit) {
  visit(node);
  ts.forEachChild(node, (child) => walk(child, visit));
}

function callName(node, owner, name) {
  return ts.isCallExpression(node) &&
    ts.isPropertyAccessExpression(node.expression) &&
    node.expression.expression.getText(node.getSourceFile()) === owner &&
    node.expression.name.text === name;
}

function jsxTags(node, sourceFile) {
  const tags = [];
  walk(node, (item) => {
    if (ts.isJsxOpeningElement(item) || ts.isJsxSelfClosingElement(item)) {
      tags.push(item.tagName.getText(sourceFile));
    }
  });
  return tags;
}

function jsxAttribute(node, name, sourceFile) {
  const attribute = node.attributes.properties.find((item) =>
    ts.isJsxAttribute(item) && item.name.getText(sourceFile) === name
  );
  if (!attribute || !attribute.initializer) return null;
  if (ts.isStringLiteral(attribute.initializer)) return attribute.initializer.text;
  if (ts.isJsxExpression(attribute.initializer) && attribute.initializer.expression) {
    return attribute.initializer.expression.getText(sourceFile);
  }
  return attribute.initializer.getText(sourceFile);
}

function inspectMain(path) {
  const main = source(path, ts.ScriptKind.TSX);
  let telemetryIndex = -1;
  let languageIndex = -1;
  let initThenCount = 0;
  let renderCount = 0;
  let rootElementId = null;
  let renderTags = [];
  let inspectorKeys = null;
  let inspectorHandler = null;

  main.statements.forEach((statement, index) => {
    if (!ts.isExpressionStatement(statement) || !ts.isCallExpression(statement.expression)) return;
    const expression = statement.expression;
    if (ts.isIdentifier(expression.expression) && expression.expression.text === 'installClientTelemetry') {
      telemetryIndex = index;
    }
    if (
      ts.isPropertyAccessExpression(expression.expression) &&
      expression.expression.name.text === 'then' &&
      ts.isCallExpression(expression.expression.expression) &&
      ts.isIdentifier(expression.expression.expression.expression) &&
      expression.expression.expression.expression.text === 'initLanguage'
    ) {
      languageIndex = index;
      initThenCount += 1;
      const callback = expression.arguments[0];
      if (!callback) return;
      walk(callback, (node) => {
        if (
          !ts.isCallExpression(node) ||
          !ts.isPropertyAccessExpression(node.expression) ||
          node.expression.name.text !== 'render'
        ) return;
        const createRoot = node.expression.expression;
        if (!callName(createRoot, 'ReactDOM', 'createRoot')) return;
        renderCount += 1;
        walk(createRoot.arguments[0], (argumentNode) => {
          if (!callName(argumentNode, 'document', 'getElementById')) return;
          const id = argumentNode.arguments[0];
          if (id && ts.isStringLiteral(id)) rootElementId = id.text;
        });
        const tree = node.arguments[0];
        if (!tree) return;
        renderTags = jsxTags(tree, main);
        walk(tree, (element) => {
          if (
            !ts.isJsxSelfClosingElement(element) ||
            element.tagName.getText(main) !== 'Inspector'
          ) return;
          inspectorKeys = jsxAttribute(element, 'keys', main);
          inspectorHandler = jsxAttribute(element, 'onInspectElement', main);
        });
      });
    }
  });

  return {
    telemetryIndex,
    languageIndex,
    initThenCount,
    renderCount,
    rootElementId,
    renderTags,
    inspectorKeys,
    inspectorHandler,
  };
}

function inspectApp(path) {
  const app = source(path, ts.ScriptKind.TSX);
  let rootProviderTags = [];
  let rootProviderEffectCount = 0;
  let emptyEffectDependencies = false;
  let storedLanguageReads = 0;
  let conditionalLanguageChanges = 0;
  let appContainerTags = [];
  let appContainerRouterBindings = [];
  let routerWrapperTags = [];
  let routerBinding = null;

  for (const statement of app.statements) {
    if (ts.isVariableStatement(statement)) {
      for (const declaration of statement.declarationList.declarations) {
        if (!ts.isIdentifier(declaration.name) || !declaration.initializer) continue;
        if (declaration.name.text === 'RootProvider') {
          rootProviderTags = jsxTags(declaration.initializer, app);
          walk(declaration.initializer, (node) => {
            if (
              ts.isCallExpression(node) &&
              ts.isIdentifier(node.expression) &&
              node.expression.text === 'useEffect'
            ) {
              rootProviderEffectCount += 1;
              const dependencies = node.arguments[1];
              emptyEffectDependencies = Boolean(
                dependencies && ts.isArrayLiteralExpression(dependencies) && dependencies.elements.length === 0
              );
            }
            if (callName(node, 'storage', 'getLanguage')) storedLanguageReads += 1;
            if (
              ts.isIfStatement(node) &&
              node.expression.getText(app) === 'lng'
            ) {
              walk(node.thenStatement, (child) => {
                if (
                  ts.isCallExpression(child) &&
                  ts.isIdentifier(child.expression) &&
                  child.expression.text === 'changeLanguageAsync' &&
                  child.arguments.length === 1 &&
                  child.arguments[0].getText(app) === 'lng'
                ) conditionalLanguageChanges += 1;
              });
            }
          });
        }
        if (declaration.name.text === 'RouterProviderWrapper') {
          routerWrapperTags = jsxTags(declaration.initializer, app);
          walk(declaration.initializer, (node) => {
            if (
              (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) &&
              node.tagName.getText(app) === 'RouterProvider'
            ) routerBinding = jsxAttribute(node, 'router', app);
          });
        }
      }
    }
    if (
      ts.isFunctionDeclaration(statement) &&
      statement.name &&
      statement.name.text === 'AppContainer'
    ) {
      appContainerTags = jsxTags(statement, app);
      walk(statement, (node) => {
        if (
          (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) &&
          node.tagName.getText(app) === 'RouterProviderWrapper'
        ) appContainerRouterBindings.push(jsxAttribute(node, 'router', app));
      });
    }
  }

  return {
    rootProviderTags,
    rootProviderEffectCount,
    emptyEffectDependencies,
    storedLanguageReads,
    conditionalLanguageChanges,
    appContainerTags,
    appContainerRouterBindings,
    routerWrapperTags,
    routerBinding,
  };
}

function omit(value, keys) {
  return Object.fromEntries(Object.entries(value).filter(([key]) => !keys.includes(key)));
}

function inspectServiceFactories(path) {
  const nextCalls = [];
  const nextRequest = (config) => {
    nextCalls.push(config);
    return config;
  };
  const module = {exports: {}};
  const transpiled = ts.transpileModule(fs.readFileSync(path, 'utf8'), {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
      esModuleInterop: true,
    },
    fileName: path,
  });
  const context = {
    module,
    exports: module.exports,
    require(name) {
      if (name === 'lodash') {
        return {isObject: (value) => value !== null && ['object', 'function'].includes(typeof value)};
      }
      if (name === 'lodash/omit') return omit;
      if (name === './next-request') return nextRequest;
      throw new Error(`Unexpected frontend contract import: ${name}`);
    },
  };
  vm.runInNewContext(transpiled.outputText, context, {filename: path, timeout: 5000});

  const legacyCalls = [];
  const legacyRequest = (url, config) => {
    legacyCalls.push({kind: 'request', url, config});
    return config;
  };
  legacyRequest.get = (url, config) => {
    legacyCalls.push({kind: 'get', url, config});
    return config;
  };
  const legacy = module.exports.default(
    {
      create: {url: '/items', method: 'POST'},
      read: {url: '/items', method: 'GET', headers: {Accept: 'application/json'}},
      unsupported: {url: '/items', method: 'OPTIONS'},
    },
    legacyRequest,
  );
  legacy.create({name: 'item'}, 'item-1');
  legacy.read({page: 2}, 'item-1');
  const unsupportedResult = legacy.unsupported();

  const inherited = {inherited: {url: '/inherited', method: 'get'}};
  const registrations = Object.assign(Object.create(inherited), {
    wrapped: {url: '/wrapped', method: 'post'},
    native: {url: '/native', method: 'get'},
    dynamic: {url: (id) => `/items/${id}`, method: 'delete'},
  });
  const next = module.exports.registerNextServer(registrations);
  next.wrapped({name: 'item'});
  next.native({params: {page: 2}, headers: {Accept: 'application/json'}}, true);
  next.dynamic('item-1');

  return {
    exports: Object.keys(module.exports).sort(),
    legacyKeys: Object.keys(legacy),
    legacyCalls,
    unsupportedResult: unsupportedResult === undefined ? 'undefined' : unsupportedResult,
    nextKeys: Object.keys(next),
    nextCalls,
  };
}

process.stdout.write(JSON.stringify({
  main: inspectMain(process.argv[2]),
  app: inspectApp(process.argv[3]),
  services: inspectServiceFactories(process.argv[4]),
}));
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


def _registration_inventory(
    main: Path = MAIN,
    app: Path = APP,
    register_server: Path = REGISTER_SERVER,
) -> dict:
    node = shutil.which("node")
    if node is None:
        raise RuntimeError("Node.js is required to check frontend registrations")
    if not TYPESCRIPT.is_file():
        raise RuntimeError("Install the locked frontend dependencies before checking registrations")
    completed = subprocess.run(
        [node, "-e", _CHECK_FRONTEND_REGISTRATIONS, str(TYPESCRIPT), str(main), str(app), str(register_server)],
        cwd=ROOT,
        capture_output=True,
        text=True,
        timeout=20,
        check=False,
    )
    if completed.returncode:
        raise RuntimeError(f"Frontend registration contract failed: {completed.stderr[-2000:]}")
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


def _assert_frontend_registrations(inventory: dict) -> None:
    main = inventory["main"]
    assert main["telemetryIndex"] >= 0
    assert main["telemetryIndex"] < main["languageIndex"]
    assert main["initThenCount"] == 1
    assert main["renderCount"] == 1
    assert main["rootElementId"] == "root"
    assert main["renderTags"] == ["React.StrictMode", "Inspector", "App"]
    assert main["inspectorKeys"] == "['alt', 'c']"
    assert main["inspectorHandler"] == "gotoVSCode"

    app = inventory["app"]
    assert app["rootProviderTags"] == ["TooltipProvider", "QueryClientProvider", "ThemeProvider", "Root"]
    assert app["rootProviderEffectCount"] == 1
    assert app["emptyEffectDependencies"] is True
    assert app["storedLanguageReads"] == 1
    assert app["conditionalLanguageChanges"] == 1
    assert app["appContainerTags"] == ["RootProvider", "RouterProviderWrapper"]
    assert app["appContainerRouterBindings"] == ["routers"]
    assert app["routerWrapperTags"] == ["RouterProvider"]
    assert app["routerBinding"] == "router"

    services = inventory["services"]
    assert services["exports"] == ["default", "registerNextServer"]
    assert services["legacyKeys"] == ["create", "read", "unsupported"]
    assert services["legacyCalls"] == [
        {"kind": "request", "url": "/items/item-1", "config": {"method": "POST", "data": {"name": "item"}}},
        {
            "kind": "get",
            "url": "/items/item-1",
            "config": {"headers": {"Accept": "application/json"}, "params": {"page": 2}},
        },
    ]
    assert services["unsupportedResult"] == "undefined"
    assert services["nextKeys"] == ["wrapped", "native", "dynamic"]
    assert services["nextCalls"] == [
        {"url": "/wrapped", "method": "post", "data": {"name": "item"}},
        {"url": "/native", "method": "get", "params": {"page": 2}, "headers": {"Accept": "application/json"}},
        {"url": "/items/item-1", "method": "delete", "data": "item-1"},
    ]


def test_owned_frontend_route_entrypoints_are_registered_and_resolvable():
    _assert_owned_entrypoints(_inventory())


def test_frontend_bootstrap_and_service_registrations_are_exact():
    _assert_frontend_registrations(_registration_inventory())


def test_frontend_entrypoint_contract_rejects_changed_lazy_target(tmp_path):
    source = ROUTES.read_text(encoding="utf-8")
    changed = source.replace("import('@/pages/openmetadata')", "import('@/pages/home')", 1)
    assert changed != source
    routes = tmp_path / "routes.tsx"
    routes.write_text(changed, encoding="utf-8")

    with pytest.raises(AssertionError):
        _assert_owned_entrypoints(_inventory(routes=routes))


def test_frontend_registration_contract_rejects_missing_bootstrap_hook(tmp_path):
    source = MAIN.read_text(encoding="utf-8")
    changed = source.replace("installClientTelemetry();", "void 0;", 1)
    assert changed != source
    main = tmp_path / "main.tsx"
    main.write_text(changed, encoding="utf-8")

    with pytest.raises(AssertionError):
        _assert_frontend_registrations(_registration_inventory(main=main))


def test_frontend_registration_contract_rejects_broken_router_handoff(tmp_path):
    source = APP.read_text(encoding="utf-8")
    changed = source.replace(
        "<RouterProviderWrapper router={routers} />",
        "<RouterProviderWrapper router={undefined as any} />",
        1,
    )
    assert changed != source
    app = tmp_path / "app.tsx"
    app.write_text(changed, encoding="utf-8")

    with pytest.raises(AssertionError):
        _assert_frontend_registrations(_registration_inventory(app=app))


def test_frontend_registration_contract_rejects_inherited_service_registration(tmp_path):
    source = REGISTER_SERVER.read_text(encoding="utf-8")
    changed = source.replace("Object.prototype.hasOwnProperty.call(requestRecord, name)", "true", 1)
    assert changed != source
    register_server = tmp_path / "register-server.ts"
    register_server.write_text(changed, encoding="utf-8")

    with pytest.raises(AssertionError):
        _assert_frontend_registrations(_registration_inventory(register_server=register_server))
