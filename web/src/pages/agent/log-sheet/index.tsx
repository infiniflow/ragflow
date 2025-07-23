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
import { useFetchMessageTrace } from '@/hooks/use-agent-request';
import {
  INodeData,
  INodeEvent,
  MessageEventType,
} from '@/hooks/use-send-message';
import { IModalProps } from '@/interfaces/common';
import { ITraceData } from '@/interfaces/database/agent';
import { cn } from '@/lib/utils';
import { get } from 'lodash';
import { BellElectric, NotebookText } from 'lucide-react';
import { useCallback, useEffect, useMemo } from 'react';
import JsonView from 'react18-json-view';
import 'react18-json-view/src/style.css';
import { useCacheChatLog } from '../hooks/use-cache-chat-log';
import useGraphStore from '../store';

type LogSheetProps = IModalProps<any> &
  Pick<
    ReturnType<typeof useCacheChatLog>,
    'currentEventListWithoutMessage' | 'currentMessageId'
  >;

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

function getInputsOrOutputs(
  nodeEventList: INodeData[],
  field: 'inputs' | 'outputs',
) {
  const inputsOrOutputs = nodeEventList.map((x) => get(x, field, {}));

  if (inputsOrOutputs.length < 2) {
    return inputsOrOutputs[0] || {};
  }

  return inputsOrOutputs;
}

export function LogSheet({
  hideModal,
  currentEventListWithoutMessage,
  currentMessageId,
}: LogSheetProps) {
  const getNode = useGraphStore((state) => state.getNode);

  const { data: traceData, setMessageId } = useFetchMessageTrace();

  useEffect(() => {
    setMessageId(currentMessageId);
  }, [currentMessageId, setMessageId]);

  const getNodeName = useCallback(
    (nodeId: string) => {
      return getNode(nodeId)?.data.name;
    },
    [getNode],
  );

  const startedNodeList = useMemo(() => {
    const duplicateList = currentEventListWithoutMessage.filter(
      (x) => x.event === MessageEventType.NodeStarted,
    ) as INodeEvent[];

    // Remove duplicate nodes
    return duplicateList.reduce<Array<INodeEvent>>((pre, cur) => {
      if (pre.every((x) => x.data.component_id !== cur.data.component_id)) {
        pre.push(cur);
      }
      return pre;
    }, []);
  }, [currentEventListWithoutMessage]);

  const hasTrace = useCallback(
    (componentId: string) => {
      if (Array.isArray(traceData)) {
        return traceData?.some((x) => x.component_id === componentId);
      }
      return false;
    },
    [traceData],
  );

  const filterTrace = useCallback(
    (componentId: string) => {
      const trace = traceData
        ?.filter((x) => x.component_id === componentId)
        .reduce<ITraceData['trace']>((pre, cur) => {
          pre.push(...cur.trace);

          return pre;
        }, []);
      return Array.isArray(trace) ? trace : {};
    },
    [traceData],
  );

  const filterFinishedNodeList = useCallback(
    (componentId: string) => {
      const nodeEventList = currentEventListWithoutMessage
        .filter(
          (x) =>
            x.event === MessageEventType.NodeFinished &&
            (x.data as INodeData)?.component_id === componentId,
        )
        .map((x) => x.data);

      return nodeEventList;
    },
    [currentEventListWithoutMessage],
  );

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
          <Timeline>
            {startedNodeList.map((x, idx) => {
              const nodeDataList = filterFinishedNodeList(x.data.component_id);
              const inputs = getInputsOrOutputs(nodeDataList, 'inputs');
              const outputs = getInputsOrOutputs(nodeDataList, 'outputs');
              return (
                <TimelineItem
                  key={idx}
                  step={idx}
                  className="group-data-[orientation=vertical]/timeline:ms-10 group-data-[orientation=vertical]/timeline:not-last:pb-8"
                >
                  <TimelineHeader>
                    <TimelineSeparator className="group-data-[orientation=vertical]/timeline:-left-7 group-data-[orientation=vertical]/timeline:h-[calc(100%-1.5rem-0.25rem)] group-data-[orientation=vertical]/timeline:translate-y-6.5 top-6 bg-background-checked" />

                    <TimelineIndicator className="bg-primary/10 group-data-completed/timeline-item:bg-primary group-data-completed/timeline-item:text-primary-foreground flex size-6 items-center justify-center border-none group-data-[orientation=vertical]/timeline:-left-7">
                      <BellElectric className="size-5" />
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
                                data={inputs}
                                title="Input"
                              ></JsonViewer>

                              {hasTrace(x.data.component_id) && (
                                <JsonViewer
                                  data={filterTrace(x.data.component_id)}
                                  title={'Trace'}
                                ></JsonViewer>
                              )}

                              <JsonViewer
                                data={outputs}
                                title={'Output'}
                              ></JsonViewer>
                            </div>
                          </AccordionContent>
                        </AccordionItem>
                      </Accordion>
                    </section>
                  </TimelineContent>
                </TimelineItem>
              );
            })}
          </Timeline>
        </section>
      </SheetContent>
    </Sheet>
  );
}
