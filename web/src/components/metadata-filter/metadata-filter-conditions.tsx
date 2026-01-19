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
import { cn } from '@/lib/utils';
import { PromptEditor } from '@/pages/agent/form/components/prompt-editor';
import { Plus, X } from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useFieldArray, useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { LogicalOperator } from '../logical-operator';
import { Card, CardContent } from '../ui/card';
import { InputSelect } from '../ui/input-select';
import { RAGFlowSelect } from '../ui/select';

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

  function ConditionCards({
    fieldName,
    index,
  }: {
    fieldName: string;
    index: number;
  }) {
    const { t } = useTranslation();
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
      <div className="flex gap-1">
        <Card
          className={cn(
            'relative bg-transparent border-input-border border flex-1 min-w-0',
          )}
        >
          <section className="p-2 bg-bg-card flex justify-between items-center">
            <FormField
              control={form.control}
              name={fieldName}
              render={({ field }) => (
                <FormItem className="flex-1 min-w-0">
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
            <div className="flex items-center">
              <Separator orientation="vertical" className="h-2.5" />
              <FormField
                control={form.control}
                name={`${name}.${index}.op`}
                render={({ field }) => (
                  <FormItem>
                    <FormControl>
                      <RAGFlowSelect
                        {...field}
                        options={switchOperatorOptions}
                        onlyShowSelectedIcon
                        triggerClassName="w-30 bg-transparent border-none"
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          </section>
          <CardContent className="p-4 ">
            <FormField
              control={form.control}
              name={`${name}.${index}.value`}
              render={({ field: valueField }) => (
                <FormItem>
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
          </CardContent>
        </Card>
        <Button variant={'ghost'} onClick={() => remove(index)}>
          <X />
        </Button>
      </div>
    );
  }
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
              <ConditionCards
                key={field.id}
                fieldName={typeField}
                index={index}
              />
            );
          })}
        </div>
      </section>
    </section>
  );
}
