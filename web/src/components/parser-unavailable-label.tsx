import { cn } from '@/lib/utils';
import { TriangleAlert } from 'lucide-react';
import { useTranslation } from 'react-i18next';

// Shared "unavailable" label for a saved parser binding whose pipeline or
// pinned version was deleted: ParserTreeSelect renders it for a missing
// select value, the dataset file row for a document that can no longer parse.
// The text color comes from the caller (TreeSelect's trigger paints a missing
// value with text-text-secondary on its own).
export function ParserUnavailableLabel({ className }: { className?: string }) {
  const { t } = useTranslation();
  return (
    <span className={cn('flex items-center gap-1.5', className)}>
      <TriangleAlert className="size-4 flex-shrink-0" />
      <span className="truncate">
        {t('knowledgeConfiguration.parserOptionUnavailable')}
      </span>
    </span>
  );
}
