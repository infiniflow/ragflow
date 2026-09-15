import { CircleX } from 'lucide-react';
import { useCallback, type MouseEvent } from 'react';
import { useTranslation } from 'react-i18next';

import { IconFontFill } from '@/components/icon-font';
import { GenerateType } from '@/constants/knowledge';
import {
  ITraceInfo,
  useDatasetGenerate,
  useGenerateStatus,
} from '@/hooks/use-dataset-generate';
import { useIsGoBackend } from '@/utils/backend-variant';
import { cn } from '@/lib/utils';
import { toFixed } from '@/utils/common-util';

type UpdateRunProgressProps = {
  data?: ITraceInfo;
  generateType: GenerateType;
};

export function UpdateRunProgress({
  data,
  generateType,
}: UpdateRunProgressProps) {
  const { t } = useTranslation();
  const { pauseGenerate } = useDatasetGenerate();
  const { status, percent } = useGenerateStatus(data);
  const isGo = useIsGoBackend();

  const handlePause = useCallback(
    (e: MouseEvent) => {
      e.stopPropagation();
      if (data?.id) {
        pauseGenerate({ task_id: data.id, type: generateType }).catch(() => {});
      }
    },
    [pauseGenerate, data?.id, generateType],
  );

  // Go: no scheduler task-level cancel, and no stable terminal state, so
  // show the MySQL inflight/backlog entry counts instead of a percentage and no
  // pause button. Error diagnostic takes priority (see plan v4.1 §4.2).
  if (isGo && status === 'running') {
    return (
      <span className="flex items-center gap-2 text-sm text-text-secondary">
        <span className="size-2 rounded-full bg-accent-primary" />
        {data?.compilationError
          ? data.compilationError
          : data?.currentPhase ||
            t('knowledgeCompilation.compiling', {
              defaultValue: 'Compiling…',
            })}
        {!data?.compilationError && (
          <span>
            {t('knowledgeCompilation.compilingCounts', {
              inflight: data?.inflight ?? 0,
              backlog: data?.backlog ?? 0,
              defaultValue: '{{inflight}} processing / {{backlog}} queued',
            })}
          </span>
        )}
      </span>
    );
  }
  if (isGo && status === 'failed') {
    return (
      <span className="flex items-center gap-2 text-sm text-state-error">
        <IconFontFill name="reparse" className="text-accent-primary" />
        {data?.compilationError || t('message.operated')}
      </span>
    );
  }

  return (
    <span className="flex items-center gap-2">
      <span className="bg-border-button h-1 w-16 rounded-full">
        <span
          className={cn('block h-1 rounded-full', {
            'bg-state-error': status === 'failed',
            'bg-accent-primary': status === 'running',
          })}
          style={{ width: `${toFixed(percent)}%` }}
        />
      </span>
      {status === 'running' && (
        <>
          <span>{(toFixed(percent) as string) + '%'}</span>
          <span
            className="text-state-error cursor-pointer"
            onClick={handlePause}
          >
            <CircleX className="size-4" />
          </span>
        </>
      )}
      {status === 'failed' && (
        <IconFontFill name="reparse" className="text-accent-primary" />
      )}
    </span>
  );
}
