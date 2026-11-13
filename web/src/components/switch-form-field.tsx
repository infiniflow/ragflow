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
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Switch } from '@/components/ui/switch';
import { cn } from '@/lib/utils';
import { MouseEventHandler, ReactNode, useCallback } from 'react';
import { useFormContext } from 'react-hook-form';

interface SwitchFormItemProps {
  name: string;
  label: ReactNode;
  vertical?: boolean;
  tooltip?: ReactNode;
  shouldStopPropagation?: boolean;
  disabled?: boolean;
}

export function SwitchFormField({
  label,
  name,
  vertical = true,
  tooltip,
  shouldStopPropagation = false,
  disabled = false,
}: SwitchFormItemProps) {
  const form = useFormContext();

  const handleClick: MouseEventHandler = useCallback(
    (e) => {
      if (shouldStopPropagation) {
        e.stopPropagation();
      }
    },
    [shouldStopPropagation],
  );

  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem
          className={cn('flex', {
            'gap-2': vertical,
            'flex-col': vertical,
            'justify-between': !vertical,
          })}
          onClick={handleClick}
        >
          <FormLabel className="w-fit" tooltip={tooltip}>
            {label}
          </FormLabel>
          <FormControl>
            <Switch
              checked={field.value}
              disabled={disabled}
              onCheckedChange={field.onChange}
              aria-readonly
              className="!m-0"
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
