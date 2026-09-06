// Real Jest process replay: source, transformer and config must invalidate cache.
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const repo = path.resolve(__dirname, "../../../..");
const web = path.join(repo, "web");
const root = fs.mkdtempSync(path.join(os.tmpdir(), "ragflow-jest-cache-"));
const original = fs.readFileSync(
  path.join(web, "jest-esbuild-transformer.cjs"),
  "utf8",
);
const results = [];
function run(name, expectedExit) {
  const started = performance.now();
  const result = spawnSync(
    process.execPath,
    [
      path.join(web, "node_modules/jest/bin/jest.js"),
      "--config",
      path.join(root, "jest.config.json"),
      "--runInBand",
      "--no-coverage",
    ],
    {
      cwd: root,
      encoding: "utf8",
      timeout: 60000,
      env: { ...process.env, NODE_PATH: path.join(web, "node_modules") },
    },
  );
  results.push({
    name,
    seconds: (performance.now() - started) / 1000,
    exit: result.status,
  });
  assert.equal(result.status, expectedExit, result.stdout + result.stderr);
}
try {
  fs.writeFileSync(path.join(root, "transformer.cjs"), original);
  const config = {
    rootDir: root,
    cacheDirectory: path.join(root, "cache"),
    testEnvironment: "node",
    transform: { "^.+\\.ts$": "<rootDir>/transformer.cjs" },
    globals: { expected: "one" },
  };
  fs.writeFileSync(path.join(root, "jest.config.json"), JSON.stringify(config));
  fs.writeFileSync(path.join(root, "value.ts"), "export const value = 'one';");
  fs.writeFileSync(
    path.join(root, "value.test.ts"),
    "import { value } from './value'; test('value', () => expect(value).toBe(globalThis.expected));",
  );
  run("empty-cache", 0);
  run("warm-cache", 0);
  fs.writeFileSync(path.join(root, "value.ts"), "export const value = 'two';");
  run("source-changed-must-fail", 1);
  config.globals.expected = "two";
  fs.writeFileSync(path.join(root, "jest.config.json"), JSON.stringify(config));
  run("config-changed-must-pass", 0);
  fs.writeFileSync(
    path.join(root, "transformer.cjs"),
    original.replace(
      "const normalizedContent = content",
      "const normalizedContent = content.replaceAll('two', 'three')",
    ),
  );
  run("transformer-changed-must-fail", 1);
  console.log(JSON.stringify(results, null, 2));
} finally {
  // Only this process's mkdtemp fixture, never repository files or shared cache.
  assert.ok(fs.realpathSync(root).startsWith(path.join(fs.realpathSync(os.tmpdir()), 'ragflow-jest-cache-')));
  fs.rmSync(root, { recursive: true, force: true });
}
