import type { Config } from 'jest';

// The Markdown renderer uses ESM-only Unified packages. Transform those
// packages in Jest's CommonJS runtime so the real rendering pipeline is testable.
const markdownDependencies = [
  '@ungap/structured-clone',
  'bail|ccount|devlop|trough|zwitch',
  'character-(?:entities[^/]*|reference-invalid)',
  'decode-named-character-reference|entities|parse-entities|stringify-entities',
  '(?:comma|space)-separated-tokens|trim-lines|longest-streak',
  'escape-string-regexp|estree-util-is-identifier-name|is-(?:alphanumerical?|decimal|hexadecimal|plain-obj)',
  'hast-util-[^/]+|hastscript|html-(?:url-attributes|void-elements)|property-information|web-namespaces',
  'markdown-table|mdast-util-[^/]+|micromark(?:-[^/]+)?',
  'parse5|react-markdown|rehype-[^/]+|remark-[^/]+|unified|unist-util-[^/]+|vfile(?:-[^/]+)?',
].join('|');

const config: Config = {
  testEnvironment: 'jsdom',
  transform: {
    // Local wrapper around esbuild-jest that also defines import.meta.env;
    // see jest-esbuild-transformer.cjs
    '^.+\\.(ts|tsx|js|jsx)$': '<rootDir>/jest-esbuild-transformer.cjs',
  },
  transformIgnorePatterns: [
    `/node_modules/(?!(?:${markdownDependencies})/)`,
    '\\.pnp\\.[^\\/]+$',
  ],
  moduleNameMapper: {
    // Asset/style stubs must precede the `@/` alias — the alias rewrites
    // `@/assets/x.png` to a real path and only the first matching mapper runs.
    '\\.(css|less|scss|sass)$': '<rootDir>/__mocks__/styleMock.js',
    '\\.(jpg|jpeg|png|gif|svg|webp)(\\?raw)?$':
      '<rootDir>/__mocks__/fileMock.js',
    // Drags the app shell (routes/react-router) into jsdom; see __mocks__
    '^@/components/layout-recognize-form-field$':
      '<rootDir>/__mocks__/layout-recognize-form-field.js',
    // The app shell itself: createBrowserRouter runs at module scope and
    // needs the fetch Request global, which jsdom does not provide.
    '^@/routes$': '<rootDir>/__mocks__/routes.js',
    // Keep the OSS no-op mapping ahead of the generic `@/` mapping.
    '^@/ee/(.*)$': '<rootDir>/src/ee-stub/$1',
    '^@/(.*)$': '<rootDir>/src/$1',
    '^human-id$': '<rootDir>/__mocks__/human-id.js',
  },
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
