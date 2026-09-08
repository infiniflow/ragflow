"use strict";

const fs = require("fs");
const path = require("path");

function fail(message) {
  process.stderr.write(`INCOMPLETE: ${message}\n`);
  process.exit(2);
}

function posix(value) {
  return value.replace(/\\/g, "/");
}

function absoluteKey(value) {
  const normalized = posix(path.resolve(value));
  return process.platform === "win32" ? normalized.toLowerCase() : normalized;
}

function repoPath(root, value) {
  const relative = posix(path.relative(root, value));
  if (relative === ".." || relative.startsWith("../") || path.isAbsolute(relative)) return null;
  return relative;
}

function readInput() {
  try {
    return JSON.parse(fs.readFileSync(0, "utf8"));
  } catch (error) {
    fail(`invalid worker input: ${error.message}`);
  }
}

function loadTypeScript(modulePath) {
  try {
    return require(modulePath);
  } catch (error) {
    fail(`cannot load TypeScript parser: ${error.message}`);
  }
}

const input = readInput();
const root = path.resolve(input.root);
const typescriptPath = path.resolve(root, input.typescript_module);
const ts = loadTypeScript(typescriptPath);
const configuredFiles = [...new Set(input.files)].sort();
const fileByKey = new Map();
for (const relative of configuredFiles) {
  const absolute = path.resolve(root, relative);
  if (repoPath(root, absolute) !== posix(relative)) fail(`source escapes repository: ${relative}`);
  fileByKey.set(absoluteKey(absolute), posix(relative));
}

const configurationErrors = [];
let compilerOptions = {
  moduleResolution: ts.ModuleResolutionKind.Bundler,
  allowImportingTsExtensions: true,
  resolveJsonModule: true,
  jsx: ts.JsxEmit.ReactJSX,
};
const tsconfigPath = path.resolve(root, input.tsconfig);
const configFile = ts.readConfigFile(tsconfigPath, ts.sys.readFile);
if (configFile.error) {
  configurationErrors.push(ts.flattenDiagnosticMessageText(configFile.error.messageText, "\n"));
} else {
  const parsed = ts.parseJsonConfigFileContent(configFile.config, ts.sys, path.dirname(tsconfigPath));
  compilerOptions = parsed.options;
  for (const error of parsed.errors) configurationErrors.push(ts.flattenDiagnosticMessageText(error.messageText, "\n"));
}

const aliases = Object.entries(input.aliases || {}).sort(([left], [right]) => right.length - left.length);
const codeExtensions = new Set([".ts", ".tsx", ".mts", ".cts", ".js", ".jsx"]);
const candidateExtensions = [
  "",
  ".ts",
  ".tsx",
  ".mts",
  ".cts",
  ".js",
  ".jsx",
  ".json",
  ".css",
  ".less",
  ".scss",
  ".sass",
  ".svg",
  ".png",
  ".jpg",
  ".jpeg",
  ".gif",
  ".webp",
  ".md",
  ".mdx",
  ".wasm",
];

function existingFile(value) {
  try {
    return fs.statSync(value).isFile();
  } catch {
    return false;
  }
}

function manualBase(specifier, containingFile) {
  if (specifier.startsWith(".")) return path.resolve(path.dirname(containingFile), specifier);
  for (const [alias, target] of aliases) {
    if (specifier === alias || specifier.startsWith(`${alias}/`)) {
      const suffix = specifier === alias ? "" : specifier.slice(alias.length + 1);
      return path.resolve(root, target, suffix);
    }
  }
  return null;
}

function manualCandidate(specifier, containingFile) {
  const base = manualBase(specifier, containingFile);
  if (!base) return null;
  for (const extension of candidateExtensions) {
    const candidate = `${base}${extension}`;
    if (existingFile(candidate)) return candidate;
  }
  for (const extension of candidateExtensions.slice(1)) {
    const candidate = path.join(base, `index${extension}`);
    if (existingFile(candidate)) return candidate;
  }
  return null;
}

function resolveSpecifier(specifier, containingFile) {
  const clean = specifier.replace(/[?#].*$/, "");
  const resolved = ts.resolveModuleName(clean, containingFile, compilerOptions, ts.sys).resolvedModule;
  const candidate = resolved ? resolved.resolvedFileName : manualCandidate(clean, containingFile);
  if (candidate) {
    const key = absoluteKey(candidate);
    if (fileByKey.has(key)) return {target: fileByKey.get(key), resolution: "local"};
    const relative = repoPath(root, candidate);
    if (relative) {
      if (relative.split("/").includes("node_modules")) return {target: clean, resolution: "external"};
      const extension = path.extname(candidate).toLowerCase();
      return {
        target: relative,
        resolution: codeExtensions.has(extension) ? "local_outside_profile" : "asset",
      };
    }
    return {target: clean, resolution: "external"};
  }
  if (manualBase(clean, containingFile)) return {target: clean, resolution: "unresolved_local"};
  return {target: clean, resolution: "external"};
}

function lineOf(source, node) {
  return source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;
}

function hasModifier(node, kind) {
  return Boolean(node.modifiers && node.modifiers.some((modifier) => modifier.kind === kind));
}

function bindingNames(name, output = []) {
  if (ts.isIdentifier(name)) output.push(name.text);
  else for (const element of name.elements || []) if (!ts.isOmittedExpression(element)) bindingNames(element.name, output);
  return output;
}

function importGroups(clause) {
  if (!clause) return [{typeOnly: false, symbols: []}];
  if (clause.isTypeOnly) {
    const symbols = [];
    if (clause.name) symbols.push("default");
    if (clause.namedBindings) {
      if (ts.isNamespaceImport(clause.namedBindings)) symbols.push("*");
      else for (const element of clause.namedBindings.elements) symbols.push((element.propertyName || element.name).text);
    }
    return [{typeOnly: true, symbols}];
  }
  const runtime = [];
  const types = [];
  if (clause.name) runtime.push("default");
  if (clause.namedBindings) {
    if (ts.isNamespaceImport(clause.namedBindings)) runtime.push("*");
    else {
      for (const element of clause.namedBindings.elements) {
        (element.isTypeOnly ? types : runtime).push((element.propertyName || element.name).text);
      }
    }
  }
  const groups = [];
  if (runtime.length || !types.length) groups.push({typeOnly: false, symbols: runtime});
  if (types.length) groups.push({typeOnly: true, symbols: types});
  return groups;
}

function exportGroups(node) {
  if (!node.exportClause || !ts.isNamedExports(node.exportClause)) {
    return [{typeOnly: Boolean(node.isTypeOnly), symbols: ["*"]}];
  }
  if (node.isTypeOnly) {
    return [{typeOnly: true, symbols: node.exportClause.elements.map((element) => (element.propertyName || element.name).text)}];
  }
  const runtime = [];
  const types = [];
  for (const element of node.exportClause.elements) {
    (element.isTypeOnly ? types : runtime).push((element.propertyName || element.name).text);
  }
  const groups = [];
  if (runtime.length) groups.push({typeOnly: false, symbols: runtime});
  if (types.length) groups.push({typeOnly: true, symbols: types});
  return groups;
}

function isFunctionBoundary(node) {
  return ts.isFunctionDeclaration(node) ||
    ts.isFunctionExpression(node) ||
    ts.isArrowFunction(node) ||
    ts.isMethodDeclaration(node) ||
    ts.isGetAccessorDeclaration(node) ||
    ts.isSetAccessorDeclaration(node) ||
    ts.isConstructorDeclaration(node);
}

function isConditionalBoundary(node) {
  return ts.isIfStatement(node) ||
    ts.isConditionalExpression(node) ||
    ts.isSwitchStatement(node) ||
    ts.isTryStatement(node) ||
    ts.isForStatement(node) ||
    ts.isForInStatement(node) ||
    ts.isForOfStatement(node) ||
    ts.isWhileStatement(node) ||
    ts.isDoStatement(node);
}

function importMetaRegistration(node, source) {
  if (!ts.isCallExpression(node)) return null;
  const expression = node.expression.getText(source);
  if (!["import.meta.glob", "import.meta.globEager", "require.context"].includes(expression)) return null;
  const patterns = [];
  let computed = false;
  const first = node.arguments[0];
  const items = first && ts.isArrayLiteralExpression(first) ? first.elements : first ? [first] : [];
  for (const item of items) {
    if (ts.isStringLiteralLike(item)) patterns.push(item.text);
    else computed = true;
  }
  if (!items.length) computed = true;
  return {kind: expression, patterns, computed};
}

function inspectFile(relative) {
  const absolute = path.resolve(root, relative);
  const text = fs.readFileSync(absolute, "utf8");
  const kind = relative.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS;
  const source = ts.createSourceFile(absolute, text, ts.ScriptTarget.Latest, true, kind);
  const edges = [];
  const issues = [];
  const registrations = [];
  const exports = new Set();

  function issue(node, kindName, detail, typeOnly = false) {
    issues.push({source: relative, line: lineOf(source, node), kind: kindName, detail, type_only: typeOnly});
  }

  function edge(node, specifier, kindName, typeOnly, symbols, state) {
    const resolution = resolveSpecifier(specifier, absolute);
    const record = {
      source: relative,
      target: resolution.target,
      specifier,
      line: lineOf(source, node),
      kind: kindName,
      phase: state.phase,
      conditional: state.conditional,
      type_only: typeOnly,
      resolution: resolution.resolution,
      symbols: [...new Set(symbols)].sort(),
    };
    edges.push(record);
    if (resolution.resolution === "unresolved_local" || resolution.resolution === "local_outside_profile") {
      issue(node, resolution.resolution, specifier, typeOnly);
    }
  }

  function visit(node, state) {
    if (ts.isImportDeclaration(node) && ts.isStringLiteralLike(node.moduleSpecifier)) {
      for (const group of importGroups(node.importClause)) {
        edge(node, node.moduleSpecifier.text, "import", group.typeOnly, group.symbols, state);
      }
    } else if (ts.isExportDeclaration(node)) {
      if (node.moduleSpecifier && ts.isStringLiteralLike(node.moduleSpecifier)) {
        for (const group of exportGroups(node)) {
          edge(node, node.moduleSpecifier.text, "re_export", group.typeOnly, group.symbols, state);
        }
      } else if (node.exportClause && ts.isNamedExports(node.exportClause)) {
        for (const element of node.exportClause.elements) exports.add(element.name.text);
      }
    } else if (ts.isImportEqualsDeclaration(node) && ts.isExternalModuleReference(node.moduleReference)) {
      const expression = node.moduleReference.expression;
      if (expression && ts.isStringLiteralLike(expression)) edge(node, expression.text, "import_equals", false, ["*"], state);
    } else if (ts.isImportTypeNode(node) && ts.isLiteralTypeNode(node.argument) && ts.isStringLiteralLike(node.argument.literal)) {
      edge(node, node.argument.literal.text, "import_type", true, ["*"], state);
    } else if (ts.isCallExpression(node)) {
      const registration = importMetaRegistration(node, source);
      if (registration) {
        registrations.push({source: relative, line: lineOf(source, node), phase: state.phase, conditional: state.conditional, ...registration});
        if (registration.computed) issue(node, "computed_dynamic_registration", registration.kind);
      } else if (node.expression.kind === ts.SyntaxKind.ImportKeyword) {
        const target = node.arguments[0];
        if (target && ts.isStringLiteralLike(target)) edge(node, target.text, "dynamic_import", false, [], state);
        else issue(node, "computed_dynamic_import", target ? target.getText(source) : "missing target");
      } else if (ts.isIdentifier(node.expression) && node.expression.text === "require") {
        const target = node.arguments[0];
        if (target && ts.isStringLiteralLike(target)) edge(node, target.text, "require", false, ["*"], state);
        else issue(node, "computed_require", target ? target.getText(source) : "missing target");
      } else if (ts.isIdentifier(node.expression) && node.expression.text === "eval") {
        issue(node, "dynamic_code", "eval");
      }
    } else if (ts.isNewExpression(node) && ts.isIdentifier(node.expression) && node.expression.text === "Function") {
      issue(node, "dynamic_code", "Function constructor");
    }

    if (
      (ts.isFunctionDeclaration(node) || ts.isClassDeclaration(node) || ts.isInterfaceDeclaration(node) || ts.isTypeAliasDeclaration(node) || ts.isEnumDeclaration(node)) &&
      node.name &&
      hasModifier(node, ts.SyntaxKind.ExportKeyword)
    ) exports.add(hasModifier(node, ts.SyntaxKind.DefaultKeyword) ? "default" : node.name.text);
    if (ts.isVariableStatement(node) && hasModifier(node, ts.SyntaxKind.ExportKeyword)) {
      for (const declaration of node.declarationList.declarations) {
        for (const name of bindingNames(declaration.name)) exports.add(name);
      }
    }
    if (ts.isExportAssignment(node)) exports.add(node.isExportEquals ? "export=" : "default");

    const childState = {
      phase: isFunctionBoundary(node) ? "call" : state.phase,
      conditional: state.conditional || isConditionalBoundary(node),
    };
    ts.forEachChild(node, (child) => visit(child, childState));
  }

  visit(source, {phase: "module", conditional: false});
  for (const diagnostic of source.parseDiagnostics) {
    const start = diagnostic.start || 0;
    const line = source.getLineAndCharacterOfPosition(start).line + 1;
    issues.push({
      source: relative,
      line,
      kind: "parse_error",
      detail: ts.flattenDiagnosticMessageText(diagnostic.messageText, "\n"),
      type_only: false,
    });
  }
  return {
    node: {
      path: relative,
      is_test: input.test_markers.some((marker) => relative.includes(marker)),
      is_declaration: relative.endsWith(".d.ts"),
      exports: [...exports].sort(),
    },
    edges,
    issues,
    registrations,
  };
}

try {
  const nodes = [];
  const edges = [];
  const issues = [];
  const dynamicRegistrations = [];
  for (const relative of configuredFiles) {
    const result = inspectFile(relative);
    nodes.push(result.node);
    edges.push(...result.edges);
    issues.push(...result.issues);
    dynamicRegistrations.push(...result.registrations);
  }
  process.stdout.write(JSON.stringify({
    compiler: {version: ts.version, configuration_errors: configurationErrors},
    nodes,
    edges,
    issues,
    dynamic_registrations: dynamicRegistrations,
  }));
} catch (error) {
  fail(error && error.stack ? error.stack : String(error));
}
