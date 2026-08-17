import '@testing-library/jest-dom';
import React from 'react';

// esbuild-jest compiles JSX with the classic runtime (React.createElement),
// while source files rely on the automatic runtime and never import React.
// Expose React globally so rendering components in tests works.
(globalThis as Record<string, unknown>).React = React;

// jsdom does not provide these, but react-router reads them at module scope
if (typeof globalThis.TextEncoder === 'undefined') {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const { TextDecoder, TextEncoder } = require('node:util');
  Object.assign(globalThis, { TextDecoder, TextEncoder });
}

// Vite's import.meta.glob is rewritten to this stub by jest-esbuild-transformer.cjs
(globalThis as Record<string, unknown>).jestImportMetaGlob = () => ({});

// jsdom does not expose fetch; some modules call it at import time and
// handle the rejection themselves (e.g. utils/backend-runtime.ts)
if (typeof globalThis.fetch === 'undefined') {
  (globalThis as Record<string, unknown>).fetch = () =>
    Promise.reject(new Error('fetch is not available in tests'));
}
