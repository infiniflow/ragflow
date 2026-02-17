import { BoolSegmented } from '@/components/bool-segmented';
import { KeyInput } from '@/components/key-input';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { useIsDarkTheme } from '@/components/theme-provider';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Separator } from '@/components/ui/separator';
import { Textarea } from '@/components/ui/textarea';
import { Editor, loader } from '@monaco-editor/react';
import { X } from 'lucide-react';
import { ReactNode, useCallback } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { TypesWithArray } from '../../../constant';
import { buildConversationVariableSelectOptions } from '../../../utils';
import { DynamicFormHeader } from '../../components/dynamic-fom-header';

loader.config({ paths: { vs: '/vs' } });

type SelectKeysProps = {
  name: string;
  label: ReactNode;
  tooltip?: string;
  keyField?: string;
  valueField?: string;
  operatorField?: string;
  nodeId?: string;
};

const VariableTypeOptions = buildConversationVariableSelectOptions();

const ConstantValueMap = {
  [TypesWithArray.Boolean]: true,
  [TypesWithArray.Number]: 0,
  [TypesWithArray.String]: '',
  [TypesWithArray.ArrayBoolean]: '[]',
  [TypesWithArray.ArrayNumber]: '[]',
  [TypesWithArray.ArrayString]: '[]',
  [TypesWithArray.ArrayObject]: '[]',
  [TypesWithArray.Object]: '{}',
};

export function DynamicResponse({
  name,
  label,
  tooltip,
  keyField = 'key',
  valueField = 'value',
  operatorField = 'type',
}: SelectKeysProps) {
  const form = useFormContext();
  const isDarkTheme = useIsDarkTheme();

  const { fields, remove, append } = useFieldArray({
    name: name,
    control: form.control,
  });

  const initializeValue = useCallback(
    (variableType: string, valueFieldAlias: string) => {
      const val = ConstantValueMap[variableType as TypesWithArray];
      form.setValue(valueFieldAlias, val, { shouldDirty: true });
    },
    [form],
  );

  const handleVariableTypeChange = useCallback(
    (variableType: string, valueFieldAlias: string) => {
      initializeValue(variableType, valueFieldAlias);
    },
    [initializeValue],
  );

  const renderParameter = useCallback(
    (operatorFieldName: string) => {
      const logicalOperator = form.getValues(operatorFieldName);

      if (logicalOperator === TypesWithArray.Boolean) {
        return <BoolSegmented></BoolSegmented>;
      }

      if (logicalOperator === TypesWithArray.Number) {
        return <Input className="w-full" type="number"></Input>;
      }

      if (logicalOperator === TypesWithArray.String) {
        return <Textarea></Textarea>;
      }

      return (
        <Editor
          height={300}
          theme={isDarkTheme ? 'vs-dark' : 'vs'}
          language={'json'}
          options={{
            minimap: { enabled: false },
            automaticLayout: true,
          }}
        />
      );
    },
    [form, isDarkTheme],
  );

  return (
    <section className="space-y-2">
      <DynamicFormHeader
        label={label}
        tooltip={tooltip}
        onClick={() =>
          append({
            [keyField]: '',
            [valueField]: '',
            [operatorField]: TypesWithArray.String,
          })
        }
      ></DynamicFormHeader>
      <div className="space-y-5">
        {fields.map((field, index) => {
          const keyFieldAlias = `${name}.${index}.${keyField}`;
          const valueFieldAlias = `${name}.${index}.${valueField}`;
          const operatorFieldAlias = `${name}.${index}.${operatorField}`;

          return (
            <section key={field.id} className="flex gap-2">
              <div className="flex-1 space-y-3 min-w-0">
                <div className="flex items-center">
                  <RAGFlowFormItem name={keyFieldAlias} className="flex-1 ">
                    <KeyInput></KeyInput>
                  </RAGFlowFormItem>
                  <Separator className="w-2" />
                  <RAGFlowFormItem name={operatorFieldAlias} className="flex-1">
                    {(field) => (
                      <SelectWithSearch
                        value={field.value}
                        onChange={(val) => {
                          handleVariableTypeChange(val, valueFieldAlias);
                          field.onChange(val);
                        }}
                        options={VariableTypeOptions}
                      ></SelectWithSearch>
                    )}
                  </RAGFlowFormItem>
                  <Separator className="w-2" />
                </div>
                <RAGFlowFormItem name={valueFieldAlias} className="w-full">
                  {renderParameter(operatorFieldAlias)}
                </RAGFlowFormItem>
              </div>

              <Button variant={'ghost'} onClick={() => remove(index)}>
                <X />
              </Button>
            </section>
          );
        })}
      </div>
    </section>
  );
}
