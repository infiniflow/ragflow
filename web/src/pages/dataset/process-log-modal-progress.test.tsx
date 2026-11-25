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

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/hooks/common-hooks', () => ({
  useTranslate: () => ({ t: (key: string) => key }),
}));

jest.mock('./ingestion-progress', () => ({
  useIngestionProgress: () => ({
    data: {
      status: '1',
      stage: 'Parser',
      unit: 'page:parse',
      current: 1,
      total: 10,
      percent: 10,
    },
    error: false,
  }),
  IngestionProgressSummary: () => {
    const ReactLib = jest.requireActual('react');
    return ReactLib.createElement(
      'div',
      { 'data-testid': 'progress-summary' },
      'Parser',
    );
  },
}));

import ProcessLogModal from './process-log-modal';
import { TooltipProvider } from '@/components/ui/tooltip';

it('shows the progress summary before any event has arrived', () => {
  render(
    React.createElement(
      TooltipProvider,
      null,
      React.createElement(ProcessLogModal, {
        visible: true,
        onCancel: jest.fn(),
        title: 'log',
        logInfo: { fileName: 'doc.pdf', details: '' },
        datasetId: 'ds1',
        logId: 'run1',
      }),
    ),
  );

  expect(screen.getByTestId('progress-summary')).toBeTruthy();
});
