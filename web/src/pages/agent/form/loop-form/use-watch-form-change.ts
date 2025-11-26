import { JsonSchemaDataType } from '@/constants/agent';
import { buildVariableValue } from '@/utils/canvas-util';
import { useCallback, useEffect } from 'react';
import { UseFormReturn, useFormContext, useWatch } from 'react-hook-form';
import { InputMode } from '../../constant';
import { IOutputs } from '../../interface';
import useGraphStore from '../../store';
import { LoopFormSchemaType } from './schema';
import { useBuildLogicalOptions } from './use-build-logical-options';

export function useWatchFormChange(
  id?: string,
  form?: UseFormReturn<LoopFormSchemaType>,
) {
  let values = useWatch({ control: form?.control });
  const { replaceNodeForm } = useGraphStore((state) => state);

  useEffect(() => {
    if (id) {
      let nextValues = {
        ...values,
        outputs: values.loop_variables?.reduce((pre, cur) => {
          const variable = cur.variable;
          if (variable) {
            pre[variable] = {
              type: cur.type,
              value: '',
            };
          }
          return pre;
        }, {} as IOutputs),
      };

      replaceNodeForm(id, nextValues);
    }
  }, [form?.formState.isDirty, id, replaceNodeForm, values]);
}

type ConditionPrefixType = `loop_termination_condition.${number}.`;
export type ConditionKeyType = `${ConditionPrefixType}variable`;
export type ConditionModeType = `${ConditionPrefixType}input_mode`;
export type ConditionValueType = `${ConditionPrefixType}value`;
export type ConditionOperatorType = `${ConditionPrefixType}operator`;
export function useInitializeConditions(id?: string) {
  const form = useFormContext<LoopFormSchemaType>();
  const { buildLogicalOptions } = useBuildLogicalOptions();

  const initializeConditionMode = useCallback(
    (modeFieldAlias: ConditionModeType, keyType: string) => {
      if (keyType === JsonSchemaDataType.Number) {
        form.setValue(modeFieldAlias, InputMode.Constant, {
          shouldDirty: true,
          shouldValidate: true,
        });
      }
    },
    [form],
  );

  const initializeConditionValue = useCallback(
    (valueFieldAlias: ConditionValueType, keyType: string) => {
      let initialValue: string | boolean | number = '';

      if (keyType === JsonSchemaDataType.Number) {
        initialValue = 0;
      } else if (keyType === JsonSchemaDataType.Boolean) {
        initialValue = true;
      }

      form.setValue(valueFieldAlias, initialValue, {
        shouldDirty: true,
        shouldValidate: true,
      });
    },
    [form],
  );

  const initializeConditionOperator = useCallback(
    (operatorFieldAlias: ConditionOperatorType, keyType: string) => {
      const logicalOptions = buildLogicalOptions(keyType);

      form.setValue(operatorFieldAlias, logicalOptions?.at(0)?.value, {
        shouldDirty: true,
        shouldValidate: true,
      });
    },
    [buildLogicalOptions, form],
  );

  const initializeVariableRelatedConditions = useCallback(
    (variable: string, variableType: string) => {
      form?.getValues('loop_termination_condition').forEach((x, idx) => {
        if (variable && x.variable === buildVariableValue(variable, id)) {
          const prefix: ConditionPrefixType = `loop_termination_condition.${idx}.`;
          initializeConditionMode(`${prefix}input_mode`, variableType);
          initializeConditionValue(`${prefix}value`, variableType);
          initializeConditionOperator(`${prefix}operator`, variableType);
        }
      });
    },
    [
      form,
      id,
      initializeConditionMode,
      initializeConditionOperator,
      initializeConditionValue,
    ],
  );

  return {
    initializeVariableRelatedConditions,
    initializeConditionMode,
    initializeConditionValue,
    initializeConditionOperator,
  };
}
