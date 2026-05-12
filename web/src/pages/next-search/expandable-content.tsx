import { Button } from '@/components/ui/button';
import { ChevronDown, ChevronUp } from 'lucide-react';
import { ReactNode, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

interface ExpandableContentProps {
  children: ReactNode;
  maxHeight?: number;
  className?: string;
}

export default function ExpandableContent({
  children,
  maxHeight = 208, // 52 * 4 = 208px (max-h-52)
  className = '',
}: ExpandableContentProps) {
  const { t } = useTranslation();
  const contentRef = useRef<HTMLDivElement>(null);
  const [isExpanded, setIsExpanded] = useState(false);
  const [isOverflowing, setIsOverflowing] = useState(false);

  useEffect(() => {
    const element = contentRef.current;
    if (!element) return;

    const checkOverflow = () => {
      setIsOverflowing(element.scrollHeight > maxHeight);
    };

    checkOverflow();

    // ResizeObserver handles element size changes
    const resizeObserver = new ResizeObserver(checkOverflow);
    resizeObserver.observe(element);

    // MutationObserver handles DOM changes (useful for async markdown content)
    const mutationObserver = new MutationObserver(checkOverflow);
    mutationObserver.observe(element, {
      childList: true,
      subtree: true,
      characterData: true,
    });

    // Listen for images to load (they affect scrollHeight)
    const images = element.querySelectorAll('img');
    const imageLoadPromises = Array.from(images).map(
      (img) =>
        new Promise<void>((resolve) => {
          if (img.complete) {
            resolve();
          } else {
            img.addEventListener('load', () => resolve(), { once: true });
            img.addEventListener('error', () => resolve(), { once: true });
          }
        }),
    );

    // Re-check after all images are loaded
    Promise.all(imageLoadPromises).then(checkOverflow);

    return () => {
      resizeObserver.disconnect();
      mutationObserver.disconnect();
    };
  }, [maxHeight, children]);

  const toggleExpand = () => {
    setIsExpanded(!isExpanded);
  };

  return (
    <div className="relative">
      <div
        ref={contentRef}
        className={`overflow-hidden scrollbar-thin transition-all duration-300 ${className}`}
        style={{
          maxHeight: isExpanded ? contentRef.current?.scrollHeight : maxHeight,
        }}
      >
        {children}
      </div>

      {!isExpanded && isOverflowing && (
        <div className="absolute bottom-0 left-0 right-0 h-20 bg-gradient-to-t from-bg-base to-transparent pointer-events-none" />
      )}

      {isOverflowing && (
        <Button
          size="sm"
          onClick={toggleExpand}
          className="absolute bottom-2 left-1/2 -translate-x-1/2"
        >
          {isExpanded ? (
            <>
              <ChevronUp size={14} />
              <span>{t('common.viewLess')}</span>
            </>
          ) : (
            <>
              <ChevronDown size={14} />
              <span>{t('common.viewMore')}</span>
            </>
          )}
        </Button>
      )}
    </div>
  );
}
