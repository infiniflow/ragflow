import {
  Timeline,
  TimelineContent,
  TimelineHeader,
  TimelineIndicator,
  TimelineItem,
  TimelineSeparator,
} from '@/components/originui/timeline';
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
import {
  ILogData,
  ILogEvent,
  MessageEventType,
} from '@/hooks/use-send-message';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { isEmpty } from 'lodash';
import { BellElectric, NotebookText } from 'lucide-react';
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

function concatData(
  firstRecord: Record<string, any> | Array<Record<string, any>>,
  nextRecord: Record<string, any> | Array<Record<string, any>>,
) {
  let result: Array<Record<string, any>> = [];

  if (!isEmpty(firstRecord)) {
    result = result.concat(firstRecord);
  }

  if (!isEmpty(nextRecord)) {
    result = result.concat(nextRecord);
  }

  return isEmpty(result) ? {} : result;
}

type EventWithIndex = { startNodeIdx: number } & ILogEvent;

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

  // Look up to find the nearest start component id and concatenate the finish and log data into one
  const finishedNodeList = useMemo(() => {
    return currentEventListWithoutMessage.filter(
      (x) =>
        x.event === MessageEventType.NodeFinished ||
        x.event === MessageEventType.NodeLogs,
    ) as ILogEvent[];
  }, [currentEventListWithoutMessage]);

  const nextList = useMemo(() => {
    return finishedNodeList.reduce<Array<EventWithIndex>>((pre, cur) => {
      const startNodeIdx = (
        currentEventListWithoutMessage as Array<ILogEvent>
      ).findLastIndex(
        (x) =>
          x.data.component_id === cur.data.component_id &&
          x.event === MessageEventType.NodeStarted,
      );

      const item = pre.find((x) => x.startNodeIdx === startNodeIdx);

      const { logs = {}, inputs = {}, outputs = {} } = cur.data;
      if (item) {
        const {
          inputs: inputList,
          outputs: outputList,
          logs: logList,
        } = item.data;

        item.data = {
          ...item.data,
          inputs: concatData(inputList, inputs),
          outputs: concatData(outputList, outputs),
          logs: concatData(logList, logs),
        };
      } else {
        pre.push({
          ...cur,
          startNodeIdx,
        });
      }

      return pre;
    }, []);
  }, [currentEventListWithoutMessage, finishedNodeList]);

  return (
    <Sheet open onOpenChange={hideModal} modal={false}>
      <SheetContent className="top-20 right-[440px]">
        <SheetHeader>
          <SheetTitle className="flex items-center gap-1">
            <NotebookText className="size-4" />
            Log
          </SheetTitle>
        </SheetHeader>
        <section className="max-h-[82vh] overflow-auto mt-6">
          <Timeline>
            {nextList.map((x, idx) => (
              <TimelineItem
                key={idx}
                step={idx}
                className="group-data-[orientation=vertical]/timeline:ms-10 group-data-[orientation=vertical]/timeline:not-last:pb-8"
              >
                <TimelineHeader>
                  <TimelineSeparator className="group-data-[orientation=vertical]/timeline:-left-7 group-data-[orientation=vertical]/timeline:h-[calc(100%-1.5rem-0.25rem)] group-data-[orientation=vertical]/timeline:translate-y-6.5 top-6 bg-background-checked" />

                  <TimelineIndicator className="bg-primary/10 group-data-completed/timeline-item:bg-primary group-data-completed/timeline-item:text-primary-foreground flex size-6 items-center justify-center border-none group-data-[orientation=vertical]/timeline:-left-7">
                    <BellElectric className="size-5" />
                    {/* <img
                      src={item.image}
                      alt={item.title}
                      className="size-6 rounded-full"
                    /> */}
                  </TimelineIndicator>
                </TimelineHeader>
                <TimelineContent className="text-foreground  rounded-lg border  mb-5">
                  <section key={idx}>
                    <Accordion
                      type="single"
                      collapsible
                      className="bg-background-card px-3"
                    >
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

                            {isEmpty((x.data as ILogData)?.logs) || (
                              <JsonViewer
                                data={(x.data as ILogData)?.logs}
                                title={'Logs'}
                              ></JsonViewer>
                            )}

                            <JsonViewer
                              data={x.data.outputs}
                              title={'Output'}
                            ></JsonViewer>
                          </div>
                        </AccordionContent>
                      </AccordionItem>
                    </Accordion>
                  </section>
                  {/* <TimelineDate className="mt-1 mb-0">{item.date}</TimelineDate> */}
                </TimelineContent>
              </TimelineItem>
            ))}
          </Timeline>
        </section>
      </SheetContent>
    </Sheet>
  );
}
