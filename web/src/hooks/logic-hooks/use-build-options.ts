import { SwitchLogicOperator } from '@/constants/agent';
import { buildOptions } from '@/utils/form';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

export function useBuildSwitchLogicOperatorOptions() {
  const { t } = useTranslation();
  return buildOptions(
    SwitchLogicOperator,
    t,
    'flow.switchLogicOperatorOptions',
  );
}

export function useBuildModelTypeOptions() {
  const { t } = useTranslation();

  const buildModelTypeOptions = useCallback(
    (list: string[]) => {
      return list.map((x) => ({
        value: x,
        label: t(`setting.modelTypes.${x}`),
      }));
    },
    [t],
  );

  return {
    buildModelTypeOptions,
  };
}
