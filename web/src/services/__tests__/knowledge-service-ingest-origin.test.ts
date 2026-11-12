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

jest.mock('@/utils/request', () => ({
  __esModule: true,
  default: Object.assign(jest.fn(), { post: jest.fn() }),
}));

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: jest.fn(),
}));

jest.mock('@/utils/api', () => ({
  __esModule: true,
  default: { documentIngest: '/api/v1/documents/ingest' },
}));

const mockPost = jest.mocked(request.post);

beforeEach(() => {
  mockPost.mockClear();
});

it('leaves manual ingestion requests unchanged', async () => {
  const data = { doc_ids: ['doc-1'], run: 1, delete: true };
  await kbService.documentIngest(data);
  expect(mockPost).toHaveBeenCalledWith('/api/v1/documents/ingest', { data });
});

it.each(['automatic-stop-retry', 'automatic-upload-parse'])(
  'forwards the %s origin without changing the request body',
  async (origin) => {
    const data = { doc_ids: ['doc-1'], run: 2 };
    const headers = { 'X-RAGFlow-Action-Origin': origin };
    await kbService.documentIngest(data, { headers });
    expect(mockPost).toHaveBeenCalledWith('/api/v1/documents/ingest', {
      data,
      headers,
    });
  },
);
