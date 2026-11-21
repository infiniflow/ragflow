// Jest transformer wrapping esbuild with the same options esbuild-jest was
// configured with, plus `define: { 'import.meta.env': '{}' }` — esbuild-jest@0.5
// does not forward esbuild's `define` option, and source files read Vite's
// import.meta.env at module scope, which crashes under jest's cjs runtime.
// Files containing a mock still go through esbuild-jest for its babel-based mock
// hoisting. The marker is `mock(`, not `ock(`: the latter also matches ordinary
// code such as `ActionContractBlock(`, which then gets the babel pass and fails
// on any imported type used in an annotation.
const path = require('node:path');
const esbuild = require('esbuild');
const esbuildJest = require('esbuild-jest');

const esbuildJestTransformer = esbuildJest.createTransformer({
  sourcemap: true,
  loaders: { '.ts': 'tsx' },
});

const supportedLoaders = ['js', 'jsx', 'ts', 'tsx', 'json'];

module.exports = {
  createTransformer() {
    return {
      process(content, filename, config, opts) {
        if (content.indexOf('mock(') >= 0) {
          return esbuildJestTransformer.process(
            content,
            filename,
            config,
            opts,
          );
        }
        const ext = path.extname(filename).slice(1);
        const loader =
          ext === 'ts' ? 'tsx' : supportedLoaders.includes(ext) ? ext : 'text';
        const result = esbuild.transformSync(content, {
          loader,
          format: 'cjs',
          target: 'es2018',
          sourcemap: true,
          sourcesContent: false,
          sourcefile: filename,
          define: {
            'import.meta.env': '{}',
            // Vite's glob import; jestImportMetaGlob is set in jest-setup.ts
            'import.meta.glob': 'jestImportMetaGlob',
          },
        });
        return { code: result.code, map: JSON.parse(result.map) };
      },
    };
  },
};
