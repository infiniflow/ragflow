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

// jsdom exposes a CSS object without `supports`; modules read it at module
// scope for feature detection, so return false (feature off) instead of
// blowing up during import.
if (typeof (globalThis as Record<string, unknown>).CSS === 'undefined') {
  (globalThis as Record<string, unknown>).CSS = {};
}
const cssGlobal = (globalThis as Record<string, unknown>).CSS as Record<
  string,
  unknown
>;
if (typeof cssGlobal.supports !== 'function') {
  cssGlobal.supports = () => false;
}

// Radix primitives (checkbox, select, popper) observe element size at mount;
// jsdom has no ResizeObserver, so stub the observing API.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
if (typeof globalThis.ResizeObserver === 'undefined') {
  (globalThis as Record<string, unknown>).ResizeObserver = ResizeObserverStub;
}

// jsdom does not expose fetch; some modules call it at import time and
// handle the rejection themselves (e.g. utils/backend-runtime.ts)
if (typeof globalThis.fetch === 'undefined') {
  (globalThis as Record<string, unknown>).fetch = () =>
    Promise.reject(new Error('fetch is not available in tests'));
}
