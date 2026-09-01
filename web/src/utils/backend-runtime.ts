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
 * Backend runtime language detection.
 *
 * Fetches /api/v1/language once at module load (app start) and caches the
 * result. Components subscribe via React's useSyncExternalStore; there is no
 * per-component fetch or useEffect.
 *
 * Pattern: module-level fetch + listener set, same subscription approach as
 * enterprise billingStatus.ts.
 */

type Listener = () => void;
const listeners = new Set<Listener>();

let backendLanguage: string | null = null;

// Kick off the fetch at module load — app start, not component mount.
const promise: Promise<string> = fetch('/api/v1/language')
  .then((r) => r.json())
  .then((body: { data?: { language?: string } }) => {
    backendLanguage = body.data?.language === 'go' ? 'go' : 'python';
    listeners.forEach((fn) => fn());
    return backendLanguage;
  })
  .catch(() => {
    backendLanguage = 'python';
    listeners.forEach((fn) => fn());
    return 'python';
  });

export const fetchBackendLanguage = (): Promise<string> => promise;

export const getBackendLanguage = (): string | null => backendLanguage;

export const isGoBackend = (): boolean => backendLanguage === 'go';

/**
 * debugRunLimitsTooltipKey returns the i18n key for the canvas "Run" button
 * tooltip describing the debug (dry-run) preview limits, or null when the
 * tooltip should not be shown.
 *
 * The tooltip applies ONLY to a dataflow (ingestion pipeline) canvas on the
 * golang backend. An agent canvas runs the agent/chat, not an ingestion
 * debug preview, so it must never show this tooltip. The python backend's
 * debug semantics also differ and are out of scope.
 *
 * It is a pure function of (backend language, is-pipeline) so it can be
 * unit-tested without mocking, and the Agent derives it via
 * useSyncExternalStore(subscribeBackendLanguage, () =>
 *   debugRunLimitsTooltipKey(isGoBackend(), isPipeline)).
 *
 * The tooltip copy itself never names the backend language; only its
 * visibility is gated on these conditions.
 */
export const debugRunLimitsTooltipKey = (
  isGo: boolean,
  isPipeline: boolean,
): string | null => (isGo && isPipeline ? 'flow.debugRunLimits' : null);

export const subscribeBackendLanguage = (listener: Listener): (() => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};
