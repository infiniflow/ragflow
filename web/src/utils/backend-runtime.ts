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

export const subscribeBackendLanguage = (listener: Listener): (() => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};
