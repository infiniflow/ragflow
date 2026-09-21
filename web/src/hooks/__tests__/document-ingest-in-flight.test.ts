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

import { sendDocumentIngest } from '../document-ingest-in-flight';

describe('sendDocumentIngest', () => {
  it('shares the request between identical calls regardless of id order', async () => {
    const request = jest.fn(() => Promise.resolve('done'));

    const first = sendDocumentIngest(
      { documentIds: ['doc-shared', 'doc-other'], run: 2 },
      request,
    );
    const second = sendDocumentIngest(
      { documentIds: ['doc-other', 'doc-shared'], run: 2 },
      request,
    );

    expect(request).toHaveBeenCalledTimes(1);
    expect(second).toBe(first);
    await expect(first).resolves.toBe('done');
  });

  it('does not reuse the entry of a settled request', async () => {
    const params = { documentIds: ['doc-settled'], run: 2 };
    await sendDocumentIngest(params, () => Promise.resolve('done'));

    const request = jest.fn(() => Promise.resolve('done'));
    await sendDocumentIngest(params, request);

    expect(request).toHaveBeenCalledTimes(1);
  });

  it('issues a new request for a forced call while an identical one is pending', async () => {
    let resolveFirst!: (value: string) => void;
    let resolveRetry!: (value: string) => void;
    const params = { documentIds: ['doc-forced'], run: 2 };
    const first = sendDocumentIngest(
      params,
      () => new Promise<string>((resolve) => (resolveFirst = resolve)),
    );

    const request = jest.fn(
      () => new Promise<string>((resolve) => (resolveRetry = resolve)),
    );
    const retry = sendDocumentIngest(params, request, { force: true });

    expect(request).toHaveBeenCalledTimes(1);
    expect(retry).not.toBe(first);

    // Settling the original request must not evict the pending retry from
    // the key.
    resolveFirst('first');
    await first;

    const third = sendDocumentIngest(params, () => Promise.resolve('third'));
    expect(third).toBe(retry);

    resolveRetry('retry');
    await expect(retry).resolves.toBe('retry');
  });
});
