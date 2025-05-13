'use client';

import { Button } from '@/components/ui/button';
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Plus, X } from 'lucide-react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { Separator } from '../ui/separator';

export function DynamicPageRange() {
  const { t } = useTranslation();
  const form = useFormContext();

  const { fields, remove, append } = useFieldArray({
    name: 'parser_config.pages',
    control: form.control,
  });

  return (
    <div>
      <FormLabel tooltip={t('knowledgeDetails.pageRangesTip')}>
        {t('knowledgeDetails.pageRanges')}
      </FormLabel>
      {fields.map((field, index) => {
        const typeField = `parser_config.pages.${index}.from`;
        return (
          <div key={field.id} className="flex items-center gap-2 pt-2">
            <FormField
              control={form.control}
              name={typeField}
              render={({ field }) => (
                <FormItem className="w-2/5">
                  <FormDescription />
                  <FormControl>
                    <Input
                      type="number"
                      placeholder={t('common.pleaseInput')}
                      className="!m-0"
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <Separator className="w-3 "></Separator>
            <FormField
              control={form.control}
              name={`parser_config.pages.${index}.to`}
              render={({ field }) => (
                <FormItem className="flex-1">
                  <FormDescription />
                  <FormControl>
                    <Input
                      type="number"
                      placeholder={t('common.pleaseInput')}
                      className="!m-0"
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <Button variant={'ghost'} onClick={() => remove(index)}>
              <X />
            </Button>
          </div>
        );
      })}
      <Button
        onClick={() => append({ from: 1, to: 100 })}
        className="mt-4 border-dashed w-full"
        variant={'outline'}
        type="button"
      >
        <Plus />
        {t('knowledgeDetails.addPage')}
      </Button>
    </div>
  );
}
