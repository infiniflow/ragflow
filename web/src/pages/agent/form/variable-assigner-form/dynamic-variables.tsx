import NumberInput from '@/components/originui/number-input';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { useIsDarkTheme } from '@/components/theme-provider';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { Separator } from '@/components/ui/separator';
import { Textarea } from '@/components/ui/textarea';
import { Editor } from '@monaco-editor/react';
import * as RadioGroupPrimitive from '@radix-ui/react-radio-group';
import { X } from 'lucide-react';
import { ReactNode, useCallback } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import {
  JsonSchemaDataType,
  VariableAssignerLogicalArrayOperator,
  VariableAssignerLogicalNumberOperator,
  VariableAssignerLogicalOperator,
} from '../../constant';
import { useGetVariableLabelOrTypeByValue } from '../../hooks/use-get-begin-query';
import { DynamicFormHeader } from '../components/dynamic-fom-header';
import { QueryVariable } from '../components/query-variable';
import { useBuildLogicalOptions } from './use-build-logical-options';

type SelectKeysProps = {
  name: string;
  label: ReactNode;
  tooltip?: string;
  keyField?: string;
  valueField?: string;
  operatorField?: string;
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
      defaultValue="yes"
      className="flex"
      value={value}
      onValueChange={onChange}
    >
      <div className="flex items-center gap-3">
        <RadioGroupItem value="yes" id="r1" />
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
  VariableAssignerLogicalOperator.Clear,
  VariableAssignerLogicalArrayOperator.RemoveFirst,
  VariableAssignerLogicalArrayOperator.RemoveLast,
];

export function DynamicVariables({
  name,
  label,
  tooltip,
  keyField = 'variable',
  valueField = 'parameter',
  operatorField = 'operator',
}: SelectKeysProps) {
  const form = useFormContext();
  const { getType } = useGetVariableLabelOrTypeByValue();
  const isDarkTheme = useIsDarkTheme();

  const { fields, remove, append } = useFieldArray({
    name: name,
    control: form.control,
  });

  const { buildLogicalOptions } = useBuildLogicalOptions();

  const getVariableType = useCallback(
    (keyFieldName: string) => {
      const key = form.getValues(keyFieldName);
      return getType(key);
    },
    [form, getType],
  );

  const renderParameter = useCallback(
    (keyFieldName: string, operatorFieldName: string) => {
      const logicalOperator = form.getValues(operatorFieldName);
      const type = getVariableType(keyFieldName);

      if (EmptyFields.includes(logicalOperator)) {
        return null;
      } else if (
        logicalOperator === VariableAssignerLogicalOperator.Overwrite ||
        VariableAssignerLogicalArrayOperator.Extend === logicalOperator
      ) {
        return <QueryVariable types={[type]} hideLabel></QueryVariable>;
      } else if (logicalOperator === VariableAssignerLogicalOperator.Set) {
        if (type === JsonSchemaDataType.Boolean) {
          return <RadioButton></RadioButton>;
        }

        if (type === JsonSchemaDataType.Number) {
          return <NumberInput className="w-full"></NumberInput>;
        }

        if (type === JsonSchemaDataType.Object) {
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

        if (type === JsonSchemaDataType.String) {
          return <Textarea></Textarea>;
        }
      } else if (
        Object.values(VariableAssignerLogicalNumberOperator).some(
          (x) => logicalOperator === x,
        )
      ) {
        return <NumberInput className="w-full"></NumberInput>;
      } else if (
        logicalOperator === VariableAssignerLogicalArrayOperator.Append
      ) {
        const subType = type.match(/<([^>]+)>/).at(1);
        return <QueryVariable types={[subType]} hideLabel></QueryVariable>;
      }
    },
    [form, getVariableType, isDarkTheme],
  );

  const handleVariableChange = useCallback(
    (operatorFieldAlias: string, valueFieldAlias: string) => () => {
      form.setValue(
        operatorFieldAlias,
        VariableAssignerLogicalOperator.Overwrite,
        { shouldDirty: true, shouldValidate: true },
      );
      form.setValue(valueFieldAlias, undefined);
    },
    [form],
  );

  const handleOperatorChange = useCallback(
    (valueFieldAlias: string) => {
      form.setValue(valueFieldAlias, undefined, {
        shouldDirty: true,
        shouldValidate: true,
      });
    },
    [form],
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
                    )}
                  ></QueryVariable>

                  <Separator className="w-2" />

                  <RAGFlowFormItem name={operatorFieldAlias} className="w-1/3">
                    {({ onChange, value }) => (
                      <SelectWithSearch
                        value={value}
                        onChange={(val) => {
                          handleOperatorChange(valueFieldAlias);
                          onChange(val);
                        }}
                        options={buildLogicalOptions(
                          getVariableType(keyFieldAlias),
                        )}
                      ></SelectWithSearch>
                    )}
                  </RAGFlowFormItem>
                </div>
                <RAGFlowFormItem name={valueFieldAlias} className="w-full">
                  {renderParameter(keyFieldAlias, operatorFieldAlias)}
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
