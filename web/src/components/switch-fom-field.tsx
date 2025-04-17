import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form';
import { Switch } from '@/components/ui/switch';
import { cn } from '@/lib/utils';
import { ReactNode } from 'react';
import { useFormContext } from 'react-hook-form';

interface SwitchFormItemProps {
  name: string;
  label: ReactNode;
  vertical?: boolean;
  tooltip?: ReactNode;
}

export function SwitchFormField({
  label,
  name,
  vertical = true,
  tooltip,
}: SwitchFormItemProps) {
  const form = useFormContext();

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
        >
          <FormLabel tooltip={tooltip}>{label}</FormLabel>
          <FormControl>
            <Switch
              checked={field.value}
              onCheckedChange={field.onChange}
              aria-readonly
              className="!m-0"
            />
          </FormControl>
        </FormItem>
      )}
    />
  );
}
