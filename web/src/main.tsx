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
// oxlint-disable-next-line no-restricted-imports -- bootstrap gate: resolve the backend variant before first render
import { fetchBackendLanguage } from './utils/backend-runtime';

// Gate first render on the backend-language probe so every variant dispatch
// below sees a concrete value (no python-flash-then-switch).
Promise.all([initLanguage(), fetchBackendLanguage()]).then(() => {
  ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
      <Inspector keys={['alt', 'c']} />
      <App />
    </React.StrictMode>,
  );
});
