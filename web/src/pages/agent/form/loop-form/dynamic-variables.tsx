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
import { InputMode, TypesWithArray } from '../../constant';
import {
  InputModeOptions,
  buildConversationVariableSelectOptions,
} from '../../utils';
import { DynamicFormHeader } from '../components/dynamic-fom-header';
import { QueryVariable } from '../components/query-variable';
import { useInitializeConditions } from './use-watch-form-change';

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

const modeField = 'input_mode';

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

export function DynamicVariables({
  name,
  label,
  tooltip,
  keyField = 'variable',
  valueField = 'value',
  operatorField = 'type',
  nodeId,
}: SelectKeysProps) {
  const form = useFormContext();
  const isDarkTheme = useIsDarkTheme();

  const { fields, remove, append } = useFieldArray({
    name: name,
    control: form.control,
  });

  const { initializeVariableRelatedConditions } =
    useInitializeConditions(nodeId);

  const initializeValue = useCallback(
    (mode: string, variableType: string, valueFieldAlias: string) => {
      if (mode === InputMode.Variable) {
        form.setValue(valueFieldAlias, '', { shouldDirty: true });
      } else {
        const val = ConstantValueMap[variableType as TypesWithArray];
        form.setValue(valueFieldAlias, val, { shouldDirty: true });
      }
    },
    [form],
  );

  const handleModeChange = useCallback(
    (mode: string, valueFieldAlias: string, operatorFieldAlias: string) => {
      const variableType = form.getValues(operatorFieldAlias);
      initializeValue(mode, variableType, valueFieldAlias);
    },
    [form, initializeValue],
  );

  const handleVariableTypeChange = useCallback(
    (
      variableType: string,
      valueFieldAlias: string,
      modeFieldAlias: string,
      keyFieldAlias: string,
    ) => {
      const mode = form.getValues(modeFieldAlias);

      initializeVariableRelatedConditions(
        form.getValues(keyFieldAlias),
        variableType,
      );

      initializeValue(mode, variableType, valueFieldAlias);
    },
    [form, initializeValue, initializeVariableRelatedConditions],
  );

  const renderParameter = useCallback(
    (operatorFieldName: string, modeFieldName: string) => {
      const mode = form.getValues(modeFieldName);
      const logicalOperator = form.getValues(operatorFieldName);

      if (mode === InputMode.Constant) {
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
      }

      return (
        <QueryVariable
          types={[logicalOperator]}
          hideLabel
          pureQuery
        ></QueryVariable>
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
            [modeField]: InputMode.Constant,
            [operatorField]: TypesWithArray.String,
          })
        }
      ></DynamicFormHeader>
      <div className="space-y-5">
        {fields.map((field, index) => {
          const keyFieldAlias = `${name}.${index}.${keyField}`;
          const valueFieldAlias = `${name}.${index}.${valueField}`;
          const operatorFieldAlias = `${name}.${index}.${operatorField}`;
          const modeFieldAlias = `${name}.${index}.${modeField}`;

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
                          handleVariableTypeChange(
                            val,
                            valueFieldAlias,
                            modeFieldAlias,
                            keyFieldAlias,
                          );
                          field.onChange(val);
                        }}
                        options={VariableTypeOptions}
                      ></SelectWithSearch>
                    )}
                  </RAGFlowFormItem>
                  <Separator className="w-2" />
                  <RAGFlowFormItem name={modeFieldAlias} className="flex-1">
                    {(field) => (
                      <SelectWithSearch
                        value={field.value}
                        onChange={(val) => {
                          handleModeChange(
                            val,
                            valueFieldAlias,
                            operatorFieldAlias,
                          );
                          field.onChange(val);
                        }}
                        options={InputModeOptions}
                      ></SelectWithSearch>
                    )}
                  </RAGFlowFormItem>
                </div>
                <RAGFlowFormItem name={valueFieldAlias} className="w-full">
                  {renderParameter(operatorFieldAlias, modeFieldAlias)}
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
