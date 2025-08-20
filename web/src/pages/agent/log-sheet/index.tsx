import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { IModalProps } from '@/interfaces/common';
import { NotebookText } from 'lucide-react';
import 'react18-json-view/src/style.css';
import { useCacheChatLog } from '../hooks/use-cache-chat-log';
import { WorkFlowTimeline } from './workflow-timeline';

type LogSheetProps = IModalProps<any> &
  Pick<
    ReturnType<typeof useCacheChatLog>,
    'currentEventListWithoutMessageById' | 'currentMessageId'
  > & { sendLoading: boolean };

export function LogSheet({
  hideModal,
  currentEventListWithoutMessageById,
  currentMessageId,
  sendLoading,
}: LogSheetProps) {
  return (
    <Sheet open onOpenChange={hideModal} modal={false}>
      <SheetContent className="top-20 right-[620px]">
        <SheetHeader>
          <SheetTitle className="flex items-center gap-1">
            <NotebookText className="size-4" />
            Log
          </SheetTitle>
        </SheetHeader>
        <section className="max-h-[82vh] overflow-auto mt-6">
          <WorkFlowTimeline
            currentEventListWithoutMessage={currentEventListWithoutMessageById(
              currentMessageId,
            )}
            currentMessageId={currentMessageId}
            sendLoading={sendLoading}
          />
        </section>
      </SheetContent>
    </Sheet>
  );
}
