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
    shouldUnregister: false,
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
        <Button
          variant="ghost"
          size="icon-sm"
          className="border-0"
          type="button"
          onClick={add}
        >
          <Plus />
        </Button>
      </div>

      <div className="grid grid-cols-[1fr_auto_auto_auto] items-center gap-2">
        <div className="contents text-text-secondary text-xs">
          <span>{t('chat.key')}</span>
          <span />
          <span>{t('chat.optional')}</span>
          <span />
        </div>

        <div className="grid grid-cols-subgrid items-center col-span-4 gap-y-4">
          {fields.map((field, index) => (
            <div key={field.id} className="contents">
              <FormField
                control={form.control}
                name={`${name}.${index}.key`}
                render={({ field }) => (
                  <FormItem className="flex-1">
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
                  <FormItem className="flex-1">
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

              <Button
                variant="ghost"
                size="icon-sm"
                className="border-0"
                type="button"
                onClick={() => remove(index)}
              >
                <X className="text-text-sub-title-invert " />
              </Button>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
