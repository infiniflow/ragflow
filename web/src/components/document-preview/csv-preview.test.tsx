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

import request from '@/utils/request';
import { render, screen, waitFor } from '@testing-library/react';
import CSVFileViewer from './csv-preview';

jest.mock('@/utils/request', () => ({
  __esModule: true,
  default: jest.fn(),
}));

jest.mock('@/utils/file-util', () => ({
  decodeBlobText: jest
    .fn()
    .mockResolvedValue('direction,model\nmultimodal,GPT-4o\nreasoning,o1'),
}));

jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { error: jest.fn() },
}));

jest.mock('@/components/ui/spin', () => ({
  Spin: () => <div data-testid="spin" />,
}));

const MockRequest = jest.mocked(request);
const OriginalResizeObserver = globalThis.ResizeObserver;

beforeAll(() => {
  globalThis.ResizeObserver = jest.fn().mockImplementation(() => ({
    observe: jest.fn(),
    unobserve: jest.fn(),
    disconnect: jest.fn(),
  }));
});

afterAll(() => {
  globalThis.ResizeObserver = OriginalResizeObserver;
});

describe('CSVFileViewer', () => {
  beforeEach(() => {
    MockRequest.mockResolvedValue({ data: new Blob(['csv']) } as never);
  });

  it('renders headers and data rows from the fetched CSV', async () => {
    render(<CSVFileViewer url="http://example.com/table.csv" />);

    await waitFor(() => {
      expect(screen.getByText('direction')).toBeInTheDocument();
    });
    expect(screen.getAllByText('multimodal').length).toBeGreaterThan(0);
    expect(screen.getAllByText('reasoning').length).toBeGreaterThan(0);
  });

  it('paints the sticky header row with an opaque surface so scrolled rows stay hidden beneath it', async () => {
    render(<CSVFileViewer url="http://example.com/table.csv" />);

    const headerRow = await waitFor(() =>
      screen.getByText('direction').closest('.sticky'),
    );

    expect(headerRow).not.toBeNull();
    expect(headerRow).toHaveClass('bg-bg-canvas');
    expect(headerRow).not.toHaveClass('bg-background-header-bar');
  });
});
