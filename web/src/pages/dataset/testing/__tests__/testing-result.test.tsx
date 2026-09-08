import '@/locales/config';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';
import { TestingResult } from '../testing-result';

type ITestingChunk = import('@/interfaces/database/dataset').ITestingChunk;

// src/routes.tsx builds a browser router at module scope, which jsdom cannot
// do (no global Request). Only the parsed-result path is needed here.
jest.mock('@/routes', () => ({
  Routes: { ParsedResult: '/chunk/parsed' },
}));

// The real module pulls the whole request stack (axios, SSE streams) into
// jsdom; only the route-derived dataset id matters here.
jest.mock('@/hooks/use-knowledge-request', () => ({
  useKnowledgeBaseId: () => 'kb-1',
}));

jest.mock('@/components/list-filter-bar', () => ({
  FilterButton: () => null,
}));

jest.mock('@/components/list-filter-bar/filter-popover', () => ({
  FilterPopover: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

jest.mock('@/components/empty/empty', () => ({
  __esModule: true,
  default: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

function chunk(overrides: Partial<ITestingChunk> = {}): ITestingChunk {
  return {
    id: 'chunk-1',
    content: 'Retrieved text',
    content_ltks: '',
    document_id: 'doc-1',
    document_keyword: 'handbook.pdf',
    image_id: '',
    important_keywords: [],
    dataset_id: '',
    similarity: 0.9,
    term_similarity: 0.8,
    vector_similarity: 0.7,
    highlight: '',
    positions: [],
    doc_type_kwd: '',
    ...overrides,
  };
}

function LocationSpy() {
  const location = useLocation();
  return (
    <div data-testid="location">{`${location.pathname}${location.search}`}</div>
  );
}

function renderResult(chunks: ITestingChunk[]) {
  return render(
    <MemoryRouter initialEntries={['/dataset/retrieval/kb-1']}>
      <Routes>
        <Route
          path="/dataset/retrieval/:id"
          element={
            <TestingResult
              data={{ chunks, doc_aggs: [], total: chunks.length }}
              loading={false}
              filterValue={{}}
              handleFilterSubmit={jest.fn()}
            />
          }
        />
        <Route path="*" element={null} />
      </Routes>
      <LocationSpy />
    </MemoryRouter>,
  );
}

describe('TestingResult', () => {
  beforeEach(() => {
    window.getSelection()?.removeAllRanges();
  });

  it('exposes each result as a keyboard reachable button', () => {
    renderResult([chunk()]);

    const card = screen.getByRole('button', {
      name: 'Open this chunk in the document',
    });
    expect(card).toHaveAttribute('tabindex', '0');
    expect(card).toHaveTextContent('Retrieved text');
  });

  it('navigates to the chunk browser with the document and chunk id', () => {
    renderResult([chunk()]);

    fireEvent.click(
      screen.getByRole('button', {
        name: 'Open this chunk in the document',
      }),
    );

    expect(screen.getByTestId('location')).toHaveTextContent(
      '/chunk/parsed/chunks?id=kb-1&doc_id=doc-1&chunk_id=chunk-1',
    );
  });

  it('prefers the dataset id carried by the chunk itself', () => {
    renderResult([chunk({ dataset_id: 'kb-other' })]);

    fireEvent.click(
      screen.getByRole('button', {
        name: 'Open this chunk in the document',
      }),
    );

    expect(screen.getByTestId('location')).toHaveTextContent('id=kb-other');
  });

  it.each(['Enter', ' '])('activates on %s', (key) => {
    renderResult([chunk()]);
    const card = screen.getByRole('button', {
      name: 'Open this chunk in the document',
    });

    fireEvent.keyDown(card, { key });

    expect(screen.getByTestId('location')).toHaveTextContent('chunk_id=chunk-1');
  });

  it('does not navigate when the click only ends a text selection', () => {
    renderResult([chunk()]);
    const card = screen.getByRole('button', {
      name: 'Open this chunk in the document',
    });

    const range = document.createRange();
    range.selectNodeContents(card);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    // Guards against a vacuous assertion if jsdom ever stops tracking ranges.
    expect(selection?.isCollapsed).toBe(false);

    fireEvent.click(card);

    expect(screen.getByTestId('location')).toHaveTextContent(
      '/dataset/retrieval/kb-1',
    );
  });
});
