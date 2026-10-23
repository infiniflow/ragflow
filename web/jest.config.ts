import type { Config } from 'jest';

const config: Config = {
  testEnvironment: 'jsdom',
  transform: {
    // Local wrapper around esbuild-jest that also defines import.meta.env;
    // see jest-esbuild-transformer.cjs
    '^.+\\.(ts|tsx|js|jsx)$': '<rootDir>/jest-esbuild-transformer.cjs',
  },
  moduleNameMapper: {
    // Asset/style stubs must precede the `@/` alias — the alias rewrites
    // `@/assets/x.png` to a real path and only the first matching mapper runs.
    '\\.(css|less|scss|sass)$': '<rootDir>/__mocks__/styleMock.js',
    '\\.(jpg|jpeg|png|gif|svg|webp)$': '<rootDir>/__mocks__/fileMock.js',
    // Drags the app shell (routes/react-router) into jsdom; see __mocks__
    '^@/components/layout-recognize-form-field$':
      '<rootDir>/__mocks__/layout-recognize-form-field.js',
    // The app shell itself: createBrowserRouter runs at module scope and
    // needs the fetch Request global, which jsdom does not provide.
    '^@/routes$': '<rootDir>/__mocks__/routes.js',
    '^@/(.*)$': '<rootDir>/src/$1',
    '^human-id$': '<rootDir>/__mocks__/human-id.js',
  },
  // d3-force and its d3-* dependencies ship as ESM only. The ontology layout
  // imports them at module scope, so without this jest cannot even parse the
  // module under test — the failure looks like a syntax error in the test file.
  transformIgnorePatterns: [
    '/node_modules/(?!(d3-force|d3-dispatch|d3-quadtree|d3-timer)/)',
  ],
  setupFilesAfterEnv: ['<rootDir>/jest-setup.ts'],
  collectCoverageFrom: [
    'src/**/*.{ts,tsx,js,jsx}',
    '!src/.umi/**',
    '!src/.umi-test/**',
    '!src/.umi-production/**',
    '!**/*.d.ts',
    '!coverage/**',
    '!dist/**',
    '!config/**',
    '!mock/**',
  ],
  coverageThreshold: {
    global: {
      lines: 1,
    },
  },
  testPathIgnorePatterns: ['/node_modules/', '/dist/'],
};

export default config;
