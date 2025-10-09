import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { cn } from '@/lib/utils';
import { CollapsibleProps } from '@radix-ui/react-collapsible';
import {
  PropsWithChildren,
  ReactNode,
  useCallback,
  useEffect,
  useState,
} from 'react';
import { IconFontFill } from './icon-font';

type CollapseProps = Omit<CollapsibleProps, 'title'> & {
  title?: ReactNode;
  rightContent?: ReactNode;
} & PropsWithChildren;

export function Collapse({
  title,
  children,
  rightContent,
  open = true,
  defaultOpen = false,
  onOpenChange,
  disabled,
}: CollapseProps) {
  const [currentOpen, setCurrentOpen] = useState(open);

  useEffect(() => {
    setCurrentOpen(open);
  }, [open]);

  const handleOpenChange = useCallback(
    (open: boolean) => {
      setCurrentOpen(open);
      onOpenChange?.(open);
    },
    [onOpenChange],
  );

  return (
    <Collapsible
      defaultOpen={defaultOpen}
      open={currentOpen}
      onOpenChange={handleOpenChange}
      disabled={disabled}
    >
      <CollapsibleTrigger className="w-full">
        <section className="flex justify-between items-center pb-2">
          <div className="flex items-center gap-1">
            <IconFontFill
              name={`more`}
              className={cn('size-4', {
                'rotate-90': !currentOpen,
              })}
            ></IconFontFill>
            {title}
          </div>
          <div>{rightContent}</div>
        </section>
      </CollapsibleTrigger>
      <CollapsibleContent>{children}</CollapsibleContent>
    </Collapsible>
  );
}
