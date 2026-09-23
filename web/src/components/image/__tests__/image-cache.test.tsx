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

import { renderHook, waitFor } from '@testing-library/react';
import {
  buildDocumentImageUrl,
  evictDocumentImage,
  useDocumentImageUrl,
} from '../index';

// A chunk keeps its img_id when its image is updated in place, so the cached
// bytes have to be reachable again by (id, documentId) and droppable.

describe('document image cache', () => {
  let fetchMock: jest.Mock;
  let createObjectUrlMock: jest.Mock;
  let revokeObjectUrlMock: jest.Mock;

  beforeEach(() => {
    fetchMock = jest.fn().mockResolvedValue({
      ok: true,
      blob: async () => new Blob(['image-bytes']),
    });
    createObjectUrlMock = jest
      .fn()
      .mockImplementation((blob: Blob) => `blob:cached-${blob.size}`);
    revokeObjectUrlMock = jest.fn();

    global.fetch = fetchMock as unknown as typeof fetch;
    URL.createObjectURL =
      createObjectUrlMock as unknown as typeof URL.createObjectURL;
    URL.revokeObjectURL =
      revokeObjectUrlMock as unknown as typeof URL.revokeObjectURL;
  });

  it('serves the cached bytes until the image is evicted', async () => {
    const id = 'kb-1-chunk-evict';
    const documentId = 'doc-1';

    const first = renderHook(() => useDocumentImageUrl(id, documentId));
    await waitFor(() => expect(first.result.current).toBeTruthy());
    expect(fetchMock).toHaveBeenCalledTimes(1);

    const second = renderHook(() => useDocumentImageUrl(id, documentId));
    await waitFor(() => expect(second.result.current).toBeTruthy());
    expect(fetchMock).toHaveBeenCalledTimes(1);

    evictDocumentImage(id, documentId);
    expect(revokeObjectUrlMock).toHaveBeenCalledTimes(1);

    renderHook(() => useDocumentImageUrl(id, documentId));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
  });

  it('fetches again when the cache-busting key changes', async () => {
    const id = 'kb-1-chunk-buster';
    const documentId = 'doc-1';

    const first = renderHook(() => useDocumentImageUrl(id, documentId, 1));
    await waitFor(() => expect(first.result.current).toBeTruthy());
    expect(fetchMock.mock.calls[0][0]).toContain('_t=1');

    // Same image, new key: the replacement bytes must be fetched.
    renderHook(() => useDocumentImageUrl(id, documentId, 2));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(fetchMock.mock.calls[1][0]).toContain('_t=2');
  });

  it('omits the cache-busting key when none is given', () => {
    expect(buildDocumentImageUrl('kb-1-chunk-plain', 'doc-1')).not.toContain(
      '_t',
    );
  });
});
