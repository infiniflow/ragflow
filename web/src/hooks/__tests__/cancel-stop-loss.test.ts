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

import {
  CANCEL_STOP_LOSS_MS,
  getCancelRequestInterval,
  markCancelRequested,
  observeStoppingDocuments,
} from '../cancel-stop-loss';

const advance = (ms: number) => jest.advanceTimersByTime(ms);

describe('cancel stop-loss', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    observeStoppingDocuments([]); // drop trackers left by a previous test
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('keeps the 1s interval while the cancel is fresh', () => {
    markCancelRequested(['doc-1']);

    expect(getCancelRequestInterval(['doc-1'])).toBe(1000);

    advance(CANCEL_STOP_LOSS_MS - 1);

    expect(getCancelRequestInterval(['doc-1'])).toBe(1000);
  });

  it('falls back to the 5s interval once the cancel is overdue', () => {
    markCancelRequested(['doc-1']);

    advance(CANCEL_STOP_LOSS_MS);

    expect(getCancelRequestInterval(['doc-1'])).toBe(5000);
  });

  it('keeps the 1s interval while any stopping document is still fresh', () => {
    markCancelRequested(['doc-1']);

    advance(CANCEL_STOP_LOSS_MS);
    markCancelRequested(['doc-2']);

    expect(getCancelRequestInterval(['doc-1', 'doc-2'])).toBe(1000);
  });

  it('reports an overdue cancel once, then latches the retry', () => {
    markCancelRequested(['doc-1']);

    expect(observeStoppingDocuments(['doc-1'])).toEqual([]);

    advance(CANCEL_STOP_LOSS_MS);

    expect(observeStoppingDocuments(['doc-1'])).toEqual(['doc-1']);
    expect(observeStoppingDocuments(['doc-1'])).toEqual([]);
  });

  it('retries a cancel again only after the user re-arms it', () => {
    markCancelRequested(['doc-1']);

    advance(CANCEL_STOP_LOSS_MS);
    expect(observeStoppingDocuments(['doc-1'])).toEqual(['doc-1']);
    advance(CANCEL_STOP_LOSS_MS);
    expect(observeStoppingDocuments(['doc-1'])).toEqual([]);

    markCancelRequested(['doc-1']);
    advance(CANCEL_STOP_LOSS_MS);

    expect(observeStoppingDocuments(['doc-1'])).toEqual(['doc-1']);
  });

  it('never retries a stopping document first seen without a local cancel', () => {
    // Page reload: the document is already stopping, nothing was clicked here.
    expect(observeStoppingDocuments(['doc-1'])).toEqual([]);

    // The window starts at the first sight, so polling stays fast ...
    expect(getCancelRequestInterval(['doc-1'])).toBe(1000);

    // ... then downgrades once the window has elapsed, and never retries.
    advance(CANCEL_STOP_LOSS_MS);

    expect(getCancelRequestInterval(['doc-1'])).toBe(5000);
    expect(observeStoppingDocuments(['doc-1'])).toEqual([]);
  });

  it('only collects the overdue ids out of a mixed batch', () => {
    markCancelRequested(['doc-old']);

    advance(CANCEL_STOP_LOSS_MS);
    markCancelRequested(['doc-new']);

    expect(observeStoppingDocuments(['doc-old', 'doc-new'])).toEqual([
      'doc-old',
    ]);

    advance(CANCEL_STOP_LOSS_MS);

    expect(observeStoppingDocuments(['doc-old', 'doc-new'])).toEqual([
      'doc-new',
    ]);
  });

  it('drops tracking once a document leaves the stopping state', () => {
    markCancelRequested(['doc-1']);

    advance(CANCEL_STOP_LOSS_MS);
    observeStoppingDocuments([]);

    // Back to stopping later: treated as a fresh observation, not a retry of
    // the old cancel.
    expect(observeStoppingDocuments(['doc-1'])).toEqual([]);
    expect(getCancelRequestInterval(['doc-1'])).toBe(1000);
  });
});
