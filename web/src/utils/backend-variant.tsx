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

/**
 * Backend variant dispatch primitives.
 *
 * This module is the ONLY supported way to branch UI/logic on the backend
 * language (Go vs Python). It wraps the raw store in
 * `@/utils/backend-runtime`, which no other file may import (enforced by
 * oxlint `no-restricted-imports`); the only other exception is the bootstrap
 * gate in `main.tsx`.
 *
 * Conventions (see web/CLAUDE.md):
 * - JSX divergence: dispatch through `<BackendVariant go={...} python={...} />`
 *   at a dispatcher file (page `index.tsx` / form dispatcher), never inline
 *   in business components.
 * - Value divergence (field names, defaults, payload transforms): select
 *   through `pickByBackend({ go, python })` inside an adapter file.
 * - Hook-level needs (e.g. a query `enabled` flag): `useIsGoBackend()`.
 *
 * The app render is gated on the language fetch in `main.tsx`, so below the
 * gate the variant never changes for the session lifetime and both sync
 * reads (`pickByBackend`) and hook reads are race-free.
 */

// oxlint-disable-next-line no-restricted-imports -- sanctioned boundary over the raw store
import {
  getBackendLanguage,
  subscribeBackendLanguage,
} from '@/utils/backend-runtime';
import { ReactElement, ReactNode, useSyncExternalStore } from 'react';

// The store collapses every non-Go answer (including the not-yet-loaded and
// fetch-failed cases) to Python, so the world is binary below the gate.
const isGo = (): boolean => getBackendLanguage() === 'go';

/** Whether the active backend is the Go one. Safe to call anywhere below the bootstrap gate. */
export function useIsGoBackend(): boolean {
  return useSyncExternalStore(subscribeBackendLanguage, isGo, isGo);
}

type BackendVariantProps = {
  go: ReactNode;
  python: ReactNode;
};

/** Render the subtree matching the current backend. Use at dispatcher files only. */
export function BackendVariant({
  go,
  python,
}: BackendVariantProps): ReactElement {
  return <>{useIsGoBackend() ? go : python}</>;
}

/** Pick a value per backend, synchronously. Use inside adapter/dispatcher files only. */
export function pickByBackend<T>(variants: { go: T; python: T }): T {
  return isGo() ? variants.go : variants.python;
}
