import { FormLayout } from '@/constants/form';
import { cn } from '@/lib/utils';
import { ReactNode, useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import { SingleFormSlider } from './ui/dual-range-slider';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from './ui/form';
import { NumberInput } from './ui/input';

export type FormLayoutType = {
  layout?: FormLayout;
};

type SliderInputFormFieldProps = {
  max?: number;
  min?: number;
  step?: number;
  name: string;
  label: string;
  tooltip?: ReactNode;
  defaultValue?: number;
  className?: string;
} & FormLayoutType;

export function SliderInputFormField({
  max,
  min,
  step,
  label,
  name,
  tooltip,
  defaultValue,
  className,
  layout = FormLayout.Horizontal,
}: SliderInputFormFieldProps) {
  const form = useFormContext();

  const isHorizontal = useMemo(() => layout !== FormLayout.Vertical, [layout]);

  return (
    <FormField
      control={form.control}
      name={name}
      defaultValue={defaultValue || 0}
      render={({ field }) => (
        <FormItem
          className={cn({ 'flex items-center gap-1 space-y-0': isHorizontal })}
        >
          <FormLabel
            tooltip={tooltip}
            className={cn({
              'text-sm whitespace-break-spaces w-1/4': isHorizontal,
            })}
          >
            {label}
          </FormLabel>
          <div
            className={cn(
              'flex items-center gap-14 justify-between',
              { 'w-3/4': isHorizontal },
              className,
            )}
          >
            <FormControl>
              <SingleFormSlider
                {...field}
                max={max}
                min={min}
                step={step}
                // defaultValue={
                //   typeof defaultValue === 'number' ? [defaultValue] : undefined
                // }
              ></SingleFormSlider>
            </FormControl>
            <FormControl>
              <NumberInput
                className={cn(
                  'h-6 w-10 p-0 text-center bg-bg-input border border-border-default text-text-secondary',
                  '[appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none',
                )}
                max={max}
                min={min}
                step={step}
                {...field}
                // defaultValue={defaultValue}
              ></NumberInput>
            </FormControl>
          </div>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
