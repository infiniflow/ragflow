/**
 * Shared helper for the Vite-injected `API_PROXY_SCHEME` runtime value.
 *
 * Vite (web/vite.config.ts) injects `__API_PROXY_SCHEME__` from
 * `import.meta.env.API_PROXY_SCHEME`. It selects which backend serves the
 * dataset compilation / index APIs:
 *
 *   - `python`: all /api requests go to the Python backend (9380); the legacy
 *     `/datasets/:id/index` run/trace endpoints remain valid.
 *   - `go`:     all /api requests go to the Go backend (9384); the legacy
 *     `/datasets/:id/index` routes are removed, so UI must use the scheduler
 *     compile-status contract instead.
 *   - `hybrid`: Vite routes `/api/v1/datasets` to the Go backend (9384);
 *     dataset compilation logic behaves like `go`.
 *
 * Default is `python` for safety (matches Vite's fallback).
 *
 * All pages must read the scheme through these predicates instead of scattering
 * `typeof __API_PROXY_SCHEME__` checks (see files-table.tsx, which previously
 * inlined this logic).
 */

declare const __API_PROXY_SCHEME__: string;

export type ApiProxyScheme = 'python' | 'go' | 'hybrid';

export function getApiProxyScheme(): ApiProxyScheme {
  const raw = typeof __API_PROXY_SCHEME__ !== 'undefined' ? __API_PROXY_SCHEME__ : '';
  switch (raw) {
    case 'go':
    case 'hybrid':
    case 'python':
      return raw;
    default:
      return 'python';
  }
}

/** True when the dataset compilation APIs are served by the Go backend. */
export function isGoDatasetBackend(): boolean {
  const s = getApiProxyScheme();
  return s === 'go' || s === 'hybrid';
}

/** True when the legacy Python run/trace `/index` endpoints are in use. */
export function isPythonDatasetBackend(): boolean {
  return getApiProxyScheme() === 'python';
}
