import { useLayoutEffect, useState, type RefObject } from 'react';

/**
 * Auto-resize a textarea to fit its content, clamped to a maximum height.
 *
 * useLayoutEffect runs synchronously after DOM mutation but before the
 * browser paints, so the height adjustment never produces a visible flicker.
 *
 * Returns `isMultiLine` — true when the rendered content spans more than one
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
  }, [ref, value, maxHeight]);

  return isMultiLine;
}
