import { CirclePause, Logs } from 'lucide-react';
import { useCallback, useEffect } from 'react';
import { useTranslation } from 'react-i18next';

import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { cn } from '@/lib/utils';
import { GenerateType } from '@/pages/dataset/dataset/generate-button/constants';
import {
  ITraceInfo,
  useDatasetGenerate,
} from '@/pages/dataset/dataset/generate-button/hook';
import { useGenerateStatus } from '@/pages/dataset/dataset/generate-button/use-generate-status';
import { replaceText } from '@/pages/dataset/process-log-modal';
import { toFixed } from '@/utils/common-util';

type WikiUpdateSheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  data?: ITraceInfo;
};

export function WikiUpdateSheet({
  open,
  onOpenChange,
  data,
}: WikiUpdateSheetProps) {
  const { t } = useTranslation();
  const { pauseGenerate } = useDatasetGenerate();
  const { status, percent } = useGenerateStatus(data);

  useEffect(() => {
    if (status === 'completed') {
      onOpenChange(false);
    }
  }, [status, onOpenChange]);

  const handlePause = useCallback(() => {
    if (data?.id) {
      pauseGenerate({ task_id: data.id, type: GenerateType.Artifact }).catch(
        () => {},
      );
    }
  }, [pauseGenerate, data?.id]);

  return (
    <Sheet open={open} onOpenChange={onOpenChange} modal={false}>
      <SheetContent className="flex flex-col">
        <SheetHeader>
          <SheetTitle>
            {t('knowledgeDetails.updateSheetTitle', {
              defaultValue: 'Update Wiki',
            })}
          </SheetTitle>
        </SheetHeader>
        <div className="flex-1 min-h-0 flex flex-col gap-4 pt-4">
          <div className="flex items-center gap-2 text-text-primary">
            <Logs className="size-5" />
            <span>
              {t('knowledgeDetails.artifact', { defaultValue: 'Artifact' })}
            </span>
          </div>
          <div className="flex items-center gap-2">
            <div
              className={cn('bg-border-button h-1 rounded-full', {
                'w-[calc(100%-100px)]': status === 'running',
                'w-[calc(100%-50px)]': status === 'failed',
              })}
            >
              <div
                className={cn('h-1 rounded-full', {
                  'bg-state-error': status === 'failed',
                  'bg-accent-primary': status === 'running',
                })}
                style={{ width: `${toFixed(percent)}%` }}
              />
            </div>
            {status === 'running' && (
              <span>{(toFixed(percent) as string) + '%'}</span>
            )}
            {status !== 'failed' && (
              <span
                className="text-state-error cursor-pointer"
                onClick={handlePause}
              >
                <CirclePause />
              </span>
            )}
          </div>
          <div className="flex-1 whitespace-pre-line text-wrap rounded-lg overflow-y-auto scrollbar-auto p-2 bg-bg-base text-sm text-text-secondary">
            {replaceText(data?.progress_msg || '')}
          </div>
        </div>
      </SheetContent>
    </Sheet>
  );
}
