/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { cn } from '@/lib/utils';
import { CollapsibleProps } from '@radix-ui/react-collapsible';
import {
  ChevronDown,
  ChevronUp,
  ListChevronsDownUp,
  ListChevronsUpDown,
} from 'lucide-react';
import * as React from 'react';
import {
  PropsWithChildren,
  ReactNode,
  useCallback,
  useEffect,
  useState,
} from 'react';

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
  const [internalOpen, setInternalOpen] = useState(open ?? defaultOpen);

  useEffect(() => {
    if (typeof open !== 'boolean') {
      setInternalOpen(defaultOpen);
    }
  }, [defaultOpen, open]);

  // A boolean `open` prop takes effect at render time so callers can rely on
  // the content being mounted right after they update it.
  const currentOpen = typeof open === 'boolean' ? open : internalOpen;

  const handleOpenChange = useCallback(
    (open: boolean) => {
      setInternalOpen(open);
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
      <section className="flex justify-between items-center gap-2">
        <CollapsibleTrigger className="flex min-w-0 flex-1 items-center">
          <div className="flex min-w-0 items-center gap-1">
            {currentOpen ? (
              <ListChevronsUpDown className="size-4 shrink-0" />
            ) : (
              <ListChevronsDownUp className="size-4 shrink-0 text-text-secondary" />
            )}
            <div
              className={cn('min-w-0 text-text-secondary', {
                'text-text-primary': currentOpen,
              })}
            >
              {title}
            </div>
          </div>
        </CollapsibleTrigger>
        {rightContent ? <div className="shrink-0">{rightContent}</div> : null}
      </section>
      <CollapsibleContent className="pt-5">{children}</CollapsibleContent>
    </Collapsible>
  );
}

export type NodeCollapsibleProps<T extends any[]> = {
  items?: T;
  children: (item: T[0], idx: number) => ReactNode;
  className?: string;
};
export function NodeCollapsible<T extends any[]>({
  items = [] as unknown as T,
  children,
  className,
}: NodeCollapsibleProps<T>) {
  const [isOpen, setIsOpen] = React.useState(false);

  const nextClassName = cn('space-y-2', className);

  const nextItems = items.every((x) => Array.isArray(x)) ? items.flat() : items;

  return (
    <Collapsible
      open={isOpen}
      onOpenChange={setIsOpen}
      className={cn('relative', nextClassName)}
    >
      {nextItems.slice(0, 3).map(children)}
      <CollapsibleContent className={nextClassName}>
        {nextItems.slice(3).map((x, idx) => children(x, idx + 3))}
      </CollapsibleContent>
      {nextItems.length > 3 && (
        <CollapsibleTrigger
          asChild
          onClick={(e) => e.stopPropagation()}
          className="absolute left-1/2 -translate-x-1/2 bottom-0 translate-y-1/2 cursor-pointer"
        >
          <div
            className={cn(
              'size-3 bg-text-secondary rounded-full flex items-center justify-center',
              { 'bg-text-primary': isOpen },
            )}
          >
            {isOpen ? (
              <ChevronUp className="stroke-bg-component" />
            ) : (
              <ChevronDown className="stroke-bg-component" />
            )}
          </div>
        </CollapsibleTrigger>
      )}
    </Collapsible>
  );
}
