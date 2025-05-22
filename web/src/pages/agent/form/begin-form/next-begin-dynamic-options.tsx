'use client';

import { Button } from '@/components/ui/button';
import {
  FormControl,
  FormField,
  FormItem,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Plus, X } from 'lucide-react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

export function BeginDynamicOptions() {
  const { t } = useTranslation();
  const form = useFormContext();
  const name = 'options';

  const { fields, remove, append } = useFieldArray({
    name: name,
    control: form.control,
  });

  return (
    <div className="space-y-5">
      {fields.map((field, index) => {
        const typeField = `${name}.${index}`;
        return (
          <div key={field.id} className="flex items-center gap-2">
            <FormField
              control={form.control}
              name={typeField}
              render={({ field }) => (
                <FormItem className="w-2/5">
                  <FormControl>
                    <Input
                      {...field}
                      placeholder={t('common.pleaseInput')}
                    ></Input>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <Button variant={'ghost'} onClick={() => remove(index)}>
              <X className="text-text-sub-title-invert " />
            </Button>
          </div>
        );
      })}
      <Button
        onClick={append}
        className="mt-4 border-dashed w-full"
        variant={'outline'}
      >
        <Plus />
        {t('flow.addVariable')}
      </Button>
    </div>
  );
}
