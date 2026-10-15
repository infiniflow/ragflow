import { expect, it, jest } from '@jest/globals';
import kbService from './knowledge-service';

const mockRequest = jest.fn<(...args: any[]) => Promise<any>>();

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: (...args: unknown[]) => mockRequest(...args),
}));
jest.mock('@/utils/request', () => ({ __esModule: true, default: {} }));

it('requests entity chunks by a short GET URL and maps the usual chunk response', async () => {
  mockRequest.mockResolvedValue({
    data: { code: 0, data: { chunks: [{ id: 'c1' }], total: 125, doc: {} } },
  });
  const params = { page: 2, page_size: 100, keywords: 'x', available: false };
  const response = await kbService.entityChunkList(
    'kb-1',
    'doc-1',
    'entity-1',
    params,
  );
  expect(mockRequest).toHaveBeenCalledWith(
    expect.objectContaining({
      url: '/api/v1/datasets/kb-1/documents/doc-1/structure/entities/entity-1/chunks',
      method: 'get',
      params,
    }),
  );
  expect(response.data.data).toMatchObject({
    chunks: [{ chunk_id: 'c1' }],
    total: 125,
  });
});
