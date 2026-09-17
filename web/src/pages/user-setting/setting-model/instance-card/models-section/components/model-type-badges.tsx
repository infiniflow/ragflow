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

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { Pencil } from 'lucide-react';
import { mapModelKey } from '../../available-models';
import { ModelTypeBadgesProps } from '../interface';

/** Max model-type badges shown inline; the rest collapse into a tooltip. */
const MAX_VISIBLE_TYPES = 3;

/** Renders the model-type badges row, collapsing overflow into a tooltip. */
export function ModelTypeBadges({
  types,
  onEdit,
  showEdit,
  editLabel,
  editTestSuffix,
}: ModelTypeBadgesProps) {
  const visible = types.slice(0, MAX_VISIBLE_TYPES);
  const hidden = types.slice(MAX_VISIBLE_TYPES);
  return (
    <>
      {visible.map((mt) => (
        <span
          key={mt}
          className="px-1.5 py-0.5 text-[10px] bg-bg-card text-text-secondary rounded-md"
        >
          {mapModelKey[mt as keyof typeof mapModelKey] || mt}
        </span>
      ))}
      {hidden.length > 0 && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span
              className="px-1.5 py-0.5 text-[10px] bg-bg-card text-text-secondary rounded-md cursor-default"
              data-testid={`models-types-overflow-${editTestSuffix}`}
            >
              +{hidden.length}
            </span>
          </TooltipTrigger>
          <TooltipContent>
            <div className="flex flex-wrap gap-1 max-w-[16rem]">
              {hidden.map((mt) => (
                <span
                  key={mt}
                  className="px-1.5 py-0.5 text-[10px] bg-bg-card text-text-secondary rounded-md"
                >
                  {mapModelKey[mt as keyof typeof mapModelKey] || mt}
                </span>
              ))}
            </div>
          </TooltipContent>
        </Tooltip>
      )}
      {showEdit && (
        <button
          type="button"
          className="ml-1 size-5 flex items-center justify-center rounded-md text-text-secondary opacity-0 transition-all hover:bg-bg-card hover:text-text-primary group-hover:opacity-100 focus-visible:opacity-100"
          onClick={(e) => {
            e.stopPropagation();
            onEdit?.();
          }}
          aria-label={editLabel}
          title={editLabel}
          data-testid={`models-edit-${editTestSuffix}`}
        >
          <Pencil className="size-3" />
        </button>
      )}
    </>
  );
}
