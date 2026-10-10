// Jest transformer wrapping esbuild with the same options esbuild-jest was
// configured with, plus `define: { 'import.meta.env': '{}' }` — esbuild-jest@0.5
// does not forward esbuild's `define` option, and source files read Vite's
// import.meta.env at module scope, which crashes under jest's cjs runtime.
// Files containing jest.mock still go through esbuild-jest for its babel-based
// mock hoisting.
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
        const ext = path.extname(filename).slice(1);
        const loader =
          ext === 'ts' ? 'tsx' : supportedLoaders.includes(ext) ? ext : 'text';
        if (content.indexOf('ock(') >= 0) {
          // esbuild-jest's babel path only PARSES TypeScript (it adds a
          // `typescript` parser plugin but no types-stripping preset), so a
          // type-only import or annotation throws before its own esbuild step:
          //   "Cannot transform the imported binding ... used in a type annotation".
          // Pre-strip the TS with esbuild so babel receives plain JS (jest.mock
          // hoisting still runs there), then let esbuild-jest transform it.
          // `jsx: 'preserve'` is essential: leaving JSX intact keeps `React` out of
          // the transformed source, so babel's mock hoisting does not hoist a
          // factory above the `React` import and reject it as out-of-scope.
          const stripped = esbuild.transformSync(content, {
            loader,
            format: 'esm',
            target: 'es2018',
            jsx: 'preserve',
            sourcemap: false,
            define: {
              'import.meta.env': '{}',
              'import.meta.glob': 'jestImportMetaGlob',
            },
          }).code;
          return esbuildJestTransformer.process(
            stripped,
            filename,
            config,
            opts,
          );
        }
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
