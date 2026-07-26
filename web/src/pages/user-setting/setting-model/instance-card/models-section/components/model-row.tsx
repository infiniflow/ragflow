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

import { Minus, Plus } from 'lucide-react';
import { ModelRowProps } from '../interface';
import { ModelTypeBadges } from './model-type-badges';
import { ModelVerifyButton } from './model-verify-button';

/** Single model row in the catalog list. */
export function ModelRow({
  model,
  isAdded,
  verifyStatus,
  hideActions,
  onVerify,
  onAdd,
  onRemove,
  onEdit,
  editLabel,
}: ModelRowProps) {
  return (
    <li
      key={model.name}
      className="group flex items-center justify-between gap-3 p-3 border-b border-border-button last:border-b-0 hover:bg-bg-input transition-colors"
      data-testid={`models-row-${model.name}`}
    >
      <div className="flex gap-1 min-w-0">
        <div className="flex items-center gap-2 min-w-0">
          <span className="font-medium text-sm text-text-primary truncate">
            {model.name}
          </span>
        </div>
        <div className="flex flex-wrap items-center gap-1">
          <ModelTypeBadges
            types={model.model_types ?? []}
            showEdit={!hideActions}
            onEdit={onEdit}
            editLabel={editLabel}
            editTestSuffix={model.name}
          />
        </div>
      </div>

      <div className="flex items-center gap-2 shrink-0">
        <ModelVerifyButton
          status={verifyStatus}
          onVerify={onVerify}
          modelName={model.name}
        />

        {!hideActions && (
          <button
            type="button"
            className="size-6 flex items-center justify-center rounded-md transition-colors text-text-secondary"
            onClick={() => (isAdded ? onRemove() : onAdd())}
            aria-label={isAdded ? `Remove ${model.name}` : `Add ${model.name}`}
          >
            {isAdded ? (
              <Minus className="size-4" />
            ) : (
              <Plus className="size-4" />
            )}
          </button>
        )}
      </div>
    </li>
  );
}
