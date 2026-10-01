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

import { act, render, waitFor } from '@testing-library/react';

import { PdfPreview } from './pdf-preview';

jest.mock('@/components/ui/spin', () => ({
  Spin: () => <div data-testid="spin" />,
}));

jest.mock('@/pages/document-viewer/file-error', () => ({
  __esModule: true,
  default: ({ children }: { children?: React.ReactNode }) => children,
}));

jest.mock('./hooks', () => ({
  useCatchDocumentError: () => '',
}));

const mockScrollTo = jest.fn();

jest.mock('react-pdf-highlighter', () => {
  const React = jest.requireActual('react');
  const createElement = React.createElement;

  const PdfLoader = ({
    children,
  }: {
    children: (pdfDocument: unknown) => React.ReactNode;
  }) => {
    const pdfDocument = {
      getPage: () =>
        Promise.resolve({
          getViewport: () => ({ width: 0, height: 0 }),
        }),
    };
    return createElement(React.Fragment, null, children(pdfDocument));
  };

  const PdfHighlighter = ({
    scrollRef,
    highlights,
  }: {
    scrollRef: (scrollTo: jest.Mock) => void;
    highlights: unknown[];
  }) => {
    scrollRef(mockScrollTo);
    return createElement('div', {
      'data-testid': 'pdf-highlighter',
      'data-highlight-count': String(highlights.length),
    });
  };

  const Highlight = () => null;

  const AreaHighlight = () => null;

  const Popup = ({ children }: { children: React.ReactNode }) =>
    createElement(React.Fragment, null, children);

  return {
    __esModule: true,
    PdfLoader,
    PdfHighlighter,
    Highlight,
    AreaHighlight,
    Popup,
  };
});

const makeHighlight = (
  pageNumber: number,
  id: string,
): {
  id: string;
  comment: { text: string; emoji: string };
  content: { text: string };
  position: {
    boundingRect: {
      width: number;
      height: number;
      x1: number;
      x2: number;
      y1: number;
      y2: number;
      pageNumber: number;
    };
    rects: unknown[];
    pageNumber: number;
  };
} => ({
  id,
  comment: { text: '', emoji: '' },
  content: { text: '' },
  position: {
    boundingRect: {
      width: 0,
      height: 0,
      x1: 0,
      x2: 0,
      y1: 0,
      y2: 0,
      pageNumber,
    },
    rects: [],
    pageNumber,
  },
});

beforeEach(() => {
  mockScrollTo.mockClear();
  jest.useFakeTimers();
});

afterEach(() => {
  jest.useRealTimers();
});

describe('PdfPreview', () => {
  it('highlights every page of a multi-page chunk (#16735)', async () => {
    const highlights = [
      makeHighlight(85, 'page-85'),
      makeHighlight(86, 'page-86'),
    ] as never;

    render(
      <PdfPreview url="http://example.com/doc.pdf" highlights={highlights} />,
    );

    await waitFor(() =>
      expect(mockScrollTo.getMockImplementation()).toBeUndefined(),
    );

    await act(async () => {
      jest.advanceTimersByTime(200);
    });

    expect(mockScrollTo).toHaveBeenCalledTimes(1);
    expect(mockScrollTo.mock.calls[0][0]).toBe(highlights[0]);
    expect(document.querySelector("[data-highlight-count]")).toHaveAttribute(
      "data-highlight-count",
      "2",
    );
  });

  it('highlights a single page chunk exactly once', async () => {
    const highlights = [makeHighlight(7, 'page-7')] as never;

    render(
      <PdfPreview url="http://example.com/doc.pdf" highlights={highlights} />,
    );

    await act(async () => {
      jest.advanceTimersByTime(200);
    });

    expect(mockScrollTo).toHaveBeenCalledTimes(1);
    expect(mockScrollTo.mock.calls[0][0]).toBe(highlights[0]);
  });

  it('scrolls to the first highlight after the selection changes', async () => {
    const first = [makeHighlight(7, 'page-7')] as never;
    const next = [makeHighlight(42, 'page-42')] as never;
    const { rerender } = render(
      <PdfPreview url="http://example.com/doc.pdf" highlights={first} />,
    );
    await act(async () => {
      jest.advanceTimersByTime(200);
    });
    rerender(<PdfPreview url="http://example.com/doc.pdf" highlights={next} />);
    await act(async () => {
      jest.advanceTimersByTime(200);
    });
    expect(mockScrollTo).toHaveBeenCalledTimes(2);
    expect(mockScrollTo.mock.calls[1][0]).toBe(next[0]);
  });

  it('does not scroll when the chunk has no highlights', async () => {
    render(<PdfPreview url="http://example.com/doc.pdf" highlights={[]} />);

    await act(async () => {
      jest.advanceTimersByTime(200);
    });

    expect(mockScrollTo).not.toHaveBeenCalled();
  });

  it('does not scroll when the chunk has no highlights prop', async () => {
    render(<PdfPreview url="http://example.com/doc.pdf" />);

    await act(async () => {
      jest.advanceTimersByTime(200);
    });

    expect(mockScrollTo).not.toHaveBeenCalled();
  });
});
