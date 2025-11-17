import { buildOptions } from '@/utils/form';
import { useCallback } from 'react';
import {
  JsonSchemaDataType,
  VariableAssignerLogicalArrayOperator,
  VariableAssignerLogicalNumberOperator,
  VariableAssignerLogicalOperator,
} from '../../constant';

export function useBuildLogicalOptions() {
  const buildLogicalOptions = useCallback((type: string) => {
    if (
      type?.toLowerCase().startsWith(JsonSchemaDataType.Array.toLowerCase())
    ) {
      return buildOptions(VariableAssignerLogicalArrayOperator);
    }

    if (type === JsonSchemaDataType.Number) {
      return buildOptions(VariableAssignerLogicalNumberOperator);
    }

    return buildOptions(VariableAssignerLogicalOperator);
  }, []);

  return {
    buildLogicalOptions,
  };
}
