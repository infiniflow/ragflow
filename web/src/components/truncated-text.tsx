import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { ElementType, ReactNode, useRef, useState } from 'react';

export function TruncatedText({
  as: Tag = 'div',
  className,
  children,
  tooltip,
  testId,
}: {
  as?: ElementType;
  className?: string;
  children?: ReactNode;
  tooltip?: ReactNode;
  testId?: string;
}) {
  const ref = useRef<HTMLElement>(null);
  const [open, setOpen] = useState(false);

  if (tooltip == null) {
    return (
      <Tag ref={ref} className={className} data-testid={testId}>
        {children}
      </Tag>
    );
  }

  return (
    <Tooltip
      open={open}
      onOpenChange={(next) => {
        const el = ref.current;
        setOpen(next && el !== null && el.scrollWidth > el.clientWidth);
      }}
    >
      <TooltipTrigger asChild>
        <Tag ref={ref} className={className} data-testid={testId} tabIndex={0}>
          {children}
        </Tag>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}
