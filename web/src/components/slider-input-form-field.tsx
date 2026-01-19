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
  numberInputClassName?: string;
  percentage?: boolean;
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
  numberInputClassName,
  layout = FormLayout.Horizontal,
  percentage = false,
}: SliderInputFormFieldProps) {
  const form = useFormContext();

  const isHorizontal = useMemo(() => layout !== FormLayout.Vertical, [layout]);
  const displayMax = percentage ? (max || 1) * 100 : max;
  const displayMin = percentage ? (min || 0) * 100 : min;
  const displayStep = percentage ? (step || 0.01) * 100 : step;
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
              'flex items-center gap-4 justify-between',
              { 'w-3/4': isHorizontal },
              className,
            )}
          >
            <FormControl>
              <SingleFormSlider
                {...field}
                value={percentage ? field.value * 100 : field.value}
                onChange={(value) =>
                  field.onChange(percentage ? value / 100 : value)
                }
                max={displayMax}
                min={displayMin}
                step={displayStep}
              ></SingleFormSlider>
            </FormControl>
            <FormControl>
              <NumberInput
                className={cn(
                  'h-6 w-10 p-0 text-center bg-bg-input border border-border-button text-text-secondary',
                  '[appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none',
                  numberInputClassName,
                )}
                max={displayMax}
                min={displayMin}
                step={displayStep}
                value={
                  percentage ? (field.value * 100).toFixed(0) : field.value
                }
                onChange={(val) => {
                  const value = Number(val || 0);
                  if (!isNaN(value)) {
                    field.onChange(
                      percentage ? (value / 100).toFixed(0) : value,
                    );
                  }
                }}
              ></NumberInput>
            </FormControl>
          </div>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
