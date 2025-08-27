import { SelectWithSearch } from '@/components/originui/select-with-search';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Separator } from '@/components/ui/separator';
import { useFetchKnowledgeMetadata } from '@/hooks/use-knowledge-request';
import { SwitchOperatorOptions } from '@/pages/agent/constant';
import { useBuildSwitchOperatorOptions } from '@/pages/agent/form/switch-form';
import { Plus, X } from 'lucide-react';
import { useCallback } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

export function MetadataFilterConditions({
  kbIds,
  prefix = '',
}: {
  kbIds: string[];
  prefix?: string;
}) {
  const { t } = useTranslation();
  const form = useFormContext();
  const name = prefix + 'meta_data_filter.manual';
  const metadata = useFetchKnowledgeMetadata(kbIds);

  const switchOperatorOptions = useBuildSwitchOperatorOptions();

  const { fields, remove, append } = useFieldArray({
    name,
    control: form.control,
  });

  const add = useCallback(
    (key: string) => () => {
      append({
        key,
        value: '',
        op: SwitchOperatorOptions[0].value,
      });
    },
    [append],
  );

  return (
    <section className="flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <FormLabel>{t('chat.conditions')}</FormLabel>
        <DropdownMenu>
          <DropdownMenuTrigger>
            <Button variant={'ghost'} type="button">
              <Plus />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            {Object.keys(metadata.data).map((key, idx) => {
              return (
                <DropdownMenuItem key={idx} onClick={add(key)}>
                  {key}
                </DropdownMenuItem>
              );
            })}
          </DropdownMenuContent>
        </DropdownMenu>
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
                      <Input
                        {...field}
                        placeholder={t('common.pleaseInput')}
                      ></Input>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <Separator className="w-3 text-text-secondary" />
              <FormField
                control={form.control}
                name={`${name}.${index}.op`}
                render={({ field }) => (
                  <FormItem className="flex-1 overflow-hidden">
                    <FormControl>
                      <SelectWithSearch
                        {...field}
                        options={switchOperatorOptions}
                      ></SelectWithSearch>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <Separator className="w-3 text-text-secondary" />
              <FormField
                control={form.control}
                name={`${name}.${index}.value`}
                render={({ field }) => (
                  <FormItem className="flex-1 overflow-hidden">
                    <FormControl>
                      <Input placeholder={t('common.pleaseInput')} {...field} />
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
