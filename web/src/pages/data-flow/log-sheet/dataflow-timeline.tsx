import {
  Timeline,
  TimelineContent,
  TimelineHeader,
  TimelineIndicator,
  TimelineItem,
  TimelineSeparator,
  TimelineTitle,
} from '@/components/originui/timeline';
import { ITraceData } from '@/interfaces/database/agent';
import { useCallback } from 'react';
import { Operator } from '../constant';
import OperatorIcon from '../operator-icon';
import useGraphStore from '../store';

export type DataflowTimelineProps = {
  traceList?: ITraceData[];
};

interface DataflowTrace {
  datetime: string;
  elapsed_time: number;
  message: string;
  progress: number;
  timestamp: number;
}
export function DataflowTimeline({ traceList }: DataflowTimelineProps) {
  const getNode = useGraphStore((state) => state.getNode);

  const getNodeData = useCallback(
    (componentId: string) => {
      return getNode(componentId)?.data;
    },
    [getNode],
  );

  const getNodeLabel = useCallback(
    (componentId: string) => {
      return getNodeData(componentId)?.label as Operator;
    },
    [getNodeData],
  );

  return (
    <Timeline>
      {Array.isArray(traceList) &&
        traceList?.map((item, index) => {
          const traces = item.trace as DataflowTrace[];
          const nodeLabel = getNodeLabel(item.component_id);

          return (
            <TimelineItem
              key={item.component_id}
              step={index}
              className="group-data-[orientation=vertical]/timeline:ms-10 group-data-[orientation=vertical]/timeline:not-last:pb-8"
            >
              <TimelineHeader>
                <TimelineSeparator className="group-data-[orientation=vertical]/timeline:-left-7 group-data-[orientation=vertical]/timeline:h-[calc(100%-1.5rem-0.25rem)] group-data-[orientation=vertical]/timeline:translate-y-7 bg-accent-primary" />
                <TimelineTitle className="">
                  <TimelineContent className="text-foreground mt-2 rounded-lg border px-4 py-3">
                    <p className="mb-2">
                      {getNodeData(item.component_id)?.name || 'END'}
                    </p>
                    <div className="divide-y space-y-1">
                      {traces.map((x, idx) => (
                        <section
                          key={idx}
                          className="text-text-secondary text-xs"
                        >
                          <div className="space-x-2">
                            <span>{x.datetime}</span>
                            <span>{x.progress * 100}%</span>
                            <span>{x.elapsed_time.toString().slice(0, 6)}</span>
                          </div>
                          {item.component_id !== 'END' && (
                            <div>{x.message}</div>
                          )}
                        </section>
                      ))}
                    </div>
                  </TimelineContent>
                </TimelineTitle>
                <TimelineIndicator className="border border-accent-primary group-data-completed/timeline-item:bg-primary group-data-completed/timeline-item:text-primary-foreground flex size-6 items-center justify-center group-data-[orientation=vertical]/timeline:-left-7">
                  {nodeLabel && (
                    <OperatorIcon
                      name={nodeLabel}
                      className="size-6 rounded-full"
                    ></OperatorIcon>
                  )}
                </TimelineIndicator>
              </TimelineHeader>
            </TimelineItem>
          );
        })}
    </Timeline>
  );
}
