'use client';

import { cn } from '@/lib/utils';
import { TimelineNodeType } from '@/pages/dataflow-result/constant';
import { parseColorToRGB } from '@/utils/common-util';
import { Slot } from '@radix-ui/react-slot';
import * as React from 'react';

// Types
type TimelineContextValue = {
  activeStep: number;
  setActiveStep: (step: number) => void;
};

// Context
const TimelineContext = React.createContext<TimelineContextValue | undefined>(
  undefined,
);

const useTimeline = () => {
  const context = React.useContext(TimelineContext);
  if (!context) {
    throw new Error('useTimeline must be used within a Timeline');
  }
  return context;
};

// Components
interface TimelineProps extends React.HTMLAttributes<HTMLDivElement> {
  defaultValue?: number;
  value?: number;
  onValueChange?: (value: number) => void;
  orientation?: 'horizontal' | 'vertical';
}

function Timeline({
  defaultValue = 1,
  value,
  onValueChange,
  orientation = 'vertical',
  className,
  ...props
}: TimelineProps) {
  const [activeStep, setInternalStep] = React.useState(defaultValue);

  const setActiveStep = React.useCallback(
    (step: number) => {
      if (value === undefined) {
        setInternalStep(step);
      }
      onValueChange?.(step);
    },
    [value, onValueChange],
  );

  const currentStep = value ?? activeStep;

  return (
    <TimelineContext.Provider
      value={{ activeStep: currentStep, setActiveStep }}
    >
      <div
        data-slot="timeline"
        className={cn(
          'group/timeline flex data-[orientation=horizontal]:w-full data-[orientation=horizontal]:flex-row data-[orientation=vertical]:flex-col',
          className,
        )}
        data-orientation={orientation}
        {...props}
      />
    </TimelineContext.Provider>
  );
}

// TimelineContent
function TimelineContent({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      data-slot="timeline-content"
      className={cn('text-muted-foreground text-sm', className)}
      {...props}
    />
  );
}

// TimelineDate
interface TimelineDateProps extends React.HTMLAttributes<HTMLTimeElement> {
  asChild?: boolean;
}

function TimelineDate({
  asChild = false,
  className,
  ...props
}: TimelineDateProps) {
  const Comp = asChild ? Slot : 'time';

  return (
    <Comp
      data-slot="timeline-date"
      className={cn(
        'text-muted-foreground mb-1 block text-xs font-medium group-data-[orientation=vertical]/timeline:max-sm:h-4',
        className,
      )}
      {...props}
    />
  );
}

// TimelineHeader
function TimelineHeader({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div data-slot="timeline-header" className={cn(className)} {...props} />
  );
}

// TimelineIndicator
interface TimelineIndicatorProps extends React.HTMLAttributes<HTMLDivElement> {
  asChild?: boolean;
}

function TimelineIndicator({
  // asChild = false,
  className,
  children,
  ...props
}: TimelineIndicatorProps) {
  return (
    <div
      data-slot="timeline-indicator"
      className={cn(
        'border-primary/20 group-data-completed/timeline-item:border-primary absolute size-4 rounded-full border-2 group-data-[orientation=horizontal]/timeline:-top-6 group-data-[orientation=horizontal]/timeline:left-0 group-data-[orientation=horizontal]/timeline:-translate-y-1/2 group-data-[orientation=vertical]/timeline:top-0 group-data-[orientation=vertical]/timeline:-left-6 group-data-[orientation=vertical]/timeline:-translate-x-1/2',
        className,
      )}
      aria-hidden="true"
      {...props}
    >
      {children}
    </div>
  );
}

// TimelineItem
interface TimelineItemProps extends React.HTMLAttributes<HTMLDivElement> {
  step: number;
}

function TimelineItem({ step, className, ...props }: TimelineItemProps) {
  const { activeStep } = useTimeline();

  return (
    <div
      data-slot="timeline-item"
      className={cn(
        'group/timeline-item has-[+[data-completed]]:[&_[data-slot=timeline-separator]]:bg-primary relative flex flex-1 flex-col gap-0.5 group-data-[orientation=horizontal]/timeline:mt-8 group-data-[orientation=horizontal]/timeline:not-last:pe-8 group-data-[orientation=vertical]/timeline:ms-8 group-data-[orientation=vertical]/timeline:not-last:pb-12',
        className,
      )}
      data-completed={step <= activeStep || undefined}
      {...props}
    />
  );
}

// TimelineSeparator
function TimelineSeparator({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      data-slot="timeline-separator"
      className={cn(
        'bg-primary/10 absolute self-start group-last/timeline-item:hidden group-data-[orientation=horizontal]/timeline:-top-6 group-data-[orientation=horizontal]/timeline:h-0.5 group-data-[orientation=horizontal]/timeline:w-[calc(100%-1rem-0.25rem)] group-data-[orientation=horizontal]/timeline:translate-x-4.5 group-data-[orientation=horizontal]/timeline:-translate-y-1/2 group-data-[orientation=vertical]/timeline:-left-6 group-data-[orientation=vertical]/timeline:h-[calc(100%-1rem-0.25rem)] group-data-[orientation=vertical]/timeline:w-0.5 group-data-[orientation=vertical]/timeline:-translate-x-1/2 group-data-[orientation=vertical]/timeline:translate-y-4.5',
        className,
      )}
      aria-hidden="true"
      {...props}
    />
  );
}

// TimelineTitle
function TimelineTitle({
  className,
  ...props
}: React.HTMLAttributes<HTMLHeadingElement>) {
  return (
    <h3
      data-slot="timeline-title"
      className={cn('text-sm font-medium', className)}
      {...props}
    />
  );
}

interface TimelineIndicatorNodeProps {
  nodeSize?: string | number;
  iconColor?: string;
  lineColor?: string;
  textColor?: string;
  indicatorBgColor?: string;
  indicatorBorderColor?: string;
}
interface TimelineNode
  extends Omit<
      React.HTMLAttributes<HTMLDivElement>,
      'id' | 'title' | 'content'
    >,
    TimelineIndicatorNodeProps {
  id: string | number;
  title?: React.ReactNode;
  content?: React.ReactNode;
  date?: React.ReactNode;
  icon?: React.ReactNode;
  completed?: boolean;
  clickable?: boolean;
  activeStyle?: TimelineIndicatorNodeProps;
  detail?: any;
  type?: TimelineNodeType;
}

interface CustomTimelineProps extends React.HTMLAttributes<HTMLDivElement> {
  nodes: TimelineNode[];
  activeStep?: number;
  nodeSize?: string | number;
  onStepChange?: (step: number, id: string | number) => void;
  orientation?: 'horizontal' | 'vertical';
  lineStyle?: 'solid' | 'dashed';
  lineColor?: string;
  indicatorColor?: string;
  defaultValue?: number;
  activeStyle?: TimelineIndicatorNodeProps;
}

const CustomTimeline = ({
  nodes,
  activeStep,
  nodeSize = 12,
  onStepChange,
  orientation = 'horizontal',
  lineStyle = 'solid',
  lineColor = 'var(--text-secondary)',
  indicatorColor = 'rgb(var(--accent-primary))',
  defaultValue = 1,
  className,
  activeStyle,
  ...props
}: CustomTimelineProps) => {
  const [internalActiveStep, setInternalActiveStep] =
    React.useState(defaultValue);
  const _lineColor = `rgb(${parseColorToRGB(lineColor)})`;
  const currentActiveStep = activeStep ?? internalActiveStep;

  const handleStepChange = (step: number, id: string | number) => {
    if (activeStep === undefined) {
      setInternalActiveStep(step);
    }
    onStepChange?.(step, id);
  };
  const [r, g, b] = parseColorToRGB(indicatorColor);
  return (
    <Timeline
      value={currentActiveStep}
      onValueChange={(step) => handleStepChange(step, nodes[step - 1]?.id)}
      orientation={orientation}
      className={className}
      {...props}
    >
      {nodes.map((node, index) => {
        const step = index + 1;
        const isCompleted = node.completed ?? step <= currentActiveStep;
        const isActive = step === currentActiveStep;
        const isClickable = node.clickable ?? true;
        const _activeStyle = node.activeStyle ?? (activeStyle || {});
        const _nodeSizeTemp =
          isActive && _activeStyle?.nodeSize
            ? _activeStyle?.nodeSize
            : node.nodeSize ?? nodeSize;
        const _nodeSize =
          typeof _nodeSizeTemp === 'number'
            ? `${_nodeSizeTemp}px`
            : _nodeSizeTemp;

        return (
          <TimelineItem
            key={node.id}
            step={step}
            className={cn(
              node.className,
              isClickable &&
                'cursor-pointer hover:opacity-80 transition-opacity',
              isCompleted && 'data-[completed]:data-completed/timeline-item',
              isActive && 'relative z-10',
            )}
            onClick={() => isClickable && handleStepChange(step, node.id)}
          >
            <TimelineSeparator
              className={cn(
                'group-data-[orientation=horizontal]/timeline:-top-6 group-data-[orientation=horizontal]/timeline:h-0.1  group-data-[orientation=horizontal]/timeline:-translate-y-1/2',
                'group-data-[orientation=vertical]/timeline:-left-6 group-data-[orientation=vertical]/timeline:w-0.1 group-data-[orientation=vertical]/timeline:-translate-x-1/2 ',
                // `group-data-[orientation=horizontal]/timeline:w-[calc(100%-0.5rem-1rem)] group-data-[orientation=vertical]/timeline:h-[calc(100%-1rem-1rem)] group-data-[orientation=vertical]/timeline:translate-y-7 group-data-[orientation=horizontal]/timeline:translate-x-7`,
              )}
              style={{
                border:
                  lineStyle === 'dashed'
                    ? `1px dashed ${isActive ? _activeStyle.lineColor || _lineColor : _lineColor}`
                    : lineStyle === 'solid'
                      ? `1px solid ${isActive ? _activeStyle.lineColor || _lineColor : _lineColor}`
                      : 'none',
                backgroundColor: 'transparent',
                width:
                  orientation === 'horizontal'
                    ? `calc(100% - ${_nodeSize} - 2px - 0.1rem)`
                    : '1px',
                height:
                  orientation === 'vertical'
                    ? `calc(100% - ${_nodeSize} - 2px - 0.1rem)`
                    : '1px',
                transform: `translate(${
                  orientation === 'horizontal' ? `${_nodeSize}` : '0'
                }, ${orientation === 'vertical' ? `${_nodeSize}` : '0'})`,
              }}
            />

            <TimelineIndicator
              className={cn(
                'flex items-center justify-center p-1',
                isCompleted && 'bg-primary border-primary',
                !isCompleted && 'border-text-secondary bg-bg-base',
              )}
              style={{
                width: _nodeSize,
                height: _nodeSize,
                borderColor: isActive
                  ? _activeStyle.indicatorBorderColor || indicatorColor
                  : isCompleted
                    ? indicatorColor
                    : '',
                // backgroundColor: isActive
                //   ? _activeStyle.indicatorBgColor || indicatorColor
                //   : isCompleted
                //     ? indicatorColor
                //     : '',
                backgroundColor: isActive
                  ? _activeStyle.indicatorBgColor ||
                    `rgba(${r}, ${g}, ${b}, 0.1)`
                  : isCompleted
                    ? `rgba(${r}, ${g}, ${b}, 0.1)`
                    : '',
              }}
            >
              {node.icon && (
                <div
                  className={cn(
                    'text-current',
                    `w-[${_nodeSize}] h-[${_nodeSize}]`,
                    isActive &&
                      `text-primary w-[${_activeStyle.nodeSize || _nodeSize}] h-[${_activeStyle.nodeSize || _nodeSize}]`,
                  )}
                  style={{
                    color: isActive ? _activeStyle.iconColor : undefined,
                  }}
                >
                  {node.icon}
                </div>
              )}
            </TimelineIndicator>

            <TimelineHeader className="transform -translate-x-[40%] text-center">
              <TimelineTitle
                className={cn(
                  'text-sm font-medium -ml-1',
                  isActive && _activeStyle.textColor
                    ? `text-${_activeStyle.textColor}`
                    : '',
                )}
                style={{
                  color: isActive ? _activeStyle.textColor : undefined,
                }}
              >
                {node.title}
              </TimelineTitle>
              {node.date && <TimelineDate>{node.date}</TimelineDate>}
            </TimelineHeader>
            {node.content && <TimelineContent>{node.content}</TimelineContent>}
          </TimelineItem>
        );
      })}
    </Timeline>
  );
};

CustomTimeline.displayName = 'CustomTimeline';

export {
  CustomTimeline,
  Timeline,
  TimelineContent,
  TimelineDate,
  TimelineHeader,
  TimelineIndicator,
  TimelineItem,
  TimelineSeparator,
  TimelineTitle,
  type TimelineNode,
};
