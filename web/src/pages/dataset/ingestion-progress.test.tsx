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
import React from 'react';

const mockTranslations = {
  'knowledgeDetails.ingestionProgress.status': 'Status',
  'knowledgeDetails.ingestionProgress.parsing': 'Parsing',
  'knowledgeDetails.ingestionProgress.completed': 'Completed',
  'knowledgeDetails.ingestionProgress.failed': 'Failed',
  'knowledgeDetails.ingestionProgress.stopped': 'Stopped',
  'knowledgeDetails.ingestionProgress.totalDuration': 'Total duration',
  'knowledgeDetails.ingestionProgress.chunks': 'Chunks',
  'knowledgeDetails.ingestionProgress.currentStage': 'Current stage',
  'knowledgeDetails.ingestionProgress.progress': 'Progress',
  'knowledgeDetails.ingestionProgress.elapsed': 'Elapsed',
  'knowledgeDetails.ingestionProgress.updatedAt': 'Updated at',
  'knowledgeDetails.ingestionProgress.unavailable':
    'Progress information is temporarily unavailable; see the full log.',
  'knowledgeDetails.ingestionProgress.detailUnavailable':
    'Detailed progress is temporarily unavailable',
  'knowledgeDetails.ingestionProgress.units.batch:generate': 'Batch generation',
  'knowledgeDetails.ingestionProgress.units.batch:embed': 'Batch embedding',
  'knowledgeDetails.ingestionProgress.units.page:parse': 'Page parsing',
};

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key, options) => mockTranslations[key] ?? options?.defaultValue ?? key,
  }),
  initReactI18next: { type: '3rdParty', init: () => undefined },
}));

jest.mock('@/services/knowledge-service', () => ({
  getIngestionProgress: jest.fn(),
}));

jest.mock('@/ee/components/ingestion-progress-detail-button', () => ({
  __esModule: true,
  default: () => null,
}));

import { IngestionProgressSummary } from './ingestion-progress';

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

describe('IngestionProgressSummary', () => {
  it('shows a distinct label for each terminal status', () => {
    const cases = [
      ['3', 'Completed'],
      ['4', 'Failed'],
      ['2', 'Stopped'],
      ['1', 'Parsing'],
    ];
    for (const [status, label] of cases) {
      const view = render(
        <IngestionProgressSummary
          datasetId="ds1"
          logId="run1"
          data={summary({ status })}
          error={false}
        />,
      );
      expect(screen.getByText(label)).toBeTruthy();
      view.unmount();
    }
  });

  it('translates batch units instead of showing a raw unit key', () => {
    render(
      <IngestionProgressSummary
        data={summary({
          status: '1',
          stage: 'Compiler',
          unit: 'batch:generate',
          current: 2,
          total: 5,
        })}
        error={false}
      />,
    );
    expect(screen.getByText(/Batch generation/)).toBeTruthy();
  });

  it('constrains server-returned stage and unit text', () => {
    render(
      <IngestionProgressSummary
        data={summary({
          status: '1',
          stage: 'A very long stage name from the server',
          unit: 'batch:embed',
        })}
        error={false}
      />,
    );
    const value = screen.getByText(/Batch embedding/);
    expect(value.className).toContain('break-words');
    expect(value.className).toContain('min-w-0');
  });
});
