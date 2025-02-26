'use client';

import { Button } from '@/components/ui/button';
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
import { CircleMinus, Plus } from 'lucide-react';
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
                  {/* <FormLabel>City</FormLabel> */}
                  <FormDescription />
                  <FormControl>
                    <RAGFlowSelect
                      // placeholder={t('common.pleaseSelect')}
                      {...field}
                      options={options}
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
                <FormItem>
                  {/* <FormLabel>State</FormLabel> */}
                  <FormDescription />
                  <FormControl>
                    {typeValue === VariableType.Reference ? (
                      <RAGFlowSelect
                        // placeholder={t('common.pleaseSelect')}
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
            <CircleMinus
              className="cursor-pointer"
              onClick={() => remove(index)}
            />
          </div>
        );
      })}
      <Button onClick={append} className="w-full mt-4">
        <Plus />
        {t('flow.addVariable')}
      </Button>
    </div>
  );
}
