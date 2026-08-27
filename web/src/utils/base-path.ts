/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

/** Vite `base` value, e.g. `/ragflow/`. */
export function normalizeViteBasePath(base?: string): string {
  if (!base || base === '/') {
    return '/';
  }
  const trimmed = base.trim();
  if (!trimmed) {
    return '/';
  }
  const withLeading = trimmed.startsWith('/') ? trimmed : `/${trimmed}`;
  return withLeading.endsWith('/') ? withLeading : `${withLeading}/`;
}

/** React Router basename, e.g. `/ragflow` (no trailing slash). */
export function getRouterBasename(base?: string): string {
  const viteBase = normalizeViteBasePath(base ?? import.meta.env.VITE_BASE_URL);
  if (viteBase === '/') {
    return '/';
  }
  return viteBase.slice(0, -1);
}

/** Prefix an absolute app path with the configured base path. */
export function withAppBasePath(path: string): string {
  if (!path.startsWith('/')) {
    path = `/${path}`;
  }
  const basename = getRouterBasename();
  if (basename === '/') {
    return path;
  }
  return `${basename}${path}`;
}

export const APP_BASE_PATH = getRouterBasename();
