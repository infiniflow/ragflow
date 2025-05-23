import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { ListCollapse } from 'lucide-react';
import { PropsWithChildren, ReactNode } from 'react';

type CollapseProps = {
  title?: ReactNode;
  rightContent?: ReactNode;
} & PropsWithChildren;

export function Collapse({ title, children, rightContent }: CollapseProps) {
  return (
    <Collapsible defaultOpen>
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
