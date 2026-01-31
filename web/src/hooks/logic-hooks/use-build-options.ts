import { SwitchLogicOperator } from '@/constants/agent';
import { buildOptions } from '@/utils/form';
import { useTranslation } from 'react-i18next';

export function useBuildSwitchLogicOperatorOptions() {
  const { t } = useTranslation();
  return buildOptions(
    SwitchLogicOperator,
    t,
    'flow.switchLogicOperatorOptions',
  );
}
