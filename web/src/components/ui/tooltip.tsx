'use client';

import * as TooltipPrimitive from '@radix-ui/react-tooltip';
import * as React from 'react';

import { cn } from '@/lib/utils';
import { CircleQuestionMark } from 'lucide-react';

const TooltipProvider = TooltipPrimitive.Provider;

const Tooltip = TooltipPrimitive.Root;

const TooltipTrigger = TooltipPrimitive.Trigger;

const TooltipContent = React.forwardRef<
  React.ElementRef<typeof TooltipPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof TooltipPrimitive.Content>
>(({ className, sideOffset = 4, ...props }, ref) => (
  <TooltipPrimitive.Content
    ref={ref}
    sideOffset={sideOffset}
    className={cn(
      'z-50 overflow-auto scrollbar-auto rounded-md whitespace-pre-wrap border bg-bg-base px-3 py-1.5 text-sm text-text-primary shadow-md animate-in fade-in-0 zoom-in-95 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 max-w-[30vw]',
      className,
    )}
    {...props}
  />
));
TooltipContent.displayName = TooltipPrimitive.Content.displayName;

export { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger };

export const FormTooltip = ({ tooltip }: { tooltip: React.ReactNode }) => {
  return (
    <Tooltip>
      <TooltipTrigger
        tabIndex={-1}
        onClick={(e) => {
          e.preventDefault(); // Prevent clicking the tooltip from triggering form save
        }}
      >
        <CircleQuestionMark className="size-3 ml-[2px] -translate-y-1" />
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
};

export function RAGFlowTooltip({
  children,
  tooltip,
}: React.PropsWithChildren & { tooltip: React.ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger>{children}</TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

export interface AntToolTipProps {
  title: React.ReactNode;
  children: React.ReactNode;
  placement?: 'top' | 'bottom' | 'left' | 'right';
  trigger?: 'hover' | 'click' | 'focus';
  className?: string;
}

export const AntToolTip: React.FC<AntToolTipProps> = ({
  title,
  children,
  placement = 'top',
  trigger = 'hover',
  className,
}) => {
  const [visible, setVisible] = React.useState(false);

  const showTooltip = () => {
    if (trigger === 'hover' || trigger === 'focus') {
      setVisible(true);
    }
  };

  const hideTooltip = () => {
    if (trigger === 'hover' || trigger === 'focus') {
      setVisible(false);
    }
  };

  const toggleTooltip = () => {
    if (trigger === 'click') {
      setVisible(!visible);
    }
  };

  const getPlacementClasses = () => {
    switch (placement) {
      case 'top':
        return 'bottom-full left-1/2 transform -translate-x-1/2 mb-2';
      case 'bottom':
        return 'top-full left-1/2 transform -translate-x-1/2 mt-2';
      case 'left':
        return 'right-full top-1/2 transform -translate-y-1/2 mr-2';
      case 'right':
        return 'left-full top-1/2 transform -translate-y-1/2 ml-2';
      default:
        return 'bottom-full left-1/2 transform -translate-x-1/2 mb-2';
    }
  };

  return (
    <div className="inline-block relative">
      <div
        onMouseEnter={showTooltip}
        onMouseLeave={hideTooltip}
        onClick={toggleTooltip}
        onFocus={showTooltip}
        onBlur={hideTooltip}
      >
        {children}
      </div>
      {visible && title && (
        <div
          className={cn(
            'absolute z-50 px-2.5 py-2 text-xs text-text-primary bg-muted rounded-sm shadow-sm whitespace-wrap w-max',
            getPlacementClasses(),
            className,
          )}
        >
          {title}
          <div
            className={cn(
              'absolute w-2 h-2  bg-muted ',
              placement === 'top' &&
                'bottom-[-4px] left-1/2 transform -translate-x-1/2 rotate-45',
              placement === 'bottom' &&
                'top-[-4px] left-1/2 transform -translate-x-1/2 rotate-45',
              placement === 'left' &&
                'right-[-4px] top-1/2 transform -translate-y-1/2 rotate-45',
              placement === 'right' &&
                'left-[-4px] top-1/2 transform -translate-y-1/2 rotate-45',
            )}
          />
        </div>
      )}
    </div>
  );
};
