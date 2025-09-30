import { SkeletonCard } from '@/components/skeleton-card';
import { Button } from '@/components/ui/button';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchAgent } from '@/hooks/use-agent-request';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { PipelineResultSearchParams } from '@/pages/dataflow-result/constant';
import {
  ArrowUpRight,
  CirclePause,
  Logs,
  SquareArrowOutUpRight,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import 'react18-json-view/src/style.css';
import { useParams } from 'umi';
import {
  isEndOutputEmpty,
  useDownloadOutput,
} from '../hooks/use-download-output';
import { UseFetchLogReturnType } from '../hooks/use-fetch-log';
import { DataflowTimeline } from './dataflow-timeline';

type LogSheetProps = IModalProps<any> & {
  handleCancel(): void;
  uploadedFileData?: Record<string, any>;
} & Pick<
    UseFetchLogReturnType,
    'isCompleted' | 'isLogEmpty' | 'isParsing' | 'logs' | 'messageId'
  >;

export function LogSheet({
  hideModal,
  isParsing,
  logs,
  handleCancel,
  isCompleted,
  isLogEmpty,
  messageId,
  uploadedFileData,
}: LogSheetProps) {
  const { t } = useTranslation();
  const { id } = useParams();
  const { data: agent } = useFetchAgent();

  const { handleDownloadJson } = useDownloadOutput(logs);
  const { navigateToDataflowResult } = useNavigatePage();

  return (
    <Sheet open onOpenChange={hideModal} modal={false}>
      <SheetContent
        className={cn('top-20')}
        onInteractOutside={(e) => e.preventDefault()}
      >
        <SheetHeader>
          <SheetTitle className="flex items-center gap-2.5">
            <Logs className="size-4" /> {t('flow.log')}
            <Button
              variant={'ghost'}
              disabled={!isCompleted}
              onClick={navigateToDataflowResult({
                id: messageId, // 'log_id',
                [PipelineResultSearchParams.AgentId]: id, // 'agent_id',
                [PipelineResultSearchParams.DocumentId]: uploadedFileData?.id, //'doc_id',
                [PipelineResultSearchParams.AgentTitle]: agent.title, //'title',
                [PipelineResultSearchParams.IsReadOnly]: 'true',
                [PipelineResultSearchParams.Type]: 'dataflow',
                [PipelineResultSearchParams.CreatedBy]:
                  uploadedFileData?.created_by,
                [PipelineResultSearchParams.DocumentExtension]:
                  uploadedFileData?.extension,
              })}
            >
              {t('dataflow.viewResult')} <ArrowUpRight />
            </Button>
          </SheetTitle>
        </SheetHeader>
        <section className="max-h-[82vh] overflow-auto mt-6">
          {isLogEmpty ? (
            <SkeletonCard className="mt-2" />
          ) : (
            <DataflowTimeline traceList={logs}></DataflowTimeline>
          )}
        </section>
        {isParsing ? (
          <Button
            className="w-full mt-8 bg-state-error/10 text-state-error hover:bg-state-error hover:text-bg-base"
            onClick={handleCancel}
          >
            <CirclePause /> {t('dataflow.cancel')}
          </Button>
        ) : (
          <Button
            onClick={handleDownloadJson}
            disabled={isEndOutputEmpty(logs)}
            className="w-full mt-8 bg-accent-primary-5 text-text-secondary hover:bg-accent-primary-5  hover:text-accent-primary hover:border-accent-primary hover:border"
          >
            <SquareArrowOutUpRight />
            {t('dataflow.exportJson')}
          </Button>
        )}
      </SheetContent>
    </Sheet>
  );
}
