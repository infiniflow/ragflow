import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { INodeEvent, MessageEventType } from '@/hooks/use-send-message';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { NotebookText } from 'lucide-react';
import { useCallback, useMemo } from 'react';
import JsonView from 'react18-json-view';
import 'react18-json-view/src/style.css';
import { useCacheChatLog } from '../hooks/use-cache-chat-log';
import useGraphStore from '../store';

type LogSheetProps = IModalProps<any> &
  Pick<ReturnType<typeof useCacheChatLog>, 'currentEventListWithoutMessage'>;

function JsonViewer({
  data,
  title,
}: {
  data: Record<string, any>;
  title: string;
}) {
  return (
    <section className="space-y-2">
      <div>{title}</div>
      <JsonView
        src={data}
        displaySize
        collapseStringsAfterLength={100000000000}
        className="w-full h-[200px] break-words overflow-auto p-2 bg-slate-800"
      />
    </section>
  );
}

export function LogSheet({
  hideModal,
  currentEventListWithoutMessage,
}: LogSheetProps) {
  const getNode = useGraphStore((state) => state.getNode);

  const getNodeName = useCallback(
    (nodeId: string) => {
      return getNode(nodeId)?.data.name;
    },
    [getNode],
  );

  const finishedNodeList = useMemo(() => {
    return currentEventListWithoutMessage.filter(
      (x) => x.event === MessageEventType.NodeFinished,
    ) as INodeEvent[];
  }, [currentEventListWithoutMessage]);

  return (
    <Sheet open onOpenChange={hideModal} modal={false}>
      <SheetContent className="top-20 right-96">
        <SheetHeader>
          <SheetTitle className="flex items-center gap-1">
            <NotebookText className="size-4" />
            Log
          </SheetTitle>
        </SheetHeader>
        <section className="max-h-[82vh] overflow-auto">
          {finishedNodeList.map((x, idx) => (
            <section key={idx}>
              <Accordion type="single" collapsible>
                <AccordionItem value={idx.toString()}>
                  <AccordionTrigger>
                    <div className="flex gap-2 items-center">
                      <span>{getNodeName(x.data?.component_id)}</span>
                      <span className="text-text-sub-title text-xs">
                        {x.data.elapsed_time?.toString().slice(0, 6)}
                      </span>
                      <span
                        className={cn(
                          'border-background  -end-1 -top-1 size-2 rounded-full border-2 bg-dot-green',
                          { 'text-dot-green': x.data.error === null },
                          { 'text-dot-red': x.data.error !== null },
                        )}
                      >
                        <span className="sr-only">Online</span>
                      </span>
                    </div>
                  </AccordionTrigger>
                  <AccordionContent>
                    <div className="space-y-2">
                      <JsonViewer
                        data={x.data.inputs}
                        title="Input"
                      ></JsonViewer>

                      <JsonViewer
                        data={x.data.outputs}
                        title="Output"
                      ></JsonViewer>
                    </div>
                  </AccordionContent>
                </AccordionItem>
              </Accordion>
            </section>
          ))}
        </section>
      </SheetContent>
    </Sheet>
  );
}
