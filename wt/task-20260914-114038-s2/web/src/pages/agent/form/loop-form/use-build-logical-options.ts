import { SwitchOperatorOptions } from '@/constants/agent';
import { camelCase, toLower } from 'lodash';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { LoopTerminationStringComparisonOperatorMap } from '../../constant';

export function useBuildLogicalOptions() {
  const { t } = useTranslation();

  const buildLogicalOptions = useCallback(
    (type: string) => {
      return LoopTerminationStringComparisonOperatorMap[
        toLower(type) as keyof typeof LoopTerminationStringComparisonOperatorMap
      ]?.map((x) => ({
        label: t(
          `flow.switchOperatorOptions.${camelCase(SwitchOperatorOptions.find((y) => y.value === x)?.label || x)}`,
        ),
        value: x,
      }));
    },
    [t],
  );

  return {
    buildLogicalOptions,
  };
}
