import { BoolSegmented } from '@/components/bool-segmented';
import { LogicalOperator } from '@/components/logical-operator';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Separator } from '@/components/ui/separator';
import { ComparisonOperator, SwitchLogicOperator } from '@/constants/agent';
import { loader } from '@monaco-editor/react';
import { toLower } from 'lodash';
import { X } from 'lucide-react';
import { ReactNode, useCallback, useMemo } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import {
  AgentVariableType,
  InputMode,
  JsonSchemaDataType,
} from '../../constant';
import { useFilterChildNodeIds } from '../../hooks/use-filter-child-node-ids';
import { useGetVariableLabelOrTypeByValue } from '../../hooks/use-get-begin-query';
import { InputModeOptions } from '../../utils';
import { DynamicFormHeader } from '../components/dynamic-fom-header';
import { QueryVariable } from '../components/query-variable';
import { LoopFormSchemaType } from './schema';
import { useBuildLogicalOptions } from './use-build-logical-options';
import {
  ConditionKeyType,
  ConditionModeType,
  ConditionOperatorType,
  ConditionValueType,
  useInitializeConditions,
} from './use-watch-form-change';

loader.config({ paths: { vs: '/vs' } });

const VariablesExceptOperatorOutputs = [AgentVariableType.Conversation];

type LoopTerminationConditionProps = {
  label: ReactNode;
  tooltip?: string;
  keyField?: string;
  valueField?: string;
  operatorField?: string;
  modeField?: string;
  nodeId?: string;
};

const EmptyFields = [ComparisonOperator.Empty, ComparisonOperator.NotEmpty];

const LogicalOperatorFieldName = 'logical_operator';

const name = 'loop_termination_condition';

export function LoopTerminationCondition({
  label,
  tooltip,
  keyField = 'variable',
  valueField = 'value',
  operatorField = 'operator',
  modeField = 'input_mode',
  nodeId,
}: LoopTerminationConditionProps) {
  const form = useFormContext<LoopFormSchemaType>();
  const childNodeIds = useFilterChildNodeIds(nodeId);

  const nodeIds = useMemo(() => {
    if (!nodeId) return [];
    return [nodeId, ...childNodeIds];
  }, [childNodeIds, nodeId]);

  const { getType } = useGetVariableLabelOrTypeByValue({
    nodeIds: nodeIds,
    variablesExceptOperatorOutputs: VariablesExceptOperatorOutputs,
  });

  const {
    initializeConditionMode,
    initializeConditionOperator,
    initializeConditionValue,
  } = useInitializeConditions(nodeId);

  const { fields, remove, append } = useFieldArray({
    name: name,
    control: form.control,
  });

  const { buildLogicalOptions } = useBuildLogicalOptions();

  const getVariableType = useCallback(
    (keyFieldName: ConditionKeyType) => {
      const key = form.getValues(keyFieldName);
      return toLower(getType(key));
    },
    [form, getType],
  );

  const initializeMode = useCallback(
    (modeFieldAlias: ConditionModeType, keyFieldAlias: ConditionKeyType) => {
      const keyType = getVariableType(keyFieldAlias);

      initializeConditionMode(modeFieldAlias, keyType);
    },
    [getVariableType, initializeConditionMode],
  );

  const initializeValue = useCallback(
    (valueFieldAlias: ConditionValueType, keyFieldAlias: ConditionKeyType) => {
      const keyType = getVariableType(keyFieldAlias);

      initializeConditionValue(valueFieldAlias, keyType);
    },
    [getVariableType, initializeConditionValue],
  );

  const handleVariableChange = useCallback(
    (
      operatorFieldAlias: ConditionOperatorType,
      valueFieldAlias: ConditionValueType,
      keyFieldAlias: ConditionKeyType,
      modeFieldAlias: ConditionModeType,
    ) => {
      return () => {
        initializeConditionOperator(
          operatorFieldAlias,
          getVariableType(keyFieldAlias),
        );

        initializeMode(modeFieldAlias, keyFieldAlias);

        initializeValue(valueFieldAlias, keyFieldAlias);
      };
    },
    [
      getVariableType,
      initializeConditionOperator,
      initializeMode,
      initializeValue,
    ],
  );

  const handleOperatorChange = useCallback(
    (
      valueFieldAlias: ConditionValueType,
      keyFieldAlias: ConditionKeyType,
      modeFieldAlias: ConditionModeType,
    ) => {
      initializeMode(modeFieldAlias, keyFieldAlias);
      initializeValue(valueFieldAlias, keyFieldAlias);
    },
    [initializeMode, initializeValue],
  );

  const handleModeChange = useCallback(
    (mode: string, valueFieldAlias: ConditionValueType) => {
      form.setValue(valueFieldAlias, mode === InputMode.Constant ? 0 : '', {
        shouldDirty: true,
      });
    },
    [form],
  );

  const renderParameterPanel = useCallback(
    (
      keyFieldName: ConditionKeyType,
      valueFieldAlias: ConditionValueType,
      modeFieldAlias: ConditionModeType,
      operatorFieldAlias: ConditionOperatorType,
    ) => {
      const type = getVariableType(keyFieldName);
      const mode = form.getValues(modeFieldAlias);
      const operator = form.getValues(operatorFieldAlias);

      if (EmptyFields.includes(operator as ComparisonOperator)) {
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
            <BoolSegmented></BoolSegmented>
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
            const keyFieldAlias =
              `${name}.${index}.${keyField}` as ConditionKeyType;
            const valueFieldAlias =
              `${name}.${index}.${valueField}` as ConditionValueType;
            const operatorFieldAlias =
              `${name}.${index}.${operatorField}` as ConditionOperatorType;
            const modeFieldAlias =
              `${name}.${index}.${modeField}` as ConditionModeType;

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
                      nodeIds={nodeIds}
                      variablesExceptOperatorOutputs={
                        VariablesExceptOperatorOutputs
                      }
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
