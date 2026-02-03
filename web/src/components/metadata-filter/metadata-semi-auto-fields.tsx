import { Button } from '@/components/ui/button';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { useBuildSwitchOperatorOptions } from '@/hooks/logic-hooks/use-build-operator-options';
import { useFetchKnowledgeMetadata } from '@/hooks/use-knowledge-request';
import { Plus, X } from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { SelectWithSearch } from '../originui/select-with-search';

export function MetadataSemiAutoFields({
  kbIds,
  prefix = '',
}: {
  kbIds: string[];
  prefix?: string;
}) {
  const { t } = useTranslation();
  const form = useFormContext();
  const name = prefix + 'meta_data_filter.semi_auto';
  const metadata = useFetchKnowledgeMetadata(kbIds);

  const { fields, remove, append } = useFieldArray({
    name,
    control: form.control,
  });

  const add = useCallback(() => {
    append({ key: '', op: '' });
  }, [append]);

  const switchOperatorOptions = useBuildSwitchOperatorOptions();

  const autoOption = { label: t('chat.meta.auto'), value: '' };

  const metadataOptions = useMemo(() => {
    return Object.keys(metadata.data || {}).map((key) => ({
      label: key,
      value: key,
    }));
  }, [metadata.data]);

  return (
    <section className="flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <FormLabel>{t('chat.metadataKeys')}</FormLabel>
        <Button
          variant={'outline'}
          type="button"
          size="sm"
          onClick={add}
          className="h-8"
        >
          <Plus className="mr-2 size-4" />
          {t('common.add')}
        </Button>
      </div>
      <div className="space-y-2">
        {fields.map((field, index) => {
          const keyField = `${name}.${index}.key`;
          const opField = `${name}.${index}.op`;
          return (
            <section key={field.id} className="flex items-start gap-2">
              <FormField
                control={form.control}
                name={keyField}
                render={({ field }) => (
                  <FormItem className="flex-[2] overflow-hidden">
                    <FormControl>
                      <SelectWithSearch
                        {...field}
                        options={metadataOptions}
                        placeholder={t('common.pleaseSelect')}
                        triggerClassName="bg-bg-input"
                        value={field.value}
                        onChange={field.onChange}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name={opField}
                render={({ field }) => (
                  <FormItem className="flex-1">
                    <FormControl>
                      <SelectWithSearch
                        {...field}
                        options={[autoOption, ...switchOperatorOptions]}
                        triggerClassName="bg-bg-input"
                        value={field.value}
                        onChange={field.onChange}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <Button
                variant={'ghost'}
                size="icon"
                onClick={() => remove(index)}
                className="mt-0 h-8 w-10"
              >
                <X className="size-4 text-text-sub-title-invert" />
              </Button>
            </section>
          );
        })}
      </div>
    </section>
  );
}
