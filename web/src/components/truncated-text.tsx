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
