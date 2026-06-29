import { cn } from '@/lib/utils';
import { escapeRegExp } from 'lodash';
import { Fragment, useMemo } from 'react';

type SearchHighlightProps = {
  text?: string | null;
  query: string;
  className?: string;
};

export function SearchHighlight({
  text,
  query,
  className,
}: SearchHighlightProps) {
  const safeText = text ?? '';

  const parts = useMemo(() => {
    const trimmedQuery = query.trim();
    if (!trimmedQuery) {
      return [{ text: safeText, highlight: false }];
    }

    const regex = new RegExp(`(${escapeRegExp(trimmedQuery)})`, 'gi');
    return safeText.split(regex).map((part) => ({
      text: part,
      highlight: part.toLowerCase() === trimmedQuery.toLowerCase(),
    }));
  }, [safeText, query]);

  return (
    <span className={cn('inline', className)}>
      {parts.map((part, index) =>
        part.highlight ? (
          <mark
            key={index}
            className="rounded-sm bg-accent-primary/25 text-inherit"
          >
            {part.text}
          </mark>
        ) : (
          <Fragment key={index}>{part.text}</Fragment>
        ),
      )}
    </span>
  );
}
