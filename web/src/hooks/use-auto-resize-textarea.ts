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
    el.style.height = `${Math.min(el.scrollHeight, maxHeight)}px`;
  }, [ref, value, maxHeight]);
}
