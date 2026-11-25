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

import { render, screen } from '@testing-library/react';
import i18next from 'i18next';
import { I18nextProvider, initReactI18next } from 'react-i18next';
import en from '@/locales/en';

// This suite uses jest.mock, so esbuild-jest's babel transform does not strip
// TypeScript. JSX here uses the automatic runtime; no React import is needed.

// Keep the real i18next key parser: the app's locale config leaves the default
// nsSeparator (':'), so unit keys such as "page:parse" must be explicitly
// opted out at the call site. A dictionary-mock would hide that regression.
jest.mock('@/services/knowledge-service', () => ({
  getIngestionProgress: jest.fn(),
}));

jest.mock('@/ee/components/ingestion-progress-detail-button', () => ({
  __esModule: true,
  default: () => null,
}));

import { IngestionProgressSummary } from './ingestion-progress';

const instance = i18next.createInstance();

beforeAll(async () => {
  await instance.use(initReactI18next).init({
    lng: 'en',
    fallbackLng: 'en',
    resources: { en: en },
    interpolation: { escapeValue: false },
  });
});

function summary(overrides = {}) {
  return {
    status: '1',
    current: 1,
    total: 10,
    percent: 10,
    duration_s: 5,
    chunk_num: 0,
    fallback: false,
    ...overrides,
  };
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function renderSummary(data: any) {
  return render(
    <I18nextProvider i18n={instance}>
      <IngestionProgressSummary
        datasetId="ds1"
        logId="run1"
        data={data}
        error={false}
      />
    </I18nextProvider>,
  );
}

describe('IngestionProgressSummary', () => {
  it('renders nothing when the query is idle or still loading', () => {
    // Adversarial: no run identity (idle) and a pending first fetch are not
    // errors. Showing the unavailable state here would present a normal
    // read as broken, and permanently for consumers without a run.
    for (const state of [{ idle: true }, { loading: true }]) {
      const view = render(
        <I18nextProvider i18n={instance}>
          <IngestionProgressSummary data={null} error={false} {...state} />
        </I18nextProvider>,
      );
      expect(screen.queryByText(/temporarily unavailable/)).toBeNull();
      view.unmount();
    }
  });

  it('still reports an actual fetch failure', () => {
    render(
      <I18nextProvider i18n={instance}>
        <IngestionProgressSummary data={null} error={true} />
      </I18nextProvider>,
    );
    expect(screen.getByText(/temporarily unavailable/)).toBeTruthy();
  });

  it('shows a distinct label for each terminal status', () => {
    const cases = [
      ['3', 'Completed'],
      ['4', 'Failed'],
      ['2', 'Stopped'],
      ['1', 'Parsing'],
    ];
    for (const [status, label] of cases) {
      const view = renderSummary(summary({ status }));
      expect(screen.getByText(label)).toBeTruthy();
      view.unmount();
    }
  });

  it('translates colon-separated unit keys with the real i18next parser', () => {
    // Adversarial: the default nsSeparator splits "units.page:parse" into a
    // namespace, so without nsSeparator:false the raw key leaks to the user.
    for (const [unit, label] of [
      ['batch:generate', 'Batch generation'],
      ['page:parse', 'Page parsing'],
    ]) {
      const view = renderSummary(
        summary({ status: '1', stage: 'Compiler', unit, current: 2, total: 5 }),
      );
      expect(screen.getByText(new RegExp(label))).toBeTruthy();
      view.unmount();
    }
  });

  it('falls back to the raw unit for an unknown colon-separated key', () => {
    const view = renderSummary(
      summary({ status: '1', stage: 'Compiler', unit: 'custom:unit' }),
    );
    expect(screen.getByText(/custom:unit/)).toBeTruthy();
    view.unmount();
  });

  it('constrains server-returned stage and unit text', () => {
    renderSummary(
      summary({
        status: '1',
        stage: 'A very long stage name from the server',
        unit: 'batch:embed',
      }),
    );
    const value = screen.getByText(/Batch embedding/);
    expect(value.className).toContain('break-words');
    expect(value.className).toContain('min-w-0');
  });
});
