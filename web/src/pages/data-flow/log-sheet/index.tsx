import { Button } from '@/components/ui/button';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { IModalProps } from '@/interfaces/common';
import { ITraceData } from '@/interfaces/database/agent';
import { cn } from '@/lib/utils';
import {
  ArrowUpRight,
  CirclePause,
  Logs,
  SquareArrowOutUpRight,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import 'react18-json-view/src/style.css';
import {
  isEndOutputEmpty,
  useDownloadOutput,
} from '../hooks/use-download-output';
import { DataflowTimeline } from './dataflow-timeline';

type LogSheetProps = IModalProps<any> & {
  isParsing: boolean;
  handleCancel(): void;
  logs?: ITraceData[];
};

export function LogSheet({
  hideModal,
  isParsing,
  logs,
  handleCancel,
}: LogSheetProps) {
  const { t } = useTranslation();

  const { handleDownloadJson } = useDownloadOutput(logs);

  return (
    <Sheet open onOpenChange={hideModal} modal={false}>
      <SheetContent
        className={cn('top-20')}
        onInteractOutside={(e) => e.preventDefault()}
      >
        <SheetHeader>
          <SheetTitle className="flex items-center gap-2.5">
            <Logs className="size-4" /> {t('flow.log')}
            <Button variant={'ghost'}>
              {t('dataflow.viewResult')} <ArrowUpRight />
            </Button>
          </SheetTitle>
        </SheetHeader>
        <section className="max-h-[82vh] overflow-auto mt-6">
          <DataflowTimeline traceList={logs}></DataflowTimeline>
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
