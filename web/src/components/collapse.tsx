import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { CollapsibleProps } from '@radix-ui/react-collapsible';
import { ListCollapse } from 'lucide-react';
import { PropsWithChildren, ReactNode } from 'react';

type CollapseProps = Omit<CollapsibleProps, 'title'> & {
  title?: ReactNode;
  rightContent?: ReactNode;
} & PropsWithChildren;

export function Collapse({
  title,
  children,
  rightContent,
  open,
  defaultOpen = false,
  onOpenChange,
  disabled,
}: CollapseProps) {
  return (
    <Collapsible
      defaultOpen={defaultOpen}
      open={open}
      onOpenChange={onOpenChange}
      disabled={disabled}
    >
      <CollapsibleTrigger className="w-full">
        <section className="flex justify-between items-center pb-2">
          <div className="flex items-center gap-1">
            <ListCollapse className="size-4" /> {title}
          </div>
          <div>{rightContent}</div>
        </section>
      </CollapsibleTrigger>
      <CollapsibleContent>{children}</CollapsibleContent>
    </Collapsible>
  );
}
