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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import Chunk from '..';

const mockPageChange = jest.fn();
// One object for every render, like the real hook's memoized result; a fresh
// pagination object each render would retrigger the page's selection effect.
const mockChunkList = {
  data: { documentInfo: {}, data: [], total: 0 },
  pagination: { current: 3, pageSize: 10, onChange: mockPageChange },
  loading: false,
  searchString: '',
  handleInputChange: jest.fn(),
  available: undefined,
  handleSetAvailable: jest.fn(),
  dataUpdatedAt: 0,
};

jest.mock('@/hooks/use-chunk-request', () => ({
  ...jest.requireActual('@/hooks/use-chunk-request'),
  useFetchNextChunkList: () => mockChunkList,
  useSwitchChunk: () => ({ switchChunk: jest.fn() }),
}));

// The graph view is where a user picks a node; stand in for its two calls.
jest.mock('../components/document-view-switch', () => ({
  __esModule: true,
  default: ({
    onChunkIdsChange,
  }: {
    onChunkIdsChange: (chunkIds: string[]) => void;
  }) => (
    <>
      <button onClick={() => onChunkIdsChange(['chunk-1', 'chunk-2'])}>
        pick node
      </button>
      <button onClick={() => onChunkIdsChange([])}>back to preview</button>
    </>
  ),
}));

// claim-list cannot go through esbuild-jest's babel pass (it uses an imported
// type in an annotation), and its panels play no part in pagination.
jest.mock('@/pages/chunk/representation/components/claim-list', () => ({
  ClaimsPanel: () => null,
  NodeDetailPanel: () => null,
}));

// The preview hooks drag in ESM-only spreadsheet viewers that Jest cannot load.
jest.mock('@/components/document-preview/hooks', () => ({
  useGetDocumentUrl: () => '/preview',
}));

// react-resizable-panels ships ESM only; the layout plays no part here.
jest.mock('@/components/ui/resizable', () => ({
  ResizablePanelGroup: ({ children }: { children?: React.ReactNode }) => (
    <>{children}</>
  ),
  ResizablePanel: ({ children }: { children?: React.ReactNode }) => (
    <>{children}</>
  ),
  ResizableHandle: () => null,
}));

// route-hook pulls in the app shell; routes constructs a browser router at
// module scope, which jsdom cannot survive.
jest.mock('@/routes', () => ({ Routes: {} }));

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/chunk?page=3&size=10']}>
      <QueryClientProvider client={new QueryClient()}>
        <Chunk />
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

describe('Chunk page chunk id filter', () => {
  beforeEach(() => mockPageChange.mockReset());

  it('returns to the first page when a node filter is applied', () => {
    renderPage();
    fireEvent.click(screen.getByText('pick node'));
    expect(mockPageChange).toHaveBeenCalledWith(1, 10);
  });

  it('returns to the first page when the filter is cleared', () => {
    renderPage();
    fireEvent.click(screen.getByText('back to preview'));
    expect(mockPageChange).toHaveBeenCalledWith(1, 10);
  });
});
