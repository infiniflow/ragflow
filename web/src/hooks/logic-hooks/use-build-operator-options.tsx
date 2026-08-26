/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
        className={cn('size-4')}
        style={
          value === ComparisonOperator.GreatThan
            ? { transform: 'rotate(180deg)' }
            : undefined
        }
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
    const filteredOptions =
      subset.length > 0
        ? SwitchOperatorOptions.filter((x) => subset.some((y) => y === x.value))
        : SwitchOperatorOptions;

    return filteredOptions.map((x) => ({
      value: x.value,
      icon: (
        <LogicalOperatorIcon
          icon={x.icon}
          value={x.value}
        ></LogicalOperatorIcon>
      ),
      label: t(`flow.switchOperatorOptions.${x.label}`),
    }));
  }, [subset, t]);

  return switchOperatorOptions;
}
