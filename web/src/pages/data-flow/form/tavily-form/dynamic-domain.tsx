import { BlockButton, Button } from '@/components/ui/button';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { t } from 'i18next';
import { X } from 'lucide-react';
import { ReactNode } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';

type DynamicDomainProps = { name: string; label: ReactNode };

export const DynamicDomain = ({ name, label }: DynamicDomainProps) => {
  const form = useFormContext();

  const { fields, append, remove } = useFieldArray({
    name: name,
    control: form.control,
  });

  return (
    <FormItem>
      <FormLabel>{label}</FormLabel>
      <div className="space-y-4">
        {fields.map((field, index) => (
          <div key={field.id} className="flex">
            <div className="space-y-2 flex-1">
              <FormField
                control={form.control}
                name={`${name}.${index}.value`}
                render={({ field }) => (
                  <FormItem className="flex-1">
                    <FormControl>
                      <Input {...field}></Input>
                    </FormControl>
                  </FormItem>
                )}
              />
            </div>
            <Button
              type="button"
              variant={'ghost'}
              onClick={() => remove(index)}
            >
              <X />
            </Button>
          </div>
        ))}
      </div>
      <FormMessage />
      <BlockButton onClick={() => append({ value: '' })}>
        {t('common.add')}
      </BlockButton>
    </FormItem>
  );
};
