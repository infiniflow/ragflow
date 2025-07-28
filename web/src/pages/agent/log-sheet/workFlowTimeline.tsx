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
import { useFetchMessageTrace } from '@/hooks/use-agent-request';
import {
  INodeData,
  INodeEvent,
  MessageEventType,
} from '@/hooks/use-send-message';
import { ITraceData } from '@/interfaces/database/agent';
import { cn } from '@/lib/utils';
import { get } from 'lodash';
import { useCallback, useEffect, useMemo, useState } from 'react';
import JsonView from 'react18-json-view';
import { Operator } from '../constant';
import { useCacheChatLog } from '../hooks/use-cache-chat-log';
import OperatorIcon from '../operator-icon';
import ToolTimelineItem from './toolTimelineItem';
type LogFlowTimelineProps = Pick<
  ReturnType<typeof useCacheChatLog>,
  'currentEventListWithoutMessage' | 'currentMessageId'
> & { canvasId?: string };
export function JsonViewer({
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
export const WorkFlowTimeline = ({
  currentEventListWithoutMessage,
  currentMessageId,
  canvasId,
}: LogFlowTimelineProps) => {
  // const getNode = useGraphStore((state) => state.getNode);
  const [isStopFetchTrace, setISStopFetchTrace] = useState(false);

  const { data: traceData, setMessageId } = useFetchMessageTrace(
    isStopFetchTrace,
    canvasId,
  );

  useEffect(() => {
    setMessageId(currentMessageId);
  }, [currentMessageId, setMessageId]);
  // const getNodeName = useCallback(
  //   (nodeId: string) => {
  //     if ('begin' === nodeId) return t('flow.begin');
  //     return getNode(nodeId)?.data.name;
  //   },
  //   [getNode],
  // );
  // const getNodeById = useCallback(
  //   (nodeId: string) => {
  //     const data = currentEventListWithoutMessage
  //       .map((x) => x.data)
  //       .filter((x) => x.component_id === nodeId);
  //     if ('begin' === nodeId) return t('flow.begin');
  //     if (data && data.length) {
  //       return data[0];
  //     }
  //     return {};
  //   },
  //   [currentEventListWithoutMessage],
  // );
  const startedNodeList = useMemo(() => {
    const finish = currentEventListWithoutMessage?.some(
      (item) => item.event === MessageEventType.WorkflowFinished,
    );
    setISStopFetchTrace(finish);
    const duplicateList = currentEventListWithoutMessage?.filter(
      (x) => x.event === MessageEventType.NodeStarted,
    ) as INodeEvent[];

    // Remove duplicate nodes
    return duplicateList?.reduce<Array<INodeEvent>>((pre, cur) => {
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
      return Array.isArray(trace) ? trace : [{}];
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
    <Timeline>
      {startedNodeList?.map((x, idx) => {
        const nodeDataList = filterFinishedNodeList(x.data.component_id);
        const finishNodeIds = nodeDataList.map(
          (x: INodeData) => x.component_id,
        );
        const inputs = getInputsOrOutputs(nodeDataList, 'inputs');
        const outputs = getInputsOrOutputs(nodeDataList, 'outputs');
        const nodeLabel = x.data.component_type;
        return (
          <>
            <TimelineItem
              key={idx}
              step={idx}
              className="group-data-[orientation=vertical]/timeline:ms-10 group-data-[orientation=vertical]/timeline:not-last:pb-8"
            >
              <TimelineHeader>
                <TimelineSeparator
                  className="group-data-[orientation=vertical]/timeline:-left-7 group-data-[orientation=vertical]/timeline:h-[calc(100%-1.5rem-0.25rem)] group-data-[orientation=vertical]/timeline:translate-y-6.5 top-6 bg-background-checked"
                  style={{
                    background:
                      x.data.component_type === 'Agent'
                        ? 'repeating-linear-gradient( to bottom, rgba(76, 164, 231, 1), rgba(76, 164, 231, 1) 5px, transparent 5px, transparent 10px'
                        : '',
                  }}
                />

                <TimelineIndicator
                  className={cn(
                    ' group-data-completed/timeline-item:bg-primary group-data-completed/timeline-item:text-primary-foreground flex size-6 p-1  items-center justify-center group-data-[orientation=vertical]/timeline:-left-7',
                    {
                      'border border-blue-500': finishNodeIds.includes(
                        x.data.component_id,
                      ),
                    },
                  )}
                >
                  <div className='relative after:content-[""] after:absolute after:inset-0 after:z-10 after:bg-transparent after:transition-all after:duration-300'>
                    <div className="absolute inset-0 z-10 flex items-center justify-center ">
                      <div
                        className={cn('rounded-full w-6 h-6', {
                          ' border-muted-foreground border-2 border-t-transparent animate-spin ':
                            !finishNodeIds.includes(x.data.component_id),
                        })}
                      ></div>
                    </div>
                    <div className="size-6 flex items-center justify-center">
                      <OperatorIcon
                        className="size-4"
                        name={nodeLabel as Operator}
                      ></OperatorIcon>
                    </div>
                  </div>
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
                          <span>{x.data?.component_name}</span>
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
                          <JsonViewer data={inputs} title="Input"></JsonViewer>

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
            {hasTrace(x.data.component_id) && (
              <ToolTimelineItem
                tools={filterTrace(x.data.component_id)}
              ></ToolTimelineItem>
            )}
          </>
        );
      })}
    </Timeline>
  );
};
