import { LogicalOperator } from '@/components/logical-operator';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { Separator } from '@/components/ui/separator';
import { SwitchLogicOperator } from '@/constants/agent';
import { loader } from '@monaco-editor/react';
import * as RadioGroupPrimitive from '@radix-ui/react-radio-group';
import { toLower } from 'lodash';
import { X } from 'lucide-react';
import { ReactNode, useCallback } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import {
  InputMode,
  JsonSchemaDataType,
  LoopTerminationComparisonOperator,
  RadioVariable,
} from '../../constant';
import { useGetVariableLabelOrTypeByValue } from '../../hooks/use-get-begin-query';
import { InputModeOptions } from '../../utils';
import { DynamicFormHeader } from '../components/dynamic-fom-header';
import { QueryVariable } from '../components/query-variable';
import { useBuildLogicalOptions } from './use-build-logical-options';

loader.config({ paths: { vs: '/vs' } });

type SelectKeysProps = {
  name: string;
  label: ReactNode;
  tooltip?: string;
  keyField?: string;
  valueField?: string;
  operatorField?: string;
  modeField?: string;
};

type RadioGroupProps = React.ComponentProps<typeof RadioGroupPrimitive.Root>;

type RadioButtonProps = Partial<
  Omit<RadioGroupProps, 'onValueChange'> & {
    onChange: RadioGroupProps['onValueChange'];
  }
>;

function RadioButton({ value, onChange }: RadioButtonProps) {
  return (
    <RadioGroup
      defaultValue={RadioVariable.Yes}
      className="flex"
      value={value}
      onValueChange={onChange}
    >
      <div className="flex items-center gap-3">
        <RadioGroupItem value={RadioVariable.Yes} id="r1" />
        <Label htmlFor="r1">Yes</Label>
      </div>
      <div className="flex items-center gap-3">
        <RadioGroupItem value="no" id="r2" />
        <Label htmlFor="r2">No</Label>
      </div>
    </RadioGroup>
  );
}

const EmptyFields = [
  LoopTerminationComparisonOperator.IsEmpty,
  LoopTerminationComparisonOperator.IsNotEmpty,
];

const LogicalOperatorFieldName = 'logical_operator';

export function LoopTerminationCondition({
  name,
  label,
  tooltip,
  keyField = 'variable',
  valueField = 'value',
  operatorField = 'operator',
  modeField = 'input_mode',
}: SelectKeysProps) {
  const form = useFormContext();
  const { getType } = useGetVariableLabelOrTypeByValue();

  const { fields, remove, append } = useFieldArray({
    name: name,
    control: form.control,
  });

  const { buildLogicalOptions } = useBuildLogicalOptions();

  const getVariableType = useCallback(
    (keyFieldName: string) => {
      const key = form.getValues(keyFieldName);
      return toLower(getType(key));
    },
    [form, getType],
  );

  const initializeMode = useCallback(
    (modeFieldAlias: string, keyFieldAlias: string) => {
      const keyType = getVariableType(keyFieldAlias);

      if (keyType === JsonSchemaDataType.Number) {
        form.setValue(modeFieldAlias, InputMode.Constant, {
          shouldDirty: true,
          shouldValidate: true,
        });
      }
    },
    [form, getVariableType],
  );

  const initializeValue = useCallback(
    (valueFieldAlias: string, keyFieldAlias: string) => {
      const keyType = getVariableType(keyFieldAlias);
      let initialValue: string | boolean | number = '';

      if (keyType === JsonSchemaDataType.Number) {
        initialValue = 0;
      } else if (keyType === JsonSchemaDataType.Boolean) {
        initialValue = RadioVariable.Yes;
      }

      form.setValue(valueFieldAlias, initialValue, {
        shouldDirty: true,
        shouldValidate: true,
      });
    },
    [form, getVariableType],
  );

  const handleVariableChange = useCallback(
    (
      operatorFieldAlias: string,
      valueFieldAlias: string,
      keyFieldAlias: string,
      modeFieldAlias: string,
    ) => {
      return () => {
        const logicalOptions = buildLogicalOptions(
          getVariableType(keyFieldAlias),
        );

        form.setValue(operatorFieldAlias, logicalOptions?.at(0)?.value, {
          shouldDirty: true,
          shouldValidate: true,
        });

        initializeMode(modeFieldAlias, keyFieldAlias);

        initializeValue(valueFieldAlias, keyFieldAlias);
      };
    },
    [
      buildLogicalOptions,
      form,
      getVariableType,
      initializeMode,
      initializeValue,
    ],
  );

  const handleOperatorChange = useCallback(
    (
      valueFieldAlias: string,
      keyFieldAlias: string,
      modeFieldAlias: string,
    ) => {
      initializeMode(modeFieldAlias, keyFieldAlias);
      initializeValue(valueFieldAlias, keyFieldAlias);
    },
    [initializeMode, initializeValue],
  );

  const handleModeChange = useCallback(
    (mode: string, valueFieldAlias: string) => {
      form.setValue(valueFieldAlias, mode === InputMode.Constant ? 0 : '', {
        shouldDirty: true,
      });
    },
    [form],
  );

  const renderParameterPanel = useCallback(
    (
      keyFieldName: string,
      valueFieldAlias: string,
      modeFieldAlias: string,
      operatorFieldAlias: string,
    ) => {
      const type = getVariableType(keyFieldName);
      const mode = form.getValues(modeFieldAlias);
      const operator = form.getValues(operatorFieldAlias);

      if (EmptyFields.includes(operator)) {
        return null;
      }

      if (type === JsonSchemaDataType.Number) {
        return (
          <section className="flex items-center gap-1">
            <RAGFlowFormItem name={modeFieldAlias}>
              {(field) => (
                <SelectWithSearch
                  value={field.value}
                  onChange={(val) => {
                    handleModeChange(val, valueFieldAlias);
                    field.onChange(val);
                  }}
                  options={InputModeOptions}
                ></SelectWithSearch>
              )}
            </RAGFlowFormItem>
            <Separator className="w-2" />
            {mode === InputMode.Constant ? (
              <RAGFlowFormItem name={valueFieldAlias}>
                <Input type="number" />
              </RAGFlowFormItem>
            ) : (
              <QueryVariable
                types={[JsonSchemaDataType.Number]}
                hideLabel
                pureQuery
                name={valueFieldAlias}
                className="flex-1 min-w-0"
              ></QueryVariable>
            )}
          </section>
        );
      }

      if (type === JsonSchemaDataType.Boolean) {
        return (
          <RAGFlowFormItem name={valueFieldAlias} className="w-full">
            <RadioButton></RadioButton>
          </RAGFlowFormItem>
        );
      }

      return (
        <RAGFlowFormItem name={valueFieldAlias} className="w-full">
          <Input />
        </RAGFlowFormItem>
      );
    },
    [form, getVariableType, handleModeChange],
  );

  return (
    <section className="space-y-2">
      <DynamicFormHeader
        label={label}
        tooltip={tooltip}
        onClick={() => {
          if (fields.length === 1) {
            form.setValue(LogicalOperatorFieldName, SwitchLogicOperator.And);
          }
          append({ [keyField]: '', [valueField]: '' });
        }}
      ></DynamicFormHeader>
      <section className="flex">
        {fields.length > 1 && (
          <LogicalOperator name={LogicalOperatorFieldName}></LogicalOperator>
        )}
        <div className="space-y-5 flex-1 min-w-0">
          {fields.map((field, index) => {
            const keyFieldAlias = `${name}.${index}.${keyField}`;
            const valueFieldAlias = `${name}.${index}.${valueField}`;
            const operatorFieldAlias = `${name}.${index}.${operatorField}`;
            const modeFieldAlias = `${name}.${index}.${modeField}`;

            return (
              <section key={field.id} className="flex gap-2">
                <div className="flex-1 space-y-3 min-w-0">
                  <div className="flex items-center">
                    <QueryVariable
                      name={keyFieldAlias}
                      hideLabel
                      className="flex-1 min-w-0"
                      onChange={handleVariableChange(
                        operatorFieldAlias,
                        valueFieldAlias,
                        keyFieldAlias,
                        modeFieldAlias,
                      )}
                    ></QueryVariable>

                    <Separator className="w-2" />

                    <RAGFlowFormItem
                      name={operatorFieldAlias}
                      className="w-1/3"
                    >
                      {({ onChange, value }) => (
                        <SelectWithSearch
                          value={value}
                          onChange={(val) => {
                            handleOperatorChange(
                              valueFieldAlias,
                              keyFieldAlias,
                              modeFieldAlias,
                            );
                            onChange(val);
                          }}
                          options={buildLogicalOptions(
                            getVariableType(keyFieldAlias),
                          )}
                        ></SelectWithSearch>
                      )}
                    </RAGFlowFormItem>
                  </div>
                  {renderParameterPanel(
                    keyFieldAlias,
                    valueFieldAlias,
                    modeFieldAlias,
                    operatorFieldAlias,
                  )}
                </div>

                <Button variant={'ghost'} onClick={() => remove(index)}>
                  <X />
                </Button>
              </section>
            );
          })}
        </div>
      </section>
    </section>
  );
}
