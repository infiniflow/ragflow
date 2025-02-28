'use client';

import { SideDown } from '@/assets/icon/Icon';
import { Button } from '@/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { RAGFlowSelect } from '@/components/ui/select';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { Plus, Trash2 } from 'lucide-react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useBuildComponentIdSelectOptions } from '../../hooks/use-get-begin-query';

interface IProps {
  node?: RAGFlowNodeType;
}

enum VariableType {
  Reference = 'reference',
  Input = 'input',
}

const getVariableName = (type: string) =>
  type === VariableType.Reference ? 'component_id' : 'value';

export function DynamicVariableForm({ node }: IProps) {
  const { t } = useTranslation();
  const form = useFormContext();
  const { fields, remove, append } = useFieldArray({
    name: 'query',
    control: form.control,
  });

  const valueOptions = useBuildComponentIdSelectOptions(
    node?.id,
    node?.parentId,
  );

  const options = [
    { value: VariableType.Reference, label: t('flow.reference') },
    { value: VariableType.Input, label: t('flow.text') },
  ];

  return (
    <div>
      {fields.map((field, index) => {
        const typeField = `query.${index}.type`;
        const typeValue = form.watch(typeField);
        return (
          <div key={field.id} className="flex items-center gap-1">
            <FormField
              control={form.control}
              name={typeField}
              render={({ field }) => (
                <FormItem className="w-2/5">
                  <FormDescription />
                  <FormControl>
                    <RAGFlowSelect
                      {...field}
                      placeholder={t('common.pleaseSelect')}
                      options={options}
                      onChange={(val) => {
                        field.onChange(val);
                        form.resetField(`query.${index}.value`);
                        form.resetField(`query.${index}.component_id`);
                      }}
                    ></RAGFlowSelect>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name={`query.${index}.${getVariableName(typeValue)}`}
              render={({ field }) => (
                <FormItem className="flex-1">
                  <FormDescription />
                  <FormControl>
                    {typeValue === VariableType.Reference ? (
                      <RAGFlowSelect
                        placeholder={t('common.pleaseSelect')}
                        {...field}
                        options={valueOptions}
                      ></RAGFlowSelect>
                    ) : (
                      <Input placeholder={t('common.pleaseInput')} {...field} />
                    )}
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <Trash2
              className="cursor-pointer mx-3 size-4 text-colors-text-functional-danger"
              onClick={() => remove(index)}
            />
          </div>
        );
      })}
      <Button onClick={append} className="mt-4" variant={'outline'} size={'sm'}>
        <Plus />
        {t('flow.addVariable')}
      </Button>
    </div>
  );
}

export function DynamicInputVariable({ node }: IProps) {
  const { t } = useTranslation();

  return (
    <Collapsible defaultOpen className="group/collapsible">
      <CollapsibleTrigger className="flex justify-between w-full pb-2">
        <span className="font-bold text-2xl text-colors-text-neutral-strong">
          {t('flow.input')}
        </span>
        <Button variant={'icon'} size={'icon'}>
          <SideDown />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <DynamicVariableForm node={node}></DynamicVariableForm>
      </CollapsibleContent>
    </Collapsible>
  );
}
