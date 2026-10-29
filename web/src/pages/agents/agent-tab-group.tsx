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

import { PillToggleGroup } from '@/components/pill-toggle-group';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { AgentTabCategory, AgentTabs, FlowTypeConfig } from './constant';

// The Compilation / Pipeline / Workflow tab group at the top of the agents
// list, driven by the canvas_category URL param.
export function AgentTabGroup({
  activeCategory,
  onTabChange,
}: {
  activeCategory: AgentTabCategory;
  onTabChange: (value: string) => void;
}) {
  const { t } = useTranslation();

  const tabOptions = useMemo(
    () =>
      AgentTabs.map((flowType) => {
        const config = FlowTypeConfig[flowType];
        return {
          value: config.canvasCategory,
          label: t(`flow.${config.labelKey}`),
          icon: config.icon,
        };
      }),
    [t],
  );

  return (
    <PillToggleGroup
      options={tabOptions}
      value={activeCategory}
      onChange={onTabChange}
    />
  );
}
