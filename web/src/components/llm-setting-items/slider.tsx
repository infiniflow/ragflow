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

import { useTranslate } from '@/hooks/common-hooks';
import { cn } from '@/lib/utils';
import { forwardRef, useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import NumberInput from '../originui/number-input';
import { SingleFormSlider } from '../ui/dual-range-slider';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '../ui/form';
import { Switch } from '../ui/switch';

type SliderInputSwitchFormFieldProps = {
  max?: number;
  min?: number;
  step?: number;
  name: string;
  label: string;
  defaultValue?: number;
  onChange?: (value: number) => void;
  className?: string;
  checkName: string;
  numberInputClassName?: string;
};

export const SliderInputSwitchFormField = forwardRef<
  HTMLDivElement,
  SliderInputSwitchFormFieldProps
>(
  (
    {
      max,
      min,
      step,
      label,
      name,
      defaultValue,
      onChange,
      className,
      checkName,
      numberInputClassName,
    },
    ref,
  ) => {
    const form = useFormContext();
    const disabled = !form.watch(checkName);
    const { t } = useTranslate('chat');

    // The typed value must stay on the slider's grid: a step of 0.01 allows two
    // decimals, an integer step (or none) allows none.
    const precision = useMemo(
      () => String(step ?? '').split('.')[1]?.length ?? 0,
      [step],
    );

    return (
      <FormField
        control={form.control}
        name={name}
        defaultValue={defaultValue}
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t(`${label}Tip`)}>{t(label)}</FormLabel>
            <div
              ref={ref}
              className={cn(
                'flex items-center gap-4 justify-between',
                className,
              )}
            >
              <FormField
                control={form.control}
                name={checkName}
                render={({ field }) => (
                  <FormItem>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormControl>
                <SingleFormSlider
                  {...field}
                  onChange={(value: number) => {
                    onChange?.(value);
                    field.onChange(value);
                  }}
                  max={max}
                  min={min}
                  step={step}
                  disabled={disabled}
                ></SingleFormSlider>
              </FormControl>
              <FormControl>
                <NumberInput
                  disabled={disabled}
                  className={cn('h-6 w-16', numberInputClassName)}
                  max={max}
                  min={min}
                  step={step}
                  precision={precision}
                  {...field}
                  hideIcons
                  onChange={(value: number) => {
                    onChange?.(value);
                    field.onChange(value);
                  }}
                ></NumberInput>
              </FormControl>
            </div>
            <FormMessage />
          </FormItem>
        )}
      />
    );
  },
);

SliderInputSwitchFormField.displayName = 'SliderInputSwitchFormField';
