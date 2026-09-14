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
import { Separator } from '@/components/ui/separator';
import { LucidePlus, LucideTrash2 } from 'lucide-react';
import { useCallback } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

export function DynamicPageRange({ prefix }: CommonProps) {
  const { t } = useTranslation();
  const form = useFormContext();

  const pagesName = buildFieldNameWithPrefix('pages', prefix);

  const { fields, remove, append } = useFieldArray({
    name: pagesName,
    control: form.control,
  });

  const handleAppend = useCallback(() => {
    append({ from: 1, to: 100000 });
  }, [append]);

  return (
    <div>
      <FormLabel tooltip={t('knowledgeDetails.pageRangesTip')}>
        {t('knowledgeDetails.pageRanges')}
      </FormLabel>
      {fields.map((field, index) => {
        return (
          <div key={field.id} className="flex items-center gap-2 pt-2">
            <FormField
              control={form.control}
              name={`${pagesName}.${index}.from`}
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
              name={`${pagesName}.${index}.to`}
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

            <Button
              className="ml-4"
              size="icon"
              variant="outline"
              type="button"
              onClick={() => remove(index)}
            >
              <LucideTrash2 />
            </Button>
          </div>
        );
      })}

      <Button
        onClick={handleAppend}
        block
        className="mt-4"
        variant="dashed"
        type="button"
      >
        <LucidePlus />
        {t('knowledgeDetails.addPage')}
      </Button>
    </div>
  );
}
