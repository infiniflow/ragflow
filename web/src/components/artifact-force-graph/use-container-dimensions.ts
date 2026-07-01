import { useEffect, useState, type RefObject } from 'react';

export function useContainerDimensions(
  ref: RefObject<HTMLElement | null>,
  enabled: boolean = true,
) {
  const [dimensions, setDimensions] = useState({ width: 0, height: 0 });

  useEffect(() => {
    if (!ref.current || !enabled) return;

    const observer = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (entry) {
        const { width, height } = entry.contentRect;
        setDimensions({ width, height });
      }
    });

    observer.observe(ref.current);
    return () => observer.disconnect();
  }, [ref, enabled]);

  return dimensions;
}
