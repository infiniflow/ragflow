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

import request from '@/utils/next-request';

import { applyOntologyFixes } from './compilation-template-group-service';

jest.mock('@/utils/next-request', () => ({
  __esModule: true,
  default: { post: jest.fn() },
}));

describe('applyOntologyFixes', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  // The axios instance resolves with the whole response, so the envelope lives on
  // `.data`. A caller reading `code` off the response itself sees `undefined`,
  // treats a successful write as refused — and then skips both the re-compile and
  // the refresh, so the reader keeps seeing the lines that were already applied.
  it('resolves with the envelope, not with the axios response', async () => {
    (request.post as jest.Mock).mockResolvedValue({
      status: 200,
      data: { code: 0, message: 'success' },
    });

    const envelope = await applyOntologyFixes('tpl-1', [
      { op: 'widen', property: 'part_of', side: 'range', class: 'work' },
    ]);

    expect(envelope).toEqual({ code: 0, message: 'success' });
    expect(envelope.code).toBe(0);
  });

  // One write for the whole selection: the expensive step is the re-compile that
  // follows, so the selection must not become one request per line.
  it('sends every selected edit in a single request', async () => {
    (request.post as jest.Mock).mockResolvedValue({ data: { code: 0 } });

    await applyOntologyFixes('tpl-1', [
      { op: 'widen', property: 'part_of', side: 'range', class: 'work' },
      { op: 'declare', property: 'genre', domain: 'work', range: 'concept' },
    ]);

    expect(request.post).toHaveBeenCalledTimes(1);
    const [url, body] = (request.post as jest.Mock).mock.calls[0];
    expect(url).toContain('tpl-1');
    expect(body).toEqual({
      fixes: [
        { op: 'widen', property: 'part_of', side: 'range', class: 'work' },
        { op: 'declare', property: 'genre', domain: 'work', range: 'concept' },
      ],
    });
  });

  // The reader has to see which edit was refused and why: the panel prints this
  // message, and a lost message turns an actionable refusal into a dead button.
  it('keeps the reason the write was refused', async () => {
    (request.post as jest.Mock).mockResolvedValue({
      data: {
        code: 500,
        message: 'edit 2 (genre): the template already declares genre.',
      },
    });

    const envelope = await applyOntologyFixes('tpl-1', []);

    expect(envelope.code).toBe(500);
    expect(envelope.message).toContain('already declares genre');
  });
});
