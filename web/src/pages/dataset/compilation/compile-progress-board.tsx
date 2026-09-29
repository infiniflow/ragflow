import { IconFontFill } from '@/components/icon-font';
import { GenerateStatus } from '@/constants/knowledge';
import { ITraceInfo } from '@/hooks/use-dataset-generate';
import { useTranslation } from 'react-i18next';

import { ProgressLogPanel } from './progress-log-panel';

type CompileProgressBoardProps = {
  status: GenerateStatus;
  data?: ITraceInfo;
  label: string;
};

// Go-backend compile progress board: the scheduler exposes no stable
// percentage and no scheduler cancel, so the center shows the MySQL
// inflight/backlog counts (or the error diagnostic) above the product label,
// and the right column streams the bounded phase log.
export function CompileProgressBoard({
  status,
  data,
  label,
}: CompileProgressBoardProps) {
  const { t } = useTranslation();

  return (
    <div className="grid h-full w-full grid-cols-[1fr_auto_1fr] items-center gap-8 p-6">
      <div />
      <div className="flex flex-col items-center gap-5">
        {status === GenerateStatus.Failed ? (
          <div className="flex flex-col items-center gap-2 text-state-error">
            <IconFontFill name="reparse" className="size-8" />
            <span className="text-text-primary">
              {data?.compilationError || t('message.operated')}
            </span>
          </div>
        ) : (
          <div className="flex flex-col items-center gap-2 text-text-secondary">
            <span className="text-4xl font-medium text-accent-primary">
              {t('knowledgeCompilation.compiling', {
                defaultValue: 'Compiling…',
              })}
            </span>
            <span>
              {t('knowledgeCompilation.compilingCounts', {
                inflight: data?.inflight ?? 0,
                backlog: data?.backlog ?? 0,
                defaultValue: '{{inflight}} processing / {{backlog}} queued',
              })}
            </span>
          </div>
        )}
        <div className="flex items-center gap-2 text-text-primary">
          <span>{label}</span>
        </div>
      </div>
      <div className="flex min-h-0 h-full justify-end">
        <ProgressLogPanel progressMsg={data?.progress_msg} />
      </div>
    </div>
  );
}
