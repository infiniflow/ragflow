'use client';

import { FormContainer } from '@/components/form-container';
import { KeyInput } from '@/components/key-input';
import { BlockButton, Button } from '@/components/ui/button';
import {
  FormControl,
  FormField,
  FormItem,
  FormMessage,
} from '@/components/ui/form';
import { Separator } from '@/components/ui/separator';
import { Operator } from '@/constants/agent';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { t } from 'i18next';
import { isEmpty } from 'lodash';
import { X } from 'lucide-react';
import { ReactNode } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useGetVariableLabelOrTypeByValue } from '../../hooks/use-get-begin-query';
import useGraphStore from '../../store';
import { QueryVariable } from '../components/query-variable';

interface IProps {
  node?: RAGFlowNodeType;
}

export function DynamicOutputForm({ node }: IProps) {
  const { t } = useTranslation();
  const form = useFormContext();
  const { nodes } = useGraphStore((state) => state);

  const childNodeIds = nodes
    .filter(
      (x) =>
        x.parentId === node?.id &&
        x.data.label !== Operator.IterationStart &&
        !isEmpty(x.data?.form?.outputs),
    )
    .map((x) => x.id);

  const name = 'outputs';

  const { getType } = useGetVariableLabelOrTypeByValue({
    nodeIds: childNodeIds,
  });

  const { fields, remove, append } = useFieldArray({
    name: name,
    control: form.control,
  });

  return (
    <div className="space-y-5">
      {fields.map((field, index) => {
        const nameField = `${name}.${index}.name`;
        const typeField = `${name}.${index}.type`;
        return (
          <div key={field.id} className="flex items-center gap-2">
            <FormField
              control={form.control}
              name={nameField}
              render={({ field }) => (
                <FormItem className="flex-1">
                  <FormControl>
                    <KeyInput
                      {...field}
                      placeholder={t('common.pleaseInput')}
                    ></KeyInput>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <Separator className="w-3 text-text-secondary" />
            <QueryVariable
              name={`${name}.${index}.ref`}
              hideLabel
              className="w-2/5"
              onChange={(val) => {
                form.setValue(typeField, `Array<${getType(val)}>`);
              }}
              nodeIds={childNodeIds}
            ></QueryVariable>
            <FormField
              control={form.control}
              name={typeField}
              render={() => <div></div>}
            />
            <Button variant={'ghost'} onClick={() => remove(index)}>
              <X className="text-text-sub-title-invert " />
            </Button>
          </div>
        );
      })}
      <BlockButton onClick={() => append({ name: '', ref: undefined })}>
        {t('common.add')}
      </BlockButton>
    </div>
  );
}

export function VariableTitle({ title }: { title: ReactNode }) {
  return <div className="font-medium text-text-primary pb-2">{title}</div>;
}

export function DynamicOutput({ node }: IProps) {
  return (
    <FormContainer>
      <VariableTitle title={t('flow.output')}></VariableTitle>
      <DynamicOutputForm node={node}></DynamicOutputForm>
    </FormContainer>
  );
}
