const path = require('node:path');
const fs = require('node:fs');
const crypto = require('node:crypto');
const esbuild = require('esbuild');
const esbuildJest = require('esbuild-jest');

const esbuildJestTransformer = esbuildJest.createTransformer({
  sourcemap: true,
  loaders: { '.ts': 'tsx' },
  jsxFactory: 'React.createElement',
});

const supportedLoaders = ['js', 'jsx', 'ts', 'tsx', 'json'];
// Hash once per process, not once per transformed module. Both install profiles
// remain supported; a lockfile or transformer change invalidates cached output.
const implementationKey = crypto.createHash('sha256');
for (const file of [
  __filename,
  ...['package-lock.json', 'pnpm-lock.yaml'].map((name) =>
    path.join(__dirname, name),
  ),
]) {
  implementationKey.update(file);
  implementationKey.update(
    fs.existsSync(file) ? fs.readFileSync(file) : '<absent>',
  );
}
implementationKey.update(esbuild.version);
implementationKey.update(require('esbuild-jest/package.json').version);
const implementationDigest = implementationKey.digest('hex');

module.exports = {
  createTransformer() {
    return {
      getCacheKey(content, filename, options) {
        return crypto
          .createHash('sha256')
          .update(
            JSON.stringify([
              implementationDigest,
              content,
              filename,
              options.configString,
              options.instrument,
              options.supportsStaticESM,
              options.supportsDynamicImport,
              options.supportsExportNamespaceFrom,
              options.supportsTopLevelAwait,
              options.transformerConfig,
            ]),
          )
          .digest('hex');
      },
      process(content, filename, config, opts) {
        const normalizedContent = content
          .replace(/\bimport\.meta\.env\b/g, '({})')
          .replace(/\bimport\.meta\.glob\b/g, 'jestImportMetaGlob');

        if (normalizedContent.includes('ock(')) {
          return esbuildJestTransformer.process(
            normalizedContent,
            filename,
            config,
            opts,
          );
        }

        const extension = path.extname(filename).slice(1);
        const loader =
          extension === 'ts'
            ? 'tsx'
            : supportedLoaders.includes(extension)
              ? extension
              : 'text';
        const result = esbuild.transformSync(normalizedContent, {
          loader,
          format: 'cjs',
          target: 'es2018',
          sourcemap: true,
          sourcesContent: false,
          sourcefile: filename,
          jsxFactory: 'React.createElement',
        });

        return { code: result.code, map: JSON.parse(result.map) };
      },
    };
  },
};
