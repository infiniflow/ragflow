import { CircleX } from 'lucide-react';
import { useCallback, type MouseEvent } from 'react';

import { IconFontFill } from '@/components/icon-font';
import { cn } from '@/lib/utils';
import { GenerateType } from '@/pages/dataset/dataset/generate-button/constants';
import {
  ITraceInfo,
  useDatasetGenerate,
} from '@/pages/dataset/dataset/generate-button/hook';
import { useGenerateStatus } from '@/pages/dataset/dataset/generate-button/use-generate-status';
import { toFixed } from '@/utils/common-util';

type WikiUpdateProgressProps = {
  data?: ITraceInfo;
};

export function WikiUpdateProgress({ data }: WikiUpdateProgressProps) {
  const { pauseGenerate } = useDatasetGenerate();
  const { status, percent } = useGenerateStatus(data);

  const handlePause = useCallback(
    (e: MouseEvent) => {
      e.stopPropagation();
      if (data?.id) {
        pauseGenerate({ task_id: data.id, type: GenerateType.Artifact }).catch(
          () => {},
        );
      }
    },
    [pauseGenerate, data?.id],
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
