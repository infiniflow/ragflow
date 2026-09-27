import { BlockButton, Button } from '@/components/ui/button';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { X } from 'lucide-react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

type DynamicListProps = {
  name:
    | 'site_include'
    | 'site_exclude'
    | 'country_include'
    | 'language_include';
  label: string;
  tooltip: string;
  placeholder: string;
};

export function DynamicList({
  name,
  label,
  tooltip,
  placeholder,
}: DynamicListProps) {
  const { t } = useTranslation();
  const form = useFormContext();
  const { fields, append, remove } = useFieldArray({
    name,
    control: form.control,
  });

  return (
    <FormItem>
      <FormLabel tooltip={tooltip}>{label}</FormLabel>
      <div className="space-y-4">
        {fields.map((item, index) => (
          <div key={item.id} className="flex items-start gap-2">
            <FormField
              control={form.control}
              name={`${name}.${index}.value`}
              render={({ field }) => (
                <FormItem className="flex-1">
                  <FormControl>
                    <Input {...field} placeholder={placeholder} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={`${t('common.remove')} ${label} ${index + 1}`}
              onClick={() => remove(index)}
            >
              <X />
            </Button>
          </div>
        ))}
      </div>
      <BlockButton type="button" onClick={() => append({ value: '' })}>
        {t('common.add')}
      </BlockButton>
    </FormItem>
  );
}
