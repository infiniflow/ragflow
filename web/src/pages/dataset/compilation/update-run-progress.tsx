import { CircleX } from 'lucide-react';
import { useCallback, type MouseEvent } from 'react';

import { IconFontFill } from '@/components/icon-font';
import { GenerateType } from '@/constants/knowledge';
import {
  ITraceInfo,
  useDatasetGenerate,
  useGenerateStatus,
} from '@/hooks/use-dataset-generate';
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
  const { pauseGenerate } = useDatasetGenerate();
  const { status, percent } = useGenerateStatus(data);

  const handlePause = useCallback(
    (e: MouseEvent) => {
      e.stopPropagation();
      if (data?.id) {
        pauseGenerate({ task_id: data.id, type: generateType }).catch(() => {});
      }
    },
    [pauseGenerate, data?.id, generateType],
  );

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
