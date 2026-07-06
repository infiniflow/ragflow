import { IconFontFill } from '@/components/icon-font';
import { DropdownMenuItem } from '@/components/ui/dropdown-menu';
import { cn } from '@/lib/utils';
import { toFixed } from '@/utils/common-util';
import { t } from 'i18next';
import { lowerFirst } from 'lodash';
import { CirclePause } from 'lucide-react';

import { replaceText } from '../../process-log-modal';
import { GenerateStatus, GenerateType, IconKeyMap } from './constants';
import { ITraceInfo, useDatasetGenerate } from './hook';
import { useGenerateStatus } from './use-generate-status';

type DatasetGenerateReturn = ReturnType<typeof useDatasetGenerate>;

interface IMenuItemProps {
  name: GenerateType;
  data: ITraceInfo;
  pauseGenerate: DatasetGenerateReturn['pauseGenerate'];
  runGenerate: DatasetGenerateReturn['runGenerate'];
}

function MenuItem({
  name: type,
  runGenerate,
  data,
  pauseGenerate,
}: IMenuItemProps) {
  const { status, percent } = useGenerateStatus(data);

  return (
    <DropdownMenuItem
      className={cn(
        'border cursor-pointer p-2 rounded-md focus:bg-transparent',
        {
          'hover:border-accent-primary hover:bg-[rgba(59,160,92,0.1)] focus:bg-[rgba(59,160,92,0.1)]':
            status === GenerateStatus.start ||
            status === GenerateStatus.completed,
          'hover:border-border hover:bg-[rgba(59,160,92,0)] focus:bg-[rgba(59,160,92,0)]':
            status !== GenerateStatus.start &&
            status !== GenerateStatus.completed,
        },
      )}
      onSelect={(e) => {
        e.preventDefault();
      }}
      onClick={(e) => {
        e.stopPropagation();
      }}
    >
      <div
        className="flex items-start gap-2 flex-col w-full"
        onClick={() => {
          if (
            status === GenerateStatus.start ||
            status === GenerateStatus.completed
          ) {
            runGenerate({ type });
          }
        }}
      >
        <div className="flex justify-start text-text-primary items-center gap-2">
          <IconFontFill
            name={IconKeyMap[type]}
            className="text-accent-primary"
          />
          {t(`knowledgeDetails.${lowerFirst(type)}`)}
        </div>
        {(status === GenerateStatus.start ||
          status === GenerateStatus.completed) && (
          <div className="text-text-secondary text-sm">
            {t(`knowledgeDetails.generate${type}`)}
          </div>
        )}
        {(status === GenerateStatus.running ||
          status === GenerateStatus.failed) && (
          <div className="flex justify-between items-center w-full px-2.5 py-1">
            <div
              className={cn(' bg-border-button h-1 rounded-full', {
                'w-[calc(100%-100px)]': status === GenerateStatus.running,
                'w-[calc(100%-50px)]': status === GenerateStatus.failed,
              })}
            >
              <div
                className={cn('h-1 rounded-full', {
                  'bg-state-error': status === GenerateStatus.failed,
                  'bg-accent-primary': status === GenerateStatus.running,
                })}
                style={{ width: `${toFixed(percent)}%` }}
              ></div>
            </div>
            {status === GenerateStatus.running && (
              <span>{(toFixed(percent) as string) + '%'}</span>
            )}
            {status === GenerateStatus.failed && (
              <span
                className="text-state-error"
                onClick={(e) => {
                  e.stopPropagation();
                  runGenerate({ type });
                }}
              >
                <IconFontFill name="reparse" className="text-accent-primary" />
              </span>
            )}
            {status !== GenerateStatus.failed && (
              <span
                className="text-state-error"
                onClick={(e) => {
                  e.stopPropagation();
                  pauseGenerate({ task_id: data.id, type });
                }}
              >
                <CirclePause />
              </span>
            )}
          </div>
        )}
        {status !== GenerateStatus.start &&
          status !== GenerateStatus.completed && (
            <div className="w-full  whitespace-pre-line text-wrap rounded-lg h-fit max-h-[350px] overflow-y-auto scrollbar-auto px-2.5 py-1">
              {replaceText(data?.progress_msg || '')}
            </div>
          )}
      </div>
    </DropdownMenuItem>
  );
}

export default MenuItem;
