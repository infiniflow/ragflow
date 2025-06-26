'use client';

import { FormContainer } from '@/components/form-container';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { BlockButton, Button } from '@/components/ui/button';
import {
  FormControl,
  FormField,
  FormItem,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Separator } from '@/components/ui/separator';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { X } from 'lucide-react';
import { ReactNode } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useBuildSubNodeOutputOptions } from './use-build-options';

interface IProps {
  node?: RAGFlowNodeType;
}

export function DynamicOutputForm({ node }: IProps) {
  const { t } = useTranslation();
  const form = useFormContext();
  const options = useBuildSubNodeOutputOptions(node?.id);
  const name = 'outputs';

  const { fields, remove, append } = useFieldArray({
    name: name,
    control: form.control,
  });

  return (
    <div className="space-y-5">
      {fields.map((field, index) => {
        const typeField = `${name}.${index}.name`;
        return (
          <div key={field.id} className="flex items-center gap-2">
            <FormField
              control={form.control}
              name={typeField}
              render={({ field }) => (
                <FormItem className="flex-1">
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
            <Separator className="w-3 text-text-sub-title" />
            <FormField
              control={form.control}
              name={`${name}.${index}.ref`}
              render={({ field }) => (
                <FormItem className="w-2/5">
                  <FormControl>
                    <SelectWithSearch
                      options={options}
                      {...field}
                    ></SelectWithSearch>
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
      <BlockButton onClick={() => append({ name: '', ref: undefined })}>
        Add
      </BlockButton>
    </div>
  );
}

export function VariableTitle({ title }: { title: ReactNode }) {
  return <div className="font-medium text-text-title pb-2">{title}</div>;
}

export function DynamicOutput({ node }: IProps) {
  return (
    <FormContainer>
      <VariableTitle title={'Output'}></VariableTitle>
      <DynamicOutputForm node={node}></DynamicOutputForm>
    </FormContainer>
  );
}
