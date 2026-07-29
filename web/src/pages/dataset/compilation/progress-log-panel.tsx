import { Logs } from 'lucide-react';
import { useLayoutEffect, useMemo, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { cn } from '@/lib/utils';

import { parseProgressMsg } from './utils/parse-progress-msg';

interface IProgressLogPanelProps {
  progressMsg?: string;
  className?: string;
}

export function ProgressLogPanel({
  progressMsg,
  className,
}: IProgressLogPanelProps) {
  const { t } = useTranslation();
  const listRef = useRef<HTMLDivElement>(null);
  const entries = useMemo(() => parseProgressMsg(progressMsg), [progressMsg]);

  // Depend on progressMsg itself (not entries.length): backend head-trimming
  // can keep the line count unchanged while the content has changed.
  useLayoutEffect(() => {
    const el = listRef.current;
    if (el) {
      el.scrollTop = el.scrollHeight;
    }
  }, [progressMsg]);

  return (
    <div className={cn('flex min-h-0 flex-col gap-2', className)}>
      <div className="flex items-center gap-2 text-text-primary">
        <Logs className="size-5" />
        <span>{t('knowledgeDetails.log')}</span>
      </div>
      <div
        ref={listRef}
        className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto scrollbar-auto"
      >
        {entries.map((entry) => (
          <div
            key={entry.key}
            className="flex items-center gap-2 rounded-lg border border-border-button bg-bg-card px-3 py-2"
          >
            <span
              className={cn(
                'size-2 shrink-0 rounded-full',
                entry.isError ? 'bg-state-error' : 'bg-state-success',
              )}
            />
            <span className="break-all text-sm text-text-primary">
              {entry.text}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
