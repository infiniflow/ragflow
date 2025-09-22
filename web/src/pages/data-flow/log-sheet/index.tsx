import { Button } from '@/components/ui/button';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { useFetchMessageTrace } from '@/hooks/use-agent-request';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { NotebookText, SquareArrowOutUpRight } from 'lucide-react';
import { useEffect } from 'react';
import 'react18-json-view/src/style.css';
import {
  isEndOutputEmpty,
  useDownloadOutput,
} from '../hooks/use-download-output';
import { DataflowTimeline } from './dataflow-timeline';

type LogSheetProps = IModalProps<any> & { messageId?: string };

export function LogSheet({ hideModal, messageId }: LogSheetProps) {
  const { setMessageId, data } = useFetchMessageTrace(false);

  const { handleDownloadJson } = useDownloadOutput(data);

  useEffect(() => {
    if (messageId) {
      setMessageId(messageId);
    }
  }, [messageId, setMessageId]);

  return (
    <Sheet open onOpenChange={hideModal} modal={false}>
      <SheetContent className={cn('top-20 right-[620px]')}>
        <SheetHeader>
          <SheetTitle className="flex items-center gap-1">
            <NotebookText className="size-4" />
          </SheetTitle>
        </SheetHeader>
        <section className="max-h-[82vh] overflow-auto mt-6">
          <DataflowTimeline traceList={data}></DataflowTimeline>
          <Button
            onClick={handleDownloadJson}
            disabled={isEndOutputEmpty(data)}
            className="w-full mt-8"
          >
            <SquareArrowOutUpRight />
            Export JSON
          </Button>
        </section>
      </SheetContent>
    </Sheet>
  );
}
