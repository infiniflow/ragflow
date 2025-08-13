import { Button } from '@/components/ui/button';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { BlurInput } from '@/components/ui/input';
import { Separator } from '@/components/ui/separator';
import { Switch } from '@/components/ui/switch';
import { Plus, X } from 'lucide-react';
import { useCallback } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

export function DynamicVariableForm() {
  const { t } = useTranslation();
  const form = useFormContext();
  const name = 'prompt_config.parameters';

  const { fields, remove, append } = useFieldArray({
    name,
    control: form.control,
  });

  const add = useCallback(() => {
    append({
      key: undefined,
      optional: false,
    });
  }, [append]);

  return (
    <section className="flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <FormLabel tooltip={t('chat.variableTip')}>
          {t('chat.variable')}
        </FormLabel>
        <Button variant={'ghost'} type="button" onClick={add}>
          <Plus />
        </Button>
      </div>
      <div className="space-y-5">
        {fields.map((field, index) => {
          const typeField = `${name}.${index}.key`;
          return (
            <div key={field.id} className="flex w-full items-center gap-2">
              <FormField
                control={form.control}
                name={typeField}
                render={({ field }) => (
                  <FormItem className="flex-1 overflow-hidden">
                    <FormControl>
                      <BlurInput
                        {...field}
                        placeholder={t('common.pleaseInput')}
                      ></BlurInput>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <Separator className="w-3 text-text-secondary" />
              <FormField
                control={form.control}
                name={`${name}.${index}.optional`}
                render={({ field }) => (
                  <FormItem className="flex-1 overflow-hidden">
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      ></Switch>
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
      </div>
    </section>
  );
}
