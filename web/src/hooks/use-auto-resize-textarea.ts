import { useLayoutEffect, type RefObject } from 'react';

/**
 * Auto-resize a textarea to fit its content, clamped to a maximum height.
 *
 * useLayoutEffect runs synchronously after DOM mutation but before the
 * browser paints, so the height adjustment never produces a visible flicker.
 */
export function useAutoResizeTextarea(
  ref: RefObject<HTMLTextAreaElement | null>,
  value: string,
  maxHeight = 160,
) {
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.style.height = 'auto';
    // scrollHeight excludes borders while height is border-box, so the
    // content box ends up a couple of pixels short and overflow-y-auto
    // would show a scrollbar even for a single line. Hide it until the
    // content actually exceeds the max height.
    const overflowing = el.scrollHeight > maxHeight;
    el.style.overflowY = overflowing ? 'auto' : 'hidden';
    el.style.height = `${Math.min(el.scrollHeight, maxHeight)}px`;
  }, [ref, value, maxHeight]);
}
