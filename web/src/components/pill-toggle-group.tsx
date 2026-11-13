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

import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group';
import { LucideIcon } from 'lucide-react';
import { ReactNode, useCallback } from 'react';

export interface PillToggleGroupOption {
  value: string;
  label: ReactNode;
  icon?: LucideIcon;
}

// Single-select pill-style tab group built on the shared ToggleGroup. Unlike
// the default joined button bar, each option renders as a separate rounded
// pill with a border; the active pill uses the card background.
export function PillToggleGroup({
  options,
  value,
  onChange,
  className,
}: {
  options: PillToggleGroupOption[];
  value: string;
  onChange: (value: string) => void;
  className?: string;
}) {
  // Radix single-mode emits '' when the selected item is clicked again;
  // a tab group must keep a selection, so the empty value is dropped here.
  const handleValueChange = useCallback(
    (nextValue: string) => {
      if (nextValue) {
        onChange(nextValue);
      }
    },
    [onChange],
  );

  return (
    <ToggleGroup
      type="single"
      value={value}
      onValueChange={handleValueChange}
      className={className ?? 'gap-3'}
    >
      {options.map((option) => (
        <ToggleGroupItem
          key={option.value}
          value={option.value}
          className="flex-initial rounded-full border-0.5 border-border-default px-4 text-text-disabled first:rounded-l-full last:rounded-r-full hover:text-text-primary data-[state=on]:bg-bg-card data-[state=on]:text-text-primary"
        >
          {option.icon ? (
            <span className="flex items-center gap-1.5">
              <option.icon className="size-4" />
              {option.label}
            </span>
          ) : (
            option.label
          )}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}
