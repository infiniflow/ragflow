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
  // Trackers live for the lifetime of the page and are not reset between
  // tests, so every test uses its own document ids.
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('keeps the fast interval while the cancel is fresh', () => {
    markCancelRequested(['doc-fresh']);

    expect(getCancelRequestInterval(['doc-fresh'])).toBe(500);

    advance(CANCEL_STOP_LOSS_MS - 1);

    expect(getCancelRequestInterval(['doc-fresh'])).toBe(500);
  });

  it('falls back to the 5s interval once the cancel is overdue', () => {
    markCancelRequested(['doc-overdue']);

    advance(CANCEL_STOP_LOSS_MS);

    expect(getCancelRequestInterval(['doc-overdue'])).toBe(5000);
  });

  it('keeps the fast interval while any stopping document is still fresh', () => {
    markCancelRequested(['doc-mixed-old']);

    advance(CANCEL_STOP_LOSS_MS);
    markCancelRequested(['doc-mixed-new']);

    expect(getCancelRequestInterval(['doc-mixed-old', 'doc-mixed-new'])).toBe(
      500,
    );
  });

  it('reports an overdue cancel once, then latches the retry', () => {
    markCancelRequested(['doc-latched']);

    expect(observeStoppingDocuments(['doc-latched'], ['doc-latched'])).toEqual(
      [],
    );

    advance(CANCEL_STOP_LOSS_MS);

    expect(observeStoppingDocuments(['doc-latched'], ['doc-latched'])).toEqual([
      'doc-latched',
    ]);
    expect(observeStoppingDocuments(['doc-latched'], ['doc-latched'])).toEqual(
      [],
    );
  });

  it('retries a cancel again only after the user re-arms it', () => {
    markCancelRequested(['doc-rearmed']);

    advance(CANCEL_STOP_LOSS_MS);
    expect(observeStoppingDocuments(['doc-rearmed'], ['doc-rearmed'])).toEqual([
      'doc-rearmed',
    ]);
    advance(CANCEL_STOP_LOSS_MS);
    expect(observeStoppingDocuments(['doc-rearmed'], ['doc-rearmed'])).toEqual(
      [],
    );

    markCancelRequested(['doc-rearmed']);
    advance(CANCEL_STOP_LOSS_MS);

    expect(observeStoppingDocuments(['doc-rearmed'], ['doc-rearmed'])).toEqual([
      'doc-rearmed',
    ]);
  });

  it('never retries a stopping document first seen without a local cancel', () => {
    // Page reload: the document is already stopping, nothing was clicked here.
    expect(observeStoppingDocuments(['doc-adopted'], ['doc-adopted'])).toEqual(
      [],
    );

    // The window starts at the first sight, so polling stays fast ...
    expect(getCancelRequestInterval(['doc-adopted'])).toBe(500);

    // ... then downgrades once the window has elapsed, and never retries.
    advance(CANCEL_STOP_LOSS_MS);

    expect(getCancelRequestInterval(['doc-adopted'])).toBe(5000);
    expect(observeStoppingDocuments(['doc-adopted'], ['doc-adopted'])).toEqual(
      [],
    );
  });

  it('only collects the overdue ids out of a mixed batch', () => {
    markCancelRequested(['doc-old']);

    advance(CANCEL_STOP_LOSS_MS);
    markCancelRequested(['doc-new']);

    expect(
      observeStoppingDocuments(['doc-old', 'doc-new'], ['doc-old', 'doc-new']),
    ).toEqual(['doc-old']);

    advance(CANCEL_STOP_LOSS_MS);

    expect(
      observeStoppingDocuments(['doc-old', 'doc-new'], ['doc-old', 'doc-new']),
    ).toEqual(['doc-new']);
  });

  it('drops tracking once a document is observed to leave the stopping state', () => {
    markCancelRequested(['doc-resumed']);

    advance(CANCEL_STOP_LOSS_MS);
    // Observed as running again: the cancel is over, drop its trackers.
    observeStoppingDocuments(['doc-resumed'], []);

    // Back to stopping later: treated as a fresh observation, not a retry of
    // the old cancel.
    expect(observeStoppingDocuments(['doc-resumed'], ['doc-resumed'])).toEqual(
      [],
    );
    expect(getCancelRequestInterval(['doc-resumed'])).toBe(500);
  });

  it('keeps tracking a cancel on a document missing from the current result', () => {
    markCancelRequested(['doc-offpage']);

    // The list moved to another page / search / filter: the document is not
    // part of the current result at all.
    advance(CANCEL_STOP_LOSS_MS - 1);
    observeStoppingDocuments(['doc-elsewhere'], ['doc-elsewhere']);

    // Once it shows up again still stopping, the cancel is overdue and the
    // retry was preserved.
    advance(1);

    expect(observeStoppingDocuments(['doc-offpage'], ['doc-offpage'])).toEqual([
      'doc-offpage',
    ]);
  });
});
