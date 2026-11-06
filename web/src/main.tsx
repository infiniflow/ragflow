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

import React from 'react';
import { Inspector } from 'react-dev-inspector';
import ReactDOM from 'react-dom/client';
import '../tailwind.css';
import App from './app';
import './global.less';
import { initLanguage } from './locales/config';
import { fetchBillingStatus } from '@/ee/services/billingStatus';

// Vite dispatches `vite:preloadError` when a lazily imported chunk fails to
// load, typically because a stale tab still references hashed files that a
// newer deployment removed. Reload once to pick up the fresh index.html; if
// the failure repeats within ReloadGuardWindowMs, let the error reach the
// error boundary instead of reloading in a loop.
const ChunkReloadFlagKey = 'ragflow-chunk-reload-at';
const ReloadGuardWindowMs = 60_000;

window.addEventListener('vite:preloadError', (event) => {
  const lastReloadAt = Number(sessionStorage.getItem(ChunkReloadFlagKey) ?? 0);
  if (Date.now() - lastReloadAt < ReloadGuardWindowMs) {
    return;
  }
  sessionStorage.setItem(ChunkReloadFlagKey, String(Date.now()));
  event.preventDefault();
  window.location.reload();
});

initLanguage()
  .then(() => fetchBillingStatus())
  .then(() => {
    ReactDOM.createRoot(document.getElementById('root')!).render(
      <React.StrictMode>
        <Inspector keys={['alt', 'c']} />
        <App />
      </React.StrictMode>,
    );
  });
