import { FormLayout } from '@/constants/form';
import { cn } from '@/lib/utils';
import { ReactNode } from 'react';
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
  layout = FormLayout.Vertical,
}: SliderInputFormFieldProps) {
  const form = useFormContext();

  const isHorizontal = layout === FormLayout.Horizontal;

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
              'text-sm text-muted-foreground whitespace-break-spaces w-1/4':
                isHorizontal,
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
                className="h-7 w-20"
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
