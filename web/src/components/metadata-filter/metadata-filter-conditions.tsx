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
import { SwitchLogicOperator, SwitchOperatorOptions } from '@/constants/agent';
import { useBuildSwitchOperatorOptions } from '@/hooks/logic-hooks/use-build-operator-options';
import { useFetchKnowledgeMetadata } from '@/hooks/use-knowledge-request';
import { PromptEditor } from '@/pages/agent/form/components/prompt-editor';
import { Plus, X } from 'lucide-react';
import { useCallback } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { LogicalOperator } from '../logical-operator';

export function MetadataFilterConditions({
  kbIds,
  prefix = '',
  canReference,
}: {
  kbIds: string[];
  prefix?: string;
  canReference?: boolean;
}) {
  const { t } = useTranslation();
  const form = useFormContext();
  const name = prefix + 'meta_data_filter.manual';
  const logic = prefix + 'meta_data_filter.logic';
  const metadata = useFetchKnowledgeMetadata(kbIds);

  const switchOperatorOptions = useBuildSwitchOperatorOptions();

  const { fields, remove, append } = useFieldArray({
    name,
    control: form.control,
  });

  const add = useCallback(
    (key: string) => () => {
      if (fields.length === 1) {
        form.setValue(logic, SwitchLogicOperator.And);
      }
      append({
        key,
        value: '',
        op: SwitchOperatorOptions[0].value,
      });
    },
    [append, fields.length, form, logic],
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
          <DropdownMenuContent className="max-h-[300px] !overflow-y-auto scrollbar-auto">
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
      <section className="flex">
        {fields.length > 1 && <LogicalOperator name={logic}></LogicalOperator>}
        <div className="space-y-5 flex-1">
          {fields.map((field, index) => {
            const typeField = `${name}.${index}.key`;
            return (
              <section key={field.id} className="flex gap-2">
                <div className="w-full space-y-2">
                  <div className="flex items-center gap-1">
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
                    <Separator className="w-1 text-text-secondary" />
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
                  </div>
                  <FormField
                    control={form.control}
                    name={`${name}.${index}.value`}
                    render={({ field }) => (
                      <FormItem className="flex-1 overflow-hidden">
                        <FormControl>
                          {canReference ? (
                            <PromptEditor
                              {...field}
                              multiLine={false}
                              showToolbar={false}
                            ></PromptEditor>
                          ) : (
                            <Input
                              placeholder={t('common.pleaseInput')}
                              {...field}
                            />
                          )}
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
                <Button variant={'ghost'} onClick={() => remove(index)}>
                  <X className="text-text-sub-title-invert " />
                </Button>
              </section>
            );
          })}
        </div>
      </section>
    </section>
  );
}
