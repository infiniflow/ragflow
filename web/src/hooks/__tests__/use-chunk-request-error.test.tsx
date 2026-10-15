import { expect, it, jest } from '@jest/globals';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import { useFetchNextChunkList } from '../use-chunk-request';

const mockChunkList = jest.fn<(...args: any[]) => Promise<any>>();
const mockEntityChunkList = jest.fn<(...args: any[]) => Promise<any>>();

jest.mock('@/services/knowledge-service', () => ({
  __esModule: true,
  default: {
    chunkList: (...args: unknown[]) => mockChunkList(...args),
    entityChunkList: (...args: unknown[]) => mockEntityChunkList(...args),
  },
}));
jest.mock('@/components/image', () => ({ evictDocumentImage: () => {} }));
jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { success: () => {} },
}));
jest.mock('@/hooks/route-hook', () => ({
  useGetKnowledgeSearchParams: () => ({
    documentId: 'doc-1',
    knowledgeId: 'kb-1',
  }),
}));
jest.mock('@/hooks/logic-hooks', () => ({
  useGetPaginationWithRouter: () => ({
    pagination: { current: 1, pageSize: 10 },
    setPagination: () => {},
  }),
  useHandleSearchChange: () => ({
    searchString: '',
    handleInputChange: () => {},
  }),
}));

it('keeps the preview usable after an entity 404 and recovers when its filter is cleared', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function Wrapper({ children }: { children?: any }) {
    return (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
  }

  const documentInfo = { id: 'doc-1', name: 'Document.txt', type: 'txt' };
  const chunks = [{ chunk_id: 'chunk-1' }];
  mockChunkList.mockResolvedValue({
    data: { code: 0, data: { doc: documentInfo, chunks, total: 1 } },
  });
  const missingEntity = Object.assign(new Error('Entity not found'), {
    response: { status: 404 },
  });
  mockEntityChunkList.mockRejectedValue(missingEntity);

  const { result, rerender } = renderHook(
    ({ entityId }: { entityId?: string }) => {
      const list = useFetchNextChunkList(true, { entityId });
      // Match the page's nested destructuring, which must remain safe on error.
      const {
        data: { documentInfo, data = [], total },
      } = list;
      return { ...list, documentInfo, chunks: data, total };
    },
    {
      wrapper: Wrapper,
      initialProps: { entityId: undefined as string | undefined },
    },
  );

  await waitFor(() => expect(result.current.total).toBe(1));
  rerender({ entityId: 'deleted-entity' });
  await waitFor(() => expect(result.current.error).toBe(missingEntity));
  expect(result.current.loading).toBe(false);
  expect(result.current.documentInfo).toEqual(documentInfo);
  expect(result.current.chunks).toEqual([]);
  expect(result.current.total).toBe(0);

  rerender({ entityId: undefined });
  await waitFor(() => expect(result.current.total).toBe(1));
  expect(result.current.error).toBeNull();
  expect(result.current.chunks).toEqual(chunks);
});
