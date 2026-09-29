import { useTranslation } from 'react-i18next';

import { ITraceInfo, useGenerateStatus } from '@/hooks/use-dataset-generate';

import { GenerableViewMode, ViewMode, ViewModeLabelKeyMap } from './constants';
import { CompileProgressBoard } from './compile-progress-board';

type EmptyStateType = GenerableViewMode;

interface ICompilationEmptyStateProps {
  type: EmptyStateType;
  data?: ITraceInfo;
}

const TitleKeyMap: Record<EmptyStateType, string> = {
  [ViewMode.LlmWiki]: 'knowledgeCompilation.noWikiPages',
  [ViewMode.Skills]: 'knowledgeCompilation.noSkills',
  [ViewMode.Graph]: 'knowledgeCompilation.noStructureGraph',
  [ViewMode.MindMap]: 'knowledgeCompilation.noStructureMindmap',
  [ViewMode.Timeline]: 'knowledgeCompilation.noStructureTimeline',
};

export function CompilationEmptyState({
  type,
  data,
}: ICompilationEmptyStateProps) {
  const { t } = useTranslation();
  const { status } = useGenerateStatus(data);

  const showProgress = status === 'running' || status === 'failed';

  return (
    <div className="flex-1 min-h-0 flex flex-col items-center justify-center border border-dashed border-border-button rounded-xl">
      {!showProgress ? (
        <div className="flex flex-col items-center gap-4">
          <p className="text-text-secondary text-lg">{t(TitleKeyMap[type])}</p>
          <p className="text-sm text-text-secondary">
            {t('knowledgeCompilation.autoCompiled')}
          </p>
        </div>
      ) : (
        <CompileProgressBoard
          status={status}
          data={data}
          label={t(ViewModeLabelKeyMap[type])}
        />
      )}
    </div>
  );
}

export default CompilationEmptyState;
