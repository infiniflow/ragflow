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

import { useBuildSwitchLogicOperatorOptions } from '@/hooks/logic-hooks/use-build-options';
import { RAGFlowFormItem } from './ragflow-form';
import { RAGFlowSelect } from './ui/select';

type LogicalOperatorProps = { name: string };

export function LogicalOperator({ name }: LogicalOperatorProps) {
  const switchLogicOperatorOptions = useBuildSwitchLogicOperatorOptions();

  return (
    <div className="relative min-w-14">
      <RAGFlowFormItem
        name={name}
        className="absolute top-1/2 -translate-y-1/2 right-1 left-0 z-10 bg-bg-base"
      >
        <RAGFlowSelect
          options={switchLogicOperatorOptions}
          triggerClassName="w-full text-xs px-1 py-0 h-6"
        ></RAGFlowSelect>
      </RAGFlowFormItem>
      <div className="absolute border-l border-y w-5 right-0 top-4 bottom-4 rounded-l-lg"></div>
    </div>
  );
}
