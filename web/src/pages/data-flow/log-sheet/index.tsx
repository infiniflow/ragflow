import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { NotebookText } from 'lucide-react';
import 'react18-json-view/src/style.css';
import { DataflowTimeline, DataflowTimelineProps } from './dataflow-timeline';

type LogSheetProps = IModalProps<any> & DataflowTimelineProps;

export function LogSheet({ hideModal, messageId }: LogSheetProps) {
  return (
    <Sheet open onOpenChange={hideModal} modal={false}>
      <SheetContent className={cn('top-20 right-[620px]')}>
        <SheetHeader>
          <SheetTitle className="flex items-center gap-1">
            <NotebookText className="size-4" />
            <DataflowTimeline messageId={messageId}></DataflowTimeline>
          </SheetTitle>
        </SheetHeader>
        <section className="max-h-[82vh] overflow-auto mt-6"></section>
      </SheetContent>
    </Sheet>
  );
}
