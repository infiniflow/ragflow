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

const mockPatch = (request as unknown as { patch: jest.Mock }).patch;

jest.mock('@/utils/request', () => ({
  __esModule: true,
  default: Object.assign(jest.fn(), { patch: jest.fn() }),
}));

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));

jest.mock('@/utils/api', () => ({
  __esModule: true,
  default: {
    chunkDetail: (datasetId: string, documentId: string, chunkId: string) =>
      `/datasets/${datasetId}/documents/${documentId}/chunks/${chunkId}`,
  },
}));

describe('kbService.setChunk image update mode', () => {
  beforeEach(() => {
    mockPatch.mockReset();
    mockPatch.mockResolvedValue({ data: { code: 0 } });
  });

  it('defaults to replace when a new image is uploaded', async () => {
    await kbService.setChunk({
      chunk_id: 'chunk-1',
      doc_id: 'doc-1',
      kb_id: 'kb-1',
      image_base64: 'aGVsbG8=',
    });

    expect(mockPatch).toHaveBeenCalledWith(
      '/datasets/kb-1/documents/doc-1/chunks/chunk-1',
      expect.objectContaining({
        data: expect.objectContaining({ image_update_mode: 'replace' }),
      }),
    );
  });

  it('keeps an explicitly requested image update mode', async () => {
    await kbService.setChunk({
      chunk_id: 'chunk-1',
      doc_id: 'doc-1',
      kb_id: 'kb-1',
      image_base64: 'aGVsbG8=',
      image_update_mode: 'append',
    });

    expect(mockPatch).toHaveBeenCalledWith(
      '/datasets/kb-1/documents/doc-1/chunks/chunk-1',
      expect.objectContaining({
        data: expect.objectContaining({ image_update_mode: 'append' }),
      }),
    );
  });

  it('does not ask for an image update when no image is uploaded', async () => {
    await kbService.setChunk({
      chunk_id: 'chunk-1',
      doc_id: 'doc-1',
      kb_id: 'kb-1',
      content: 'updated content',
    });

    const [, config] = mockPatch.mock.calls[0];
    expect(config.data.image_update_mode).toBeUndefined();
  });
});
