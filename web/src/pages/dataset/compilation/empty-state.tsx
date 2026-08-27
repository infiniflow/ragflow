import { CirclePause, WandSparkles } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

import { IconFontFill } from '@/components/icon-font';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import {
  GenerateType,
  IconKeyMap,
} from '@/pages/dataset/dataset/generate-button/constants';
import {
  ITraceInfo,
  useDatasetGenerate,
} from '@/pages/dataset/dataset/generate-button/hook';
import { useGenerateStatus } from '@/pages/dataset/dataset/generate-button/use-generate-status';
import { replaceText } from '@/pages/dataset/process-log-modal';
import { toFixed } from '@/utils/common-util';

type EmptyStateType = 'llm-wiki' | 'skills';

interface ICompilationEmptyStateProps {
  type: EmptyStateType;
  disabled?: boolean;
  data?: ITraceInfo;
}

const DefaultGenerateTypeMap: Record<EmptyStateType, GenerateType> = {
  'llm-wiki': GenerateType.Artifact,
  skills: GenerateType.ToSkills,
};

const TitleKeyMap: Record<EmptyStateType, string> = {
  'llm-wiki': 'knowledgeDetails.noWikiPages',
  skills: 'knowledgeDetails.noSkills',
};

const LabelKeyMap: Record<EmptyStateType, string> = {
  'llm-wiki': 'knowledgeDetails.artifact',
  skills: 'knowledgeDetails.toSkills',
};

export function CompilationEmptyState({
  type,
  disabled,
  data,
}: ICompilationEmptyStateProps) {
  const { t } = useTranslation();
  const generateType = DefaultGenerateTypeMap[type];
  const { runGenerate, pauseGenerate } = useDatasetGenerate();
  const { status, percent } = useGenerateStatus(data);

  const handleGenerate = useCallback(() => {
    runGenerate({ type: generateType }).catch(() => {});
  }, [runGenerate, generateType]);

  const handlePause = useCallback(() => {
    if (data?.id) {
      pauseGenerate({ task_id: data.id, type: generateType }).catch(() => {});
    }
  }, [pauseGenerate, data?.id, generateType]);

  const showProgress = status === 'running' || status === 'failed';

  return (
    <div className="flex-1 min-h-0 flex flex-col items-center justify-center border border-dashed border-border-button rounded-xl">
      {!showProgress ? (
        <div className="flex flex-col items-center gap-4">
          <p className="text-text-secondary text-lg">{t(TitleKeyMap[type])}</p>
          <Button
            variant="outline"
            onClick={handleGenerate}
            disabled={disabled}
          >
            <WandSparkles className="mr-2 size-4" />
            {t('knowledgeDetails.generate')}
          </Button>
        </div>
      ) : (
        <div className="w-full max-w-md p-6 flex flex-col gap-4">
          <div className="flex items-center gap-2 text-text-primary">
            <IconFontFill
              name={IconKeyMap[generateType]}
              className="text-accent-primary"
            />
            <span>{t(LabelKeyMap[type])}</span>
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
            {status === 'failed' && (
              <span
                className="text-state-error cursor-pointer"
                onClick={handleGenerate}
              >
                <IconFontFill name="reparse" className="text-accent-primary" />
              </span>
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
          <div className="whitespace-pre-line text-wrap rounded-lg max-h-[350px] overflow-y-auto scrollbar-auto p-2 bg-bg-base text-sm text-text-secondary">
            {replaceText(data?.progress_msg || '')}
          </div>
        </div>
      )}
    </div>
  );
}

export default CompilationEmptyState;
