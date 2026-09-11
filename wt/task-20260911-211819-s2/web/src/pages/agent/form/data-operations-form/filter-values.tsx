import { KeyInput } from '@/components/key-input';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import { useBuildSwitchOperatorOptions } from '@/hooks/logic-hooks/use-build-operator-options';
import { X } from 'lucide-react';
import { ReactNode } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { DataOperationsOperatorOptions } from '../../constant';
import { DynamicFormHeader } from '../components/dynamic-fom-header';
import { PromptEditor } from '../components/prompt-editor';

type SelectKeysProps = {
  name: string;
  label: ReactNode;
  tooltip?: string;
  keyField?: string;
  valueField?: string;
  operatorField?: string;
};
export function FilterValues({
  name,
  label,
  tooltip,
  keyField = 'key',
  valueField = 'value',
  operatorField = 'operator',
}: SelectKeysProps) {
  const form = useFormContext();

  const { fields, remove, append } = useFieldArray({
    name: name,
    control: form.control,
  });

  const operatorOptions = useBuildSwitchOperatorOptions(
    DataOperationsOperatorOptions,
  );

  return (
    <section className="space-y-2">
      <DynamicFormHeader
        label={label}
        tooltip={tooltip}
        onClick={() => append({ [keyField]: '', [valueField]: '' })}
      ></DynamicFormHeader>

      <div className="space-y-5">
        {fields.map((field, index) => {
          const keyFieldAlias = `${name}.${index}.${keyField}`;
          const valueFieldAlias = `${name}.${index}.${valueField}`;
          const operatorFieldAlias = `${name}.${index}.${operatorField}`;

          return (
            <div key={field.id} className="flex items-center gap-2">
              <RAGFlowFormItem name={keyFieldAlias} className="flex-1">
                <KeyInput></KeyInput>
              </RAGFlowFormItem>
              <Separator className="w-2" />

              <RAGFlowFormItem name={operatorFieldAlias} className="flex-1">
                <SelectWithSearch
                  {...field}
                  options={operatorOptions}
                ></SelectWithSearch>
              </RAGFlowFormItem>
              <Separator className="w-2" />

              <RAGFlowFormItem name={valueFieldAlias} className="flex-1">
                <PromptEditor showToolbar={false} multiLine={false} />
              </RAGFlowFormItem>
              <Button variant={'ghost'} onClick={() => remove(index)}>
                <X />
              </Button>
            </div>
          );
        })}
      </div>
    </section>
  );
}
