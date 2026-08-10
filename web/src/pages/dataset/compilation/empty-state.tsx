import { CircleX, WandSparkles } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

import { IconFontFill } from '@/components/icon-font';
import { Button } from '@/components/ui/button';
import {
  ITraceInfo,
  useDatasetGenerate,
  useGenerateStatus,
} from '@/hooks/use-dataset-generate';
import { isGoDatasetBackend } from '@/utils/api-proxy-scheme';

import {
  GenerableViewMode,
  ViewMode,
  ViewModeGenerateTypeMap,
  ViewModeLabelKeyMap,
} from './constants';
import { ProgressLogPanel } from './progress-log-panel';
import { ProgressRing } from './progress-ring';

type EmptyStateType = GenerableViewMode;

interface ICompilationEmptyStateProps {
  type: EmptyStateType;
  disabled?: boolean;
  data?: ITraceInfo;
}

const TitleKeyMap: Record<EmptyStateType, string> = {
  [ViewMode.LlmWiki]: 'knowledgeDetails.noWikiPages',
  [ViewMode.Skills]: 'knowledgeDetails.noSkills',
  [ViewMode.Graph]: 'knowledgeDetails.noStructureGraph',
  [ViewMode.MindMap]: 'knowledgeDetails.noStructureMindmap',
  [ViewMode.Timeline]: 'knowledgeDetails.noStructureTimeline',
  // [ViewMode.SessionEssence]: 'knowledgeDetails.noStructureSessionEssence',
  // [ViewMode.SessionGraph]: 'knowledgeDetails.noStructureSessionGraph',
};

export function CompilationEmptyState({
  type,
  disabled,
  data,
}: ICompilationEmptyStateProps) {
  const { t } = useTranslation();
  const generateType = ViewModeGenerateTypeMap[type];
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
  const isGo = isGoDatasetBackend();

  return (
    <div className="flex-1 min-h-0 flex flex-col items-center justify-center border border-dashed border-border-button rounded-xl">
      {!showProgress ? (
        <div className="flex flex-col items-center gap-4">
          <p className="text-text-secondary text-lg">{t(TitleKeyMap[type])}</p>
          {!isGo && (
            <Button
              variant="outline"
              onClick={handleGenerate}
              disabled={disabled}
            >
              <WandSparkles className="mr-2 size-4" />
              {t('knowledgeDetails.generate')}
            </Button>
          )}
          {isGo && (
            <p className="text-sm text-text-secondary">
              {t('knowledgeDetails.autoCompiled')}
            </p>
          )}
        </div>
      ) : (
        <div className="grid h-full w-full grid-cols-[1fr_auto_1fr] items-center gap-8 p-6">
          <div />
          <div className="flex flex-col items-center gap-5">
            {isGo ? (
              // Go/hybrid: no stable percentage and no scheduler cancel, so show
              // the MySQL inflight/backlog counts (or the error diagnostic).
              status === 'failed' ? (
                <div className="flex flex-col items-center gap-2 text-state-error">
                  <IconFontFill name="reparse" className="size-8" />
                  <span className="text-text-primary">
                    {data?.compilationError || t('message.operated')}
                  </span>
                </div>
              ) : (
                <div className="flex flex-col items-center gap-2 text-text-secondary">
                  <span className="text-4xl font-medium text-accent-primary">
                    {t('knowledgeDetails.compiling', {
                      defaultValue: 'Compiling…',
                    })}
                  </span>
                  <span>
                    {t('knowledgeDetails.compilingCounts', {
                      inflight: data?.inflight ?? 0,
                      backlog: data?.backlog ?? 0,
                      defaultValue:
                        '{{inflight}} processing / {{backlog}} queued',
                    })}
                  </span>
                </div>
              )
            ) : (
              <ProgressRing percent={percent} failed={status === 'failed'} />
            )}
            <div className="flex items-center gap-2 text-text-primary">
              <span>{t(ViewModeLabelKeyMap[type])}</span>
              {!isGo && status === 'failed' && (
                <span className="cursor-pointer" onClick={handleGenerate}>
                  <IconFontFill
                    name="reparse"
                    className="text-accent-primary"
                  />
                </span>
              )}
              {!isGo && status !== 'failed' && (
                <span
                  className="text-state-error cursor-pointer"
                  onClick={handlePause}
                >
                  <CircleX className="size-5 text-state-error" />
                </span>
              )}
            </div>
          </div>
          <div className="flex min-h-0 h-full justify-end">
            <ProgressLogPanel progressMsg={data?.progress_msg} />
          </div>
        </div>
      )}
    </div>
  );
}

export default CompilationEmptyState;
