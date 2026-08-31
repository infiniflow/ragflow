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

import { useLayoutEffect, useState, type RefObject } from 'react';

/**
 * Auto-resize a textarea to fit its content, clamped to a maximum height.
 *
 * useLayoutEffect runs synchronously after DOM mutation but before the
 * browser paints, so the height adjustment never produces a visible flicker.
 *
 * Measurement also re-runs when the element's content-box width changes
 * (e.g. responsive layout or font-metric changes): soft-wrapping can add or
 * remove lines without `value` changing, which would otherwise leave
 * `isMultiLine` stale and misposition controls. A ResizeObserver watches
 * width only; height-only changes (which we cause ourselves when setting
 * `el.style.height`) are ignored to avoid feedback loops.
 *
 * Returns `isMultiLine` - true when the rendered content spans more than one
 * line (including soft-wrapped lines), derived from scrollHeight vs. the
 * computed single-line height.
 */
export function useAutoResizeTextarea(
  ref: RefObject<HTMLTextAreaElement | null>,
  value: string,
  maxHeight = 160,
) {
  const [isMultiLine, setIsMultiLine] = useState(false);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;

    const measure = () => {
      el.style.height = 'auto';
      // scrollHeight excludes borders while height is border-box, so the
      // content box ends up a couple of pixels short and overflow-y-auto
      // would show a scrollbar even for a single line. Hide it until the
      // content actually exceeds the max height.
      const scrollHeight = el.scrollHeight;
      const overflowing = scrollHeight > maxHeight;
      el.style.overflowY = overflowing ? 'auto' : 'hidden';
      el.style.height = `${Math.min(scrollHeight, maxHeight)}px`;

      const style = getComputedStyle(el);
      const lineHeight =
        parseFloat(style.lineHeight) || parseFloat(style.fontSize) * 1.2;
      const verticalPadding =
        parseFloat(style.paddingTop) + parseFloat(style.paddingBottom);
      setIsMultiLine(scrollHeight > verticalPadding + lineHeight + 1);
    };

    measure();

    let lastWidth: number | undefined;
    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const width = entry.contentRect.width;
        if (width === lastWidth) continue;
        lastWidth = width;
        measure();
      }
    });
    ro.observe(el);

    return () => ro.disconnect();
  }, [ref, value, maxHeight]);

  return isMultiLine;
}
