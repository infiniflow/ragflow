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
import { Input } from './ui/input';

type SliderInputFormFieldProps = {
  max?: number;
  min?: number;
  step?: number;
  name: string;
  label: string;
  tooltip?: ReactNode;
  defaultValue?: number;
  className?: string;
};

export function SliderInputFormField({
  max,
  min,
  step,
  label,
  name,
  tooltip,
  defaultValue,
  className,
}: SliderInputFormFieldProps) {
  const form = useFormContext();

  return (
    <FormField
      control={form.control}
      name={name}
      defaultValue={defaultValue}
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={tooltip}>{label}</FormLabel>
          <div
            className={cn(
              'flex items-center gap-14 justify-between',
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
              <Input
                type={'number'}
                className="h-7 w-20"
                max={max}
                min={min}
                step={step}
                {...field}
                // defaultValue={defaultValue}
              ></Input>
            </FormControl>
          </div>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
