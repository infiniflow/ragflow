import '@/locales/config';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
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
    </MemoryRouter>,
  );
}

describe('TestingResult', () => {
  it('links the document name to the chunk browser with the chunk id', () => {
    renderResult([chunk()]);

    expect(screen.getByRole('link', { name: 'handbook.pdf' })).toHaveAttribute(
      'href',
      '/chunk/parsed/chunks?id=kb-1&doc_id=doc-1&chunk_id=chunk-1',
    );
  });

  it('prefers the dataset id carried by the chunk itself', () => {
    renderResult([chunk({ dataset_id: 'kb-other' })]);

    expect(screen.getByRole('link', { name: 'handbook.pdf' })).toHaveAttribute(
      'href',
      '/chunk/parsed/chunks?id=kb-other&doc_id=doc-1&chunk_id=chunk-1',
    );
  });

  // Retrieval results only live in memory, so a same-tab navigation would
  // throw them away; the link also has to survive a middle click.
  it('opens the document in a new tab', () => {
    renderResult([chunk()]);
    const link = screen.getByRole('link', { name: 'handbook.pdf' });

    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noreferrer');
  });

  it('leaves the result body inert so its text stays selectable', () => {
    renderResult([chunk()]);

    expect(screen.queryByRole('button')).not.toBeInTheDocument();
    expect(screen.getByText('Retrieved text')).toBeInTheDocument();
  });
});
