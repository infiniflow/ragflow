import {
  Timeline,
  TimelineContent,
  TimelineHeader,
  TimelineIndicator,
  TimelineItem,
  TimelineSeparator,
  TimelineTitle,
} from '@/components/originui/timeline';
import { Progress } from '@/components/ui/progress';
import { ITraceData } from '@/interfaces/database/agent';
import { cn } from '@/lib/utils';
import { isEmpty } from 'lodash';
import { File } from 'lucide-react';
import { useCallback } from 'react';
import { Operator } from '../constant';
import OperatorIcon from '../operator-icon';
import useGraphStore from '../store';

export type DataflowTimelineProps = {
  traceList?: ITraceData[];
};

const END = 'END';

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

          const latest = traces[traces.length - 1];
          const progress = latest.progress * 100;

          return (
            <TimelineItem
              key={item.component_id}
              step={index}
              className="group-data-[orientation=vertical]/timeline:ms-10 group-data-[orientation=vertical]/timeline:not-last:pb-8 pb-6"
            >
              <TimelineHeader>
                <TimelineSeparator className="group-data-[orientation=vertical]/timeline:-left-7 group-data-[orientation=vertical]/timeline:h-[calc(100%-1.5rem-0.25rem)] group-data-[orientation=vertical]/timeline:translate-y-7 bg-accent-primary" />
                <TimelineTitle className="">
                  <TimelineContent
                    className={cn(
                      'text-foreground rounded-lg border px-4 py-3',
                    )}
                  >
                    <section className="flex items-center justify-between mb-2">
                      <span className="flex-1 truncate">
                        {getNodeData(item.component_id)?.name || END}
                      </span>
                      <div className="flex-1 flex items-center gap-5">
                        <Progress value={progress} className="h-1 flex-1" />
                        <span className="text-accent-primary text-xs">
                          {progress.toFixed(2)}%
                        </span>
                      </div>
                    </section>
                    <div className="divide-y space-y-1">
                      {traces
                        .filter((x) => !isEmpty(x.message))
                        .map((x, idx) => (
                          <section
                            key={idx}
                            className="text-text-secondary text-xs space-x-2 py-2.5 !m-0"
                          >
                            <span>{x.datetime}</span>
                            {item.component_id !== 'END' && (
                              <span
                                className={cn({
                                  'text-state-error':
                                    x.message.startsWith('[ERROR]'),
                                })}
                              >
                                {x.message}
                              </span>
                            )}
                            <span>
                              {x.elapsed_time.toString().slice(0, 6)}s
                            </span>
                          </section>
                        ))}
                    </div>
                  </TimelineContent>
                </TimelineTitle>
                <TimelineIndicator
                  className={cn(
                    'border border-accent-primary group-data-completed/timeline-item:bg-primary group-data-completed/timeline-item:text-primary-foreground flex size-5 items-center justify-center group-data-[orientation=vertical]/timeline:-left-7',
                    {
                      'rounded bg-accent-primary': nodeLabel === Operator.Begin,
                    },
                  )}
                >
                  {item.component_id === END ? (
                    <span className="rounded-full inline-block size-2 bg-accent-primary"></span>
                  ) : nodeLabel === Operator.Begin ? (
                    <File className="size-3.5 text-bg-base"></File>
                  ) : (
                    <OperatorIcon
                      name={nodeLabel}
                      className="size-3.5 rounded-full"
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
