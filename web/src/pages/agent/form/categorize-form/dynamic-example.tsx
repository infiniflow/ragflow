import { Button } from '@/components/ui/button';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Textarea } from '@/components/ui/textarea';
import { Plus, X } from 'lucide-react';
import { memo } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

type DynamicExampleProps = { name: string };

const DynamicExample = ({ name }: DynamicExampleProps) => {
  const { t } = useTranslation();
  const form = useFormContext();

  const { fields, append, remove } = useFieldArray({
    name: name,
    control: form.control,
  });

  return (
    <FormItem>
      <FormLabel tooltip={t('flow.msgTip')}>{t('flow.examples')}</FormLabel>
      <div className="space-y-4">
        {fields.map((field, index) => (
          <div key={field.id} className="flex items-start gap-2">
            <FormField
              control={form.control}
              name={`${name}.${index}.value`}
              render={({ field }) => (
                <FormItem className="flex-1">
                  <FormControl>
                    <Textarea {...field}> </Textarea>
                  </FormControl>
                </FormItem>
              )}
            />
            {index === 0 ? (
              <Button
                type="button"
                variant={'ghost'}
                onClick={() => append({ value: '' })}
              >
                <Plus />
              </Button>
            ) : (
              <Button
                type="button"
                variant={'ghost'}
                onClick={() => remove(index)}
              >
                <X />
              </Button>
            )}
          </div>
        ))}
      </div>
      <FormMessage />
    </FormItem>
  );
};

export default memo(DynamicExample);
