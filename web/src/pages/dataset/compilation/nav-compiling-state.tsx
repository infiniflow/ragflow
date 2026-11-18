import { GenerateStatus } from '@/constants/knowledge';
import { ITraceInfo } from '@/hooks/use-dataset-generate';
import { useTranslation } from 'react-i18next';

import { CompileProgressBoard } from './compile-progress-board';

type NavCompilingStateProps = {
  status: GenerateStatus;
  data?: ITraceInfo;
};

// Full-view placeholder while the Go backend compiles the dataset navigation
// tree and there is no tree content to show yet (the first compile). Once a
// tree exists, incremental compiles surface through the left panel's log
// entry instead of replacing the view.
export function NavCompilingState({ status, data }: NavCompilingStateProps) {
  const { t } = useTranslation();

  return (
    <div className="flex-1 min-h-0 flex flex-col items-center justify-center border border-dashed border-border-button rounded-xl">
      <CompileProgressBoard
        status={status}
        data={data}
        label={t('knowledgeCompilation.navTree')}
      />
    </div>
  );
}
