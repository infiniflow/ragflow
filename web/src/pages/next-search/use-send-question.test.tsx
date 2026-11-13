import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook } from '@testing-library/react';
import { useState } from 'react';
import { useSendQuestion } from './hooks';

const mockSend = jest.fn();
const mockRetrieval = jest.fn();

jest.mock('@/hooks/logic-hooks', () => ({
  useGetPaginationWithRouter: () => ({}),
  useSendMessageWithSse: () => ({
    send: (...args: unknown[]) => mockSend(...args),
    answer: {},
    done: true,
    error: 'summary failed',
    stopOutputMessage: jest.fn(),
  }),
}));
jest.mock('@/hooks/route-hook', () => ({
  useSetPaginationParams: () => ({ page: 1, size: 10 }),
}));
jest.mock('@/hooks/use-knowledge-request', () => ({
  useKnowledgeBaseId: () => 'kb-1',
  useSelectTestingResult: () => ({ chunks: [], total: 0 }),
}));
jest.mock('@/services/knowledge-service', () => ({
  __esModule: true,
  default: {
    retrievalTest: (...args: unknown[]) => mockRetrieval(...args),
    retrievalTestShare: jest.fn(),
  },
}));
jest.mock('@/services/next-chat-service', () => ({
  __esModule: true,
  default: { chatsRelatedQuestions: jest.fn() },
}));
jest.mock('@/services/search-service', () => ({
  __esModule: true,
  default: { getRelatedQuestionsShare: jest.fn() },
}));
jest.mock('react-router', () => ({
  useSearchParams: () => [new URLSearchParams()],
}));
jest.mock('../next-searches/hooks', () => ({}));
jest.mock('./document-preview-modal/hooks', () => ({}));

function wrapper({ children }: { children?: any }) {
  const [client] = useState(() => new QueryClient());
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  mockSend.mockReset();
  mockRetrieval.mockReset();
  mockRetrieval.mockResolvedValue({
    data: { code: 0, data: { doc_aggs: [] } },
  });
});

it('retries the AI summary without resetting the document filter or re-running retrieval', async () => {
  const { result } = renderHook(
    () => useSendQuestion(['kb-1'], 'tenant-1', 'search-1'),
    { wrapper },
  );

  await act(async () => {
    result.current.sendQuestion('what is ragflow');
  });
  expect(mockSend).toHaveBeenCalledTimes(1);
  expect(mockRetrieval).toHaveBeenCalledTimes(1);

  // The user narrows the results while the summary has failed.
  act(() => result.current.setSelectedDocumentIds(['doc-1', 'doc-2']));
  act(() => result.current.setSearchStr('something typed afterwards'));

  await act(async () => {
    result.current.retryAnswer();
  });

  expect(mockSend).toHaveBeenCalledTimes(2);
  expect(mockSend.mock.calls[1][1]).toMatchObject({
    question: 'what is ragflow',
    search_id: 'search-1',
  });
  expect(result.current.selectedDocumentIds).toEqual(['doc-1', 'doc-2']);
  expect(mockRetrieval).toHaveBeenCalledTimes(1);
  expect(result.current.answerError).toBe('summary failed');
});

it('does nothing when there is no question to retry', async () => {
  const { result } = renderHook(
    () => useSendQuestion(['kb-1'], 'tenant-1', 'search-1'),
    { wrapper },
  );

  await act(async () => {
    result.current.retryAnswer();
  });
  expect(mockSend).not.toHaveBeenCalled();
});

it('a new search still resets the document filter', async () => {
  const { result } = renderHook(
    () => useSendQuestion(['kb-1'], 'tenant-1', 'search-1'),
    { wrapper },
  );
  act(() => result.current.setSelectedDocumentIds(['doc-1']));

  await act(async () => {
    result.current.sendQuestion('another question');
  });

  expect(result.current.selectedDocumentIds).toEqual([]);
  expect(mockRetrieval).toHaveBeenCalledTimes(1);
});
