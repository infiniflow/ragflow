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
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { cn } from '@/lib/utils';
import { ArrowUpRight } from 'lucide-react';
import { ReactNode, cloneElement, isValidElement } from 'react';
import {
  ControllerRenderProps,
  UseControllerProps,
  useFormContext,
} from 'react-hook-form';

type RAGFlowFormItemProps = {
  name: string;
  label?: ReactNode;
  tooltip?: ReactNode;
  description?: ReactNode;
  children: ReactNode | ((field: ControllerRenderProps) => ReactNode);
  horizontal?: boolean;
  required?: boolean;
  labelClassName?: string;
  valueClassName?: string;
  className?: string;
  labelLink?: {
    text: ReactNode;
    onClick: () => void;
  };
} & Pick<UseControllerProps<any>, 'rules'>;

export function RAGFlowFormItem({
  name,
  label,
  tooltip,
  description,
  children,
  horizontal = false,
  required = false,
  labelClassName,
  valueClassName,
  className,
  rules,
  labelLink,
}: RAGFlowFormItemProps) {
  const form = useFormContext();
  return (
    <FormField
      control={form.control}
      rules={rules}
      name={name}
      render={({ field }) => (
        <FormItem
          className={cn(
            {
              'flex items-center w-full space-y-0 justify-between': horizontal,
            },
            className,
          )}
        >
          {label && (
            <div className={cn('flex items-center gap-2 justify-between')}>
              <FormLabel
                required={required}
                tooltip={tooltip}
                className={cn(labelClassName)}
              >
                {label}
              </FormLabel>
              {labelLink && (
                <div
                  className="text-sm flex text-text-primary cursor-pointer items-center shrink-0"
                  onClick={labelLink.onClick}
                >
                  {labelLink.text}
                  <ArrowUpRight size={14} />
                </div>
              )}
            </div>
          )}
          <div
            className={cn(
              'flex flex-col',
              {
                'w-full': !horizontal,
                'w-3/4': horizontal,
              },
              valueClassName,
            )}
          >
            <FormControl>
              {typeof children === 'function'
                ? children(field)
                : isValidElement(children)
                  ? cloneElement(children, { ...field })
                  : children}
            </FormControl>
            {description && <FormDescription>{description}</FormDescription>}
            <FormMessage />
          </div>
        </FormItem>
      )}
    />
  );
}
