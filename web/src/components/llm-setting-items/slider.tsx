import { useTranslate } from '@/hooks/common-hooks';
import { cn } from '@/lib/utils';
import { useFormContext } from 'react-hook-form';
import { SingleFormSlider } from '../ui/dual-range-slider';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '../ui/form';
import { Input } from '../ui/input';
import { Switch } from '../ui/switch';

type SliderInputSwitchFormFieldProps = {
  max?: number;
  min?: number;
  step?: number;
  name: string;
  label: string;
  defaultValue?: number;
  className?: string;
  checkName: string;
};

export function SliderInputSwitchFormField({
  max,
  min,
  step,
  label,
  name,
  defaultValue,
  className,
  checkName,
}: SliderInputSwitchFormFieldProps) {
  const form = useFormContext();
  const disabled = !form.watch(checkName);
  const { t } = useTranslate('chat');

  return (
    <FormField
      control={form.control}
      name={name}
      defaultValue={defaultValue}
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={t(`${label}Tip`)}>{t(label)}</FormLabel>
          <div
            className={cn('flex items-center gap-4 justify-between', className)}
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
                max={max}
                min={min}
                step={step}
                disabled={disabled}
              ></SingleFormSlider>
            </FormControl>
            <FormControl>
              <Input
                disabled={disabled}
                type={'number'}
                className="h-7 w-20"
                max={max}
                min={min}
                step={step}
                {...field}
              ></Input>
            </FormControl>
          </div>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
