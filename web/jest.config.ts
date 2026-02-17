import { Config, configUmiAlias, createConfig } from 'umi/test';

export default async () => {
  return (await configUmiAlias({
    ...createConfig({
      target: 'browser',
      jsTransformer: 'esbuild',
      // config opts for esbuild , it will pass to esbuild directly
      jsTransformerOpts: { jsx: 'automatic' },
    }),
    setupFilesAfterEnv: ['<rootDir>/jest-setup.ts'],
    collectCoverageFrom: [
      '**/*.{ts,tsx,js,jsx}',
      '!.umi/**',
      '!.umi-test/**',
      '!.umi-production/**',
      '!.umirc.{js,ts}',
      '!.umirc.*.{js,ts}',
      '!jest.config.{js,ts}',
      '!coverage/**',
      '!dist/**',
      '!config/**',
      '!mock/**',
    ],
    // if you require some es-module npm package, please uncomment below line and insert your package name
    // transformIgnorePatterns: ['node_modules/(?!.*(lodash-es|your-es-pkg-name)/)']
    coverageThreshold: {
      global: {
        lines: 1,
      },
    },
  })) as Config.InitialOptions;
};
