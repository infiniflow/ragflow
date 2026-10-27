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

jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { error: jest.fn() },
}));

jest.mock('@/components/ui/spin', () => ({
  Spin: () => <div data-testid="spin" />,
}));

// jsdom's Blob has no arrayBuffer(), so the real decoder cannot read it.
let mockCsvText = '';
jest.mock('@/utils/file-util', () => ({
  decodeBlobText: async () => mockCsvText,
}));

const MockRequest = jest.mocked(request);
const OriginalResizeObserver = globalThis.ResizeObserver;

const respondWith = (csvText: string) => {
  mockCsvText = csvText;
  MockRequest.mockResolvedValue({ data: new Blob([]) } as never);
};

const renderViewer = async (csvText: string, positions?: number[][]) => {
  respondWith(csvText);
  const view = render(
    <CSVFileViewer url="http://example.com/data.csv" positions={positions} />,
  );
  await waitFor(() =>
    expect(screen.queryByTestId('spin')).not.toBeInTheDocument(),
  );
  return view;
};

/** Cells painted with the chunk highlight, as "row:col" against the DOM. */
const highlightedCells = () =>
  [...document.querySelectorAll('.table-row')].flatMap((row, rowIndex) =>
    [...row.querySelectorAll('.table-cell')].flatMap((cell, cellIndex) =>
      cell.className.includes('bg-state-warning')
        ? [`${rowIndex}:${cellIndex}`]
        : [],
    ),
  );

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

beforeEach(() => {
  MockRequest.mockReset();
});

describe('CSVFileViewer', () => {
  it('renders an empty file without crashing', async () => {
    await renderViewer('');
    expect(screen.queryByTestId('spin')).not.toBeInTheDocument();
    expect(document.querySelectorAll('.table-cell')).toHaveLength(0);
  });

  it('highlights the cells a chunk position covers', async () => {
    await renderViewer('h1,h2\nr1a,r1b\nr2a,r2b\n', [[1, 2, 2, 2, 2]]);
    // Row 0 is the header; row 1 is data record 2.
    expect(highlightedCells()).toEqual(['1:1']);
  });

  it('highlights nothing when a position cannot be applied', async () => {
    await renderViewer('h1,h2\nr1a,r1b\n', [[1, 7, 7, 1, 1]]);
    expect(highlightedCells()).toEqual([]);
    expect(screen.getAllByText('r1a').length).toBeGreaterThan(0);
  });

  it('locates a row far past the first rendered window', async () => {
    // 150k rows: the old Math.max(...lengths) threw a RangeError here, and the
    // virtualized window means the target row is not in the initial render.
    const lines = ['h1,h2,h3'];
    for (let i = 0; i < 150_000; i++) {
      lines.push(`a${i},b${i},c${i}`);
    }
    await renderViewer(`${lines.join('\n')}\n`, [[1, 150_000, 150_000, 2, 2]]);

    const container = document.querySelector('.overflow-auto') as HTMLElement;
    expect(container).not.toBeNull();
    await waitFor(() => expect(container.scrollTop).toBeGreaterThan(0));
    expect(screen.getAllByText('b149998').length).toBeGreaterThan(0);
    // Exactly one cell is highlighted, and it holds the chunk's column of the
    // target row — the virtual window only decides where that row lands in the
    // DOM, so the assertion follows the highlighted cell back to its text.
    const highlighted = [...document.querySelectorAll('.table-cell')].filter(
      (cell) => cell.className.includes('bg-state-warning'),
    );
    expect(highlighted).toHaveLength(1);
    expect(highlighted[0].textContent).toBe('b149998');
  });

  it('renders the table in place and does not scroll when positions are rejected', async () => {
    await renderViewer('h1,h2\nr1a,r1b\nr2a,r2b\n', [[2, 1, 1, 1, 1]]);
    const container = document.querySelector('.overflow-auto') as HTMLElement;
    expect(container.scrollTop).toBe(0);
    expect(highlightedCells()).toEqual([]);
    expect(screen.getAllByText('r2b').length).toBeGreaterThan(0);
  });
});
