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
import { DocPreviewer } from './doc-preview';

const mockImportDocxFile = jest.fn().mockResolvedValue(undefined);
let mockEditorImportDocxFile = mockImportDocxFile;

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

jest.mock('./hooks', () => ({
  isZipLikeBlob: jest.fn().mockResolvedValue(true),
  useDocumentResizeObserver: () => ({
    containerWidth: 800,
    setContainerRef: jest.fn(),
  }),
  useDocxPreviewZoom: () => ({
    zoomScale: 100,
    minZoom: 50,
    maxZoom: 200,
    handleZoomIn: jest.fn(),
    handleZoomOut: jest.fn(),
  }),
}));

jest.mock('@extend-ai/react-docx', () => ({
  DocxEditorViewer: ({
    pageVirtualization,
  }: {
    pageVirtualization?: { enabled?: boolean };
  }) => (
    <div
      data-testid="docx-viewer"
      data-page-virtualization={JSON.stringify(pageVirtualization)}
    />
  ),
  useDocxEditor: () => ({
    importDocxFile: mockEditorImportDocxFile,
    status: 'ready',
    totalPages: 2,
  }),
  useDocxPageLayout: () => ({ layout: { pageWidthPx: 800 } }),
  parseDocx: jest.fn(),
  packageToArrayBuffer: jest.fn(),
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

describe('DocPreviewer', () => {
  beforeEach(() => {
    mockEditorImportDocxFile = mockImportDocxFile;
    mockImportDocxFile.mockClear();
    mockImportDocxFile.mockImplementation(async () => {
      // Mimic @extend-ai/react-docx returning a new callback after import.
      mockEditorImportDocxFile = jest.fn().mockResolvedValue(undefined);
    });
    MockRequest.mockResolvedValue({
      data: new Blob([new Uint8Array([0x50, 0x4b, 0x03, 0x04])], {
        type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
      }),
    } as never);
  });

  it('loads the document once even when importDocxFile identity changes', async () => {
    const { rerender } = render(
      <DocPreviewer url="http://example.com/document.docx" />,
    );

    await waitFor(() => {
      expect(mockImportDocxFile).toHaveBeenCalledTimes(1);
    });

    rerender(<DocPreviewer url="http://example.com/document.docx" />);

    await waitFor(() => {
      expect(screen.getByTestId('docx-viewer')).toBeInTheDocument();
    });
    expect(mockImportDocxFile).toHaveBeenCalledTimes(1);
  });

  it('disables internal DOCX page virtualization to avoid update loops', async () => {
    render(<DocPreviewer url="http://example.com/document.docx" />);

    await waitFor(() => {
      expect(screen.getByTestId('docx-viewer')).toBeInTheDocument();
    });

    expect(screen.getByTestId('docx-viewer')).toHaveAttribute(
      'data-page-virtualization',
      JSON.stringify({ enabled: false }),
    );
  });
});
