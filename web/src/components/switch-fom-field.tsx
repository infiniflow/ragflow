import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form';
import { Switch } from '@/components/ui/switch';
import { ReactNode } from 'react';
import { useFormContext } from 'react-hook-form';

interface SwitchFormItemProps {
  name: string;
  label: ReactNode;
}

export function SwitchFormField({ label, name }: SwitchFormItemProps) {
  const form = useFormContext();

  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem className="flex justify-between">
          <FormLabel className="text-base">{label}</FormLabel>
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
