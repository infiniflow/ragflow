import {
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
import { cn } from '@/lib/utils';
import { Operator } from '../constant';
import OperatorIcon from '../operator-icon';
import { JsonViewer } from './workFlowTimeline';

const ToolTimelineItem = ({ tools }: { tools: Record<string, any>[] }) => {
  if (!tools || tools.length === 0 || !Array.isArray(tools)) return null;
  const blackList = ['add_memory', 'gen_citations'];
  const filteredTools = tools.filter(
    (tool) => !blackList.includes(tool.tool_name),
  );
  const capitalizeWords = (str: string, separator: string = '_'): string => {
    if (!str) return '';

    return str
      .split(separator)
      .map((word) => {
        return word.charAt(0).toUpperCase() + word.slice(1).toLowerCase();
      })
      .join(' ');
  };
  return (
    <>
      {filteredTools?.map((tool, idx) => {
        return (
          <TimelineItem
            key={'tool_' + idx}
            step={idx}
            className="group-data-[orientation=vertical]/timeline:ms-10 group-data-[orientation=vertical]/timeline:not-last:pb-8"
          >
            <TimelineHeader>
              <TimelineSeparator
                className="group-data-[orientation=vertical]/timeline:-left-7 group-data-[orientation=vertical]/timeline:h-[calc(100%-1.5rem-0.25rem)] group-data-[orientation=vertical]/timeline:translate-y-6.5 top-6"
                style={{
                  background:
                    idx < filteredTools.length - 1
                      ? 'repeating-linear-gradient( to bottom, rgba(76, 164, 231, 1), rgba(76, 164, 231, 1) 5px, transparent 5px, transparent 10px'
                      : 'rgba(76, 164, 231, 1)',
                  width: '1px',
                }}
              />

              <TimelineIndicator
                className={cn(
                  'group-data-completed/timeline-item:bg-primary group-data-completed/timeline-item:text-primary-foreground flex size-6 p-1 items-center justify-center group-data-[orientation=vertical]/timeline:-left-7',
                  {
                    'border border-blue-500': !(
                      idx >= filteredTools.length - 1 && tool.result === '...'
                    ),
                  },
                )}
              >
                <div className='relative after:content-[""] after:absolute after:inset-0 after:z-10 after:bg-transparent after:transition-all after:duration-300'>
                  <div className="absolute inset-0 z-10 flex items-center justify-center ">
                    <div
                      className={cn('rounded-full w-6 h-6', {
                        ' border-muted-foreground border-2 border-t-transparent animate-spin ':
                          idx >= filteredTools.length - 1 &&
                          tool.result === '...',
                      })}
                    ></div>
                  </div>
                  <div className="size-6 flex items-center justify-center">
                    <OperatorIcon
                      className="size-4"
                      name={'Agent' as Operator}
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
                        <span>
                          {tool.path + ' '}
                          {capitalizeWords(tool.tool_name, '_')}
                        </span>
                        <span className="text-text-sub-title text-xs">
                          {/* 0:00
                          {x.data.elapsed_time?.toString().slice(0, 6)} */}
                        </span>
                        <span
                          className={cn(
                            'border-background  -end-1 -top-1 size-2 rounded-full border-2 bg-dot-green',
                          )}
                        >
                          <span className="sr-only">Online</span>
                        </span>
                      </div>
                    </AccordionTrigger>
                    <AccordionContent>
                      <div className="space-y-2">
                        <JsonViewer
                          data={tool.result}
                          title="content"
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
    </>
  );
};

export default ToolTimelineItem;
