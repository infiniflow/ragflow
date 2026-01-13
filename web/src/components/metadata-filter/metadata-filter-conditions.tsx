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
import { useCallback, useMemo } from 'react';
import { useFieldArray, useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { LogicalOperator } from '../logical-operator';
import { InputSelect } from '../ui/input-select';

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

  const RenderField = ({
    fieldName,
    index,
  }: {
    fieldName: string;
    index: number;
  }) => {
    const form = useFormContext();
    const key = useWatch({ name: fieldName });
    const valueOptions = useMemo(() => {
      if (!key || !metadata?.data || !metadata?.data[key]) return [];
      if (typeof metadata?.data[key] === 'object') {
        return Object.keys(metadata?.data[key]).map((item: string) => ({
          value: item,
          label: item,
        }));
      }
      return [];
    }, [key]);

    return (
      <section className="flex gap-2">
        <div className="flex-1 flex flex-col gap-2 min-w-0">
          <div className="flex items-center gap-1">
            <FormField
              control={form.control}
              name={fieldName}
              render={({ field }) => (
                <FormItem className="flex-1 overflow-hidden min-w-0">
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
                <FormItem className="flex-1 overflow-hidden min-w-0">
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
            render={({ field: valueField }) => (
              <FormItem className="flex-1 overflow-hidden min-w-0">
                <FormControl>
                  {canReference ? (
                    <PromptEditor
                      {...valueField}
                      multiLine={false}
                      showToolbar={false}
                    ></PromptEditor>
                  ) : (
                    <InputSelect
                      placeholder={t('common.pleaseInput')}
                      {...valueField}
                      options={valueOptions}
                      className="w-full"
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
  };
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
        <div className="space-y-5 flex-1 w-[calc(100%-56px)]">
          {fields.map((field, index) => {
            const typeField = `${name}.${index}.key`;
            return (
              <RenderField key={field.id} fieldName={typeField} index={index} />
            );
          })}
        </div>
      </section>
    </section>
  );
}
