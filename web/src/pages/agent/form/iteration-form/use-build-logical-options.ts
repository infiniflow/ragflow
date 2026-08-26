import { buildOptions } from '@/utils/form';
import { camelCase } from 'lodash';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  JsonSchemaDataType,
  VariableAssignerLogicalArrayOperator,
  VariableAssignerLogicalNumberOperator,
  VariableAssignerLogicalNumberOperatorLabelMap,
  VariableAssignerLogicalOperator,
} from '../../constant';

export function useBuildLogicalOptions() {
  const { t } = useTranslation();

  const buildVariableAssignerLogicalOptions = useCallback(
    (record: Record<string, any>) => {
      return buildOptions(
        record,
        t,
        'flow.variableAssignerLogicalOperatorOptions',
        true,
      );
    },
    [t],
  );

  const buildLogicalOptions = useCallback(
    (type: string) => {
      if (
        type?.toLowerCase().startsWith(JsonSchemaDataType.Array.toLowerCase())
      ) {
        return buildVariableAssignerLogicalOptions(
          VariableAssignerLogicalArrayOperator,
        );
      }

      if (type === JsonSchemaDataType.Number) {
        return Object.values(VariableAssignerLogicalNumberOperator).map(
          (val) => ({
            label: t(
              `flow.variableAssignerLogicalOperatorOptions.${camelCase(VariableAssignerLogicalNumberOperatorLabelMap[val as keyof typeof VariableAssignerLogicalNumberOperatorLabelMap] || val)}`,
            ),
            value: val,
          }),
        );
      }

      return buildVariableAssignerLogicalOptions(
        VariableAssignerLogicalOperator,
      );
    },
    [buildVariableAssignerLogicalOptions, t],
  );

  return {
    buildLogicalOptions,
  };
}
