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

import { cn } from '@/lib/utils';
import { Ellipsis } from 'lucide-react';
import React from 'react';
import { Button, ButtonProps } from './ui/button';

export const MoreButton = React.forwardRef<HTMLButtonElement, ButtonProps>(
  function MoreButton({ className, size, ...props }, ref) {
    return (
      <Button
        ref={ref}
        variant="ghost"
        size={size || 'icon-xs'}
        className={cn(
          'opacity-0 size-3.5 transition-all bg-transparent group-hover:bg-transparent',
          // Keep the 14px visual size but increase the hit area with an
          // absolutely positioned pseudo-element (14px + 8px on each side).
          'relative after:absolute after:-inset-2 after:content-[""]',
          'group-focus-within:opacity-100 group-hover:opacity-100 aria-expanded:opacity-100',
          className,
        )}
        {...props}
      >
        <Ellipsis />
      </Button>
    );
  },
);
