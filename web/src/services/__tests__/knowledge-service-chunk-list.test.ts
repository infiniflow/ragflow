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

const mockGet = (request as unknown as { get: jest.Mock }).get;

jest.mock('@/utils/request', () => ({
  __esModule: true,
  default: Object.assign(jest.fn(), { get: jest.fn() }),
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
  },
}));

const sentParams = () => mockGet.mock.calls[0][1].params;

describe('kbService.chunkList id filter', () => {
  beforeEach(() => {
    mockGet.mockReset();
    mockGet.mockResolvedValue({
      data: { code: 0, data: { chunks: [], doc: {} } },
    });
  });

  it('sends the ids as one comma-separated query value', async () => {
    // axios turns an array into `chunk_ids[]=a&chunk_ids[]=b`, and the server
    // only reads the bare `chunk_ids` name, so the list has to arrive as one
    // value it can split itself.
    await kbService.chunkList({
      kb_id: 'kb-1',
      doc_id: 'doc-1',
      chunk_ids: ['0123456789abcdef', 'fedcba9876543210'],
    });

    expect(sentParams().chunk_ids).toBe('0123456789abcdef,fedcba9876543210');
  });

  it('leaves a single id unchanged', async () => {
    await kbService.chunkList({
      kb_id: 'kb-1',
      doc_id: 'doc-1',
      chunk_ids: '0123456789abcdef',
    });

    expect(sentParams().chunk_ids).toBe('0123456789abcdef');
  });

  it('omits an empty id list instead of sending an empty filter', async () => {
    await kbService.chunkList({
      kb_id: 'kb-1',
      doc_id: 'doc-1',
      chunk_ids: [],
    });

    expect(sentParams().chunk_ids).toBeUndefined();
  });

  it('sends the one id prefix as a plain query value', async () => {
    // One prefix per request goes out as typed, never comma-joined.
    await kbService.chunkList({
      kb_id: 'kb-1',
      doc_id: 'doc-1',
      chunk_id_prefix: '6877',
    });

    expect(sentParams().chunk_id_prefix).toBe('6877');
  });

  it('omits an empty prefix instead of sending an empty filter', async () => {
    await kbService.chunkList({
      kb_id: 'kb-1',
      doc_id: 'doc-1',
      chunk_id_prefix: '',
    });

    expect(sentParams().chunk_id_prefix).toBeUndefined();
  });
});
