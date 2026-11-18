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

import { AgentQuery } from '@/constants/agent';
import { useCallback } from 'react';
import { useSearchParams } from 'react-router';
import {
  AgentTabCategory,
  CanvasCategoryToFlowType,
  FlowType,
  FlowTypeConfig,
} from '../constant';

const DefaultTabCategory = FlowTypeConfig[FlowType.Agent].canvasCategory;

function resolveTabCategory(value: string | null): AgentTabCategory {
  return value && CanvasCategoryToFlowType[value] !== undefined
    ? (value as AgentTabCategory)
    : DefaultTabCategory;
}

export function useAgentTab() {
  const [searchUrl, setSearchUrl] = useSearchParams();

  const activeCategory = resolveTabCategory(
    searchUrl.get(AgentQuery.CanvasCategory),
  );

  const handleTabChange = useCallback(
    (value: string | number) => {
      const nextCategory = resolveTabCategory(String(value));
      if (nextCategory === activeCategory) {
        return;
      }
      const nextParams = new URLSearchParams(searchUrl);
      nextParams.set(AgentQuery.CanvasCategory, nextCategory);
      nextParams.set('page', '1');
      setSearchUrl(nextParams, { replace: true });
    },
    [activeCategory, searchUrl, setSearchUrl],
  );

  return {
    searchUrl,
    setSearchUrl,
    activeCategory,
    activeType: CanvasCategoryToFlowType[activeCategory],
    handleTabChange,
  };
}
