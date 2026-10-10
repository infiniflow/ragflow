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

// jsdom does not expose web streams; eventsource-parser reads TransformStream
// at module scope (via hooks/logic-hooks.ts)
if (typeof globalThis.TransformStream === 'undefined') {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const { TransformStream } = require('node:stream/web');
  Object.assign(globalThis, { TransformStream });
}

// jsdom does not expose the Fetch API globals; react-router's createBrowserRouter
// builds a `new Request(...)` at module scope (via routes.tsx -> route-hook)
if (typeof (globalThis as Record<string, unknown>).Request === 'undefined') {
  class MinimalRequest {
    url: string;
    method: string;
    headers: Record<string, string>;
    signal: unknown;
    constructor(input: unknown, init: { method?: string; headers?: Record<string, string>; signal?: unknown } = {}) {
      this.url = String(input);
      this.method = init.method ?? 'GET';
      this.headers = init.headers ?? {};
      this.signal = init.signal;
    }
    clone() {
      return new MinimalRequest(this.url, { method: this.method, headers: this.headers, signal: this.signal });
    }
    text() {
      return Promise.resolve('');
    }
    json() {
      return Promise.resolve(null);
    }
  }
  (globalThis as Record<string, unknown>).Request = MinimalRequest;
}

// jsdom does not implement CSS.supports; css-support.ts calls it at module scope
if (typeof globalThis.CSS === 'undefined') {
  (globalThis as Record<string, unknown>).CSS = {};
}
if (typeof globalThis.CSS.supports !== 'function') {
  globalThis.CSS.supports = () => false;
}

// Vite's import.meta.glob is rewritten to this stub by jest-esbuild-transformer.cjs
(globalThis as Record<string, unknown>).jestImportMetaGlob = () => ({});

// jsdom's Blob does not implement arrayBuffer(); file-util.ts calls
// `new Blob([data]).arrayBuffer()` (txt/csv previews). Polyfill it via FileReader.
if (
  typeof globalThis.Blob !== 'undefined' &&
  typeof globalThis.Blob.prototype.arrayBuffer !== 'function'
) {
  globalThis.Blob.prototype.arrayBuffer = function (this: Blob) {
    return new Promise<ArrayBuffer>((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result as ArrayBuffer);
      reader.onerror = () => reject(reader.error);
      reader.readAsArrayBuffer(this);
    });
  };
}

// jsdom does not expose fetch; some modules call it at import time and
// handle the rejection themselves (e.g. utils/backend-runtime.ts)
if (typeof globalThis.fetch === 'undefined') {
  (globalThis as Record<string, unknown>).fetch = () =>
    Promise.reject(new Error('fetch is not available in tests'));
}
