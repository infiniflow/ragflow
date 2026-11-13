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

import kbService from '@/services/knowledge-service';
import request from '@/utils/request';

const mockRequest = request as unknown as {
  get: jest.Mock;
  post: jest.Mock;
  patch: jest.Mock;
};

jest.mock('@/utils/request', () => ({
  __esModule: true,
  default: Object.assign(jest.fn(), {
    get: jest.fn(),
    post: jest.fn(),
    patch: jest.fn(),
  }),
}));

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));

jest.mock('@/utils/api', () => ({
  __esModule: true,
  default: {
    chunkList: (datasetId: string, documentId: string) =>
      `/datasets/${datasetId}/documents/${documentId}/chunks`,
    chunkDetail: (datasetId: string, documentId: string, chunkId: string) =>
      `/datasets/${datasetId}/documents/${documentId}/chunks/${chunkId}`,
  },
}));

const ChunkIds = {
  kb_id: 'kb-1',
  doc_id: 'doc-1',
  chunk_id: 'chunk-1',
};
const ChunkUrl = '/datasets/kb-1/documents/doc-1/chunks/chunk-1';

describe('chunk editor request fields', () => {
  beforeEach(() => {
    jest.resetAllMocks();
    mockRequest.post.mockResolvedValue({ data: { code: 0 } });
    mockRequest.patch.mockResolvedValue({ data: { code: 0 } });
  });

  it.each([
    { keywords: [], questions: [], storedAvailable: false, editedAvailable: 1 },
    {
      keywords: ['updated keyword'],
      questions: ['Updated question?'],
      storedAvailable: true,
      editedAvailable: 0,
    },
    {
      keywords: [],
      questions: [],
      storedAvailable: true,
      editedAvailable: 0,
    },
  ])(
    'uses edited fields instead of retained storage fields: %j',
    async ({ keywords, questions, storedAvailable, editedAvailable }) => {
      mockRequest.get.mockResolvedValue({
        data: {
          code: 0,
          data: {
            id: 'chunk-1',
            document_id: 'doc-1',
            content: 'Original content',
            content_with_weight: 'Original content',
            important_keywords: keywords.length ? 'old keyword' : '',
            important_kwd: keywords.length ? ['old keyword'] : [],
            questions: questions.length ? 'Old question?' : '',
            question_kwd: questions.length ? ['Old question?'] : [],
            available: storedAvailable,
            tag_kwd: ['existing tag'],
            tag_feas: { 'existing tag': 2 },
          },
        },
      });

      const response = await kbService.getChunk(ChunkIds);
      expect(mockRequest.get).toHaveBeenCalledWith(ChunkUrl);

      await kbService.setChunk({
        ...response.data.data,
        ...ChunkIds,
        content_with_weight: 'Edited content',
        important_kwd: keywords,
        question_kwd: questions,
        available_int: editedAvailable,
      });

      expect(mockRequest.patch).toHaveBeenCalledWith(ChunkUrl, {
        data: expect.objectContaining({
          content: 'Edited content',
          important_keywords: keywords,
          questions,
          available: editedAvailable === 1,
          tag_kwd: ['existing tag'],
          tag_feas: { 'existing tag': 2 },
        }),
      });
    },
  );

  it('preserves explicit empty edits over nonempty stored values', async () => {
    await kbService.setChunk({
      ...ChunkIds,
      content: 'Stored content',
      content_with_weight: '',
      important_keywords: ['stored keyword'],
      important_kwd: [],
      questions: ['Stored question?'],
      question_kwd: [],
      available: true,
      available_int: 0,
    });

    expect(mockRequest.patch).toHaveBeenCalledWith(ChunkUrl, {
      data: expect.objectContaining({
        content: '',
        important_keywords: [],
        questions: [],
        available: false,
      }),
    });
  });

  it.each(['setChunk', 'createChunk'] as const)(
    '%s accepts REST-only fields without changing their values',
    async (method) => {
      const payload = {
        content: 'REST content',
        important_keywords: ['keyword'],
        questions: ['Question?'],
        available: false,
        tag_kwd: ['tag'],
        tag_feas: { tag: 3 },
        positions: [[1, 2, 3, 4, 5]],
      };
      await kbService[method]({ ...ChunkIds, ...payload });
      const transport =
        method === 'setChunk' ? mockRequest.patch : mockRequest.post;

      expect(transport).toHaveBeenCalledWith(
        method === 'setChunk'
          ? ChunkUrl
          : '/datasets/kb-1/documents/doc-1/chunks',
        { data: expect.objectContaining(payload) },
      );
    },
  );

  it.each(['setChunk', 'createChunk'] as const)(
    '%s leaves omitted fields out of the serialized request',
    async (method) => {
      await kbService[method]({
        ...ChunkIds,
        content_with_weight: 'Only text',
      });
      const transport =
        method === 'setChunk' ? mockRequest.patch : mockRequest.post;
      const [, config] = transport.mock.calls[0];

      expect(JSON.parse(JSON.stringify(config.data))).toEqual({
        content: 'Only text',
      });
    },
  );

  it('maps editor-only fields when creating a chunk', async () => {
    await kbService.createChunk({
      ...ChunkIds,
      content_with_weight: 'New content',
      important_kwd: [],
      question_kwd: [],
      available_int: 1,
    });

    expect(mockRequest.post).toHaveBeenCalledWith(
      '/datasets/kb-1/documents/doc-1/chunks',
      {
        data: expect.objectContaining({
          content: 'New content',
          important_keywords: [],
          questions: [],
          available: true,
        }),
      },
    );
  });
});
