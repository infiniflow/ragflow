import { useLayoutEffect, useRef, useState } from 'react';

const LAYOUT_GAP = 48;
const FIT_BUFFER = 16;

export function useHeaderNavLayout(measureKey = '') {
  const headerRef = useRef<HTMLElement>(null);
  const logoRef = useRef<HTMLDivElement>(null);
  const expandedRightMeasureRef = useRef<HTMLDivElement>(null);
  const navMeasureRef = useRef<HTMLDivElement>(null);
  const [isCompact, setIsCompact] = useState(true);

  useLayoutEffect(() => {
    const measure = () => {
      const header = headerRef.current;
      const logo = logoRef.current;
      const expandedRight = expandedRightMeasureRef.current;
      const nav = navMeasureRef.current;

      if (!header || !logo || !expandedRight || !nav) {
        return;
      }

      const navWidth = nav.scrollWidth;
      const availableForDesktop =
        header.clientWidth -
        logo.offsetWidth -
        expandedRight.offsetWidth -
        LAYOUT_GAP;

      setIsCompact(navWidth + FIT_BUFFER > availableForDesktop);
    };

    measure();

    const observer = new ResizeObserver(measure);
    [
      headerRef.current,
      logoRef.current,
      expandedRightMeasureRef.current,
      navMeasureRef.current,
    ].forEach((node) => {
      if (node) {
        observer.observe(node);
      }
    });

    return () => observer.disconnect();
  }, [measureKey]);

  return {
    headerRef,
    logoRef,
    expandedRightMeasureRef,
    navMeasureRef,
    isCompact,
  };
}
