import { IconFont } from '@/components/icon-font';
import { ComparisonOperator, SwitchOperatorOptions } from '@/constants/agent';
import { cn } from '@/lib/utils';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

export const LogicalOperatorIcon = function OperatorIcon({
  icon,
  value,
}: Omit<(typeof SwitchOperatorOptions)[0], 'label'>) {
  if (typeof icon === 'string') {
    return (
      <IconFont
        name={icon}
        className={cn('size-4', {
          'rotate-180': value === '>',
        })}
      ></IconFont>
    );
  }
  return icon;
};

export function useBuildSwitchOperatorOptions(
  subset: ComparisonOperator[] = [],
) {
  const { t } = useTranslation();

  const switchOperatorOptions = useMemo(() => {
    return SwitchOperatorOptions.filter((x) =>
      subset.some((y) => y === x.value),
    ).map((x) => ({
      value: x.value,
      icon: (
        <LogicalOperatorIcon
          icon={x.icon}
          value={x.value}
        ></LogicalOperatorIcon>
      ),
      label: t(`flow.switchOperatorOptions.${x.label}`),
    }));
  }, [t]);

  return switchOperatorOptions;
}
