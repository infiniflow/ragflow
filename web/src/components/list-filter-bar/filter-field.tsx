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

import { Checkbox } from '@/components/ui/checkbox';
import { ChevronDown, ChevronUp } from 'lucide-react';
import { memo, useState } from 'react';
import { ControllerRenderProps, useFormContext } from 'react-hook-form';
import { FormControl, FormField, FormItem, FormLabel } from '../ui/form';
import { FilterType } from './interface';
const handleCheckChange = ({
  checked,
  field,
  item,
  isNestedField = false,
  parentId = '',
}: {
  checked: boolean;
  field: ControllerRenderProps<
    { [x: string]: { [x: string]: any } | string[] },
    string
  >;
  item: FilterType;
  isNestedField?: boolean;
  parentId?: string;
}) => {
  if (isNestedField && parentId) {
    const currentValue = field.value || {};
    const currentParentValues =
      (currentValue as Record<string, string[]>)[parentId] || [];

    const newParentValues = checked
      ? [...currentParentValues, item.id.toString()]
      : currentParentValues.filter(
          (value: string) => value !== item.id.toString(),
        );

    const newValue = newParentValues?.length
      ? {
          ...currentValue,
          [parentId]: newParentValues,
        }
      : { ...currentValue };

    // if (newValue[parentId].length === 0) {
    //   delete newValue[parentId];
    // }

    return field.onChange(newValue);
  } else {
    const list = checked
      ? [...(Array.isArray(field.value) ? field.value : []), item.id.toString()]
      : (Array.isArray(field.value) ? field.value : []).filter(
          (value) => value !== item.id.toString(),
        );
    return field.onChange(list);
  }
};

// True when any leaf option inside `node` is part of the current selection.
// Used to auto-expand a group so an already-applied filter stays visible when
// the popover is reopened, instead of being hidden behind a collapsed header.
const subtreeHasSelected = (
  node: FilterType,
  selectedIds: Set<string>,
): boolean => {
  if (node.list && node.list.length > 0) {
    return node.list.some((child) => subtreeHasSelected(child, selectedIds));
  }
  return selectedIds.has(node.id.toString());
};

const FilterItem = memo(
  ({
    item,
    field,
    level = 0,
  }: {
    item: FilterType;
    field: ControllerRenderProps<
      { [x: string]: { [x: string]: any } | string[] },
      string
    >;
    level: number;
  }) => {
    return (
      <div
        className={`flex items-center justify-between text-text-primary text-xs ${level > 0 ? 'ml-1' : ''}`}
      >
        <FormItem className="flex flex-row space-x-3 space-y-0 items-center min-w-0 flex-1">
          <FormControl>
            <div className="flex items-center space-x-2 min-w-0">
              <Checkbox
                checked={Boolean(field.value?.includes?.(item.id.toString()))}
                onCheckedChange={(checked: boolean) =>
                  handleCheckChange({ checked, field, item })
                }
                // className="hidden group-hover:block"
              />
              <div
                onClick={() =>
                  handleCheckChange({
                    checked: !field.value?.includes(item.id.toString()),
                    field,
                    item,
                  })
                }
                className="truncate text-sm font-normal leading-normal peer-disabled:cursor-not-allowed peer-disabled:opacity-70 text-text-secondary"
              >
                {item.label}
              </div>
            </div>
          </FormControl>
        </FormItem>
        {item.count !== undefined && (
          <span className="text-sm shrink-0 ml-2 text-right">{item.count}</span>
        )}
      </div>
    );
  },
);
FilterItem.displayName = 'FilterItem';
export const FilterField = memo(
  ({
    item,
    parent,
    level = 0,
    autoExpand = false,
    selectedIds,
  }: {
    item: FilterType;
    parent: FilterType;
    level?: number;
    autoExpand?: boolean;
    selectedIds: Set<string>;
  }) => {
    const form = useFormContext();
    const hasNestedList = item.list && item.list.length > 0;

    // Start expanded when this group already holds a selected option (an
    // applied filter), so the selection is visible on open. `selectedIds` comes
    // from the applied `value` prop and is stable while the popover is open, so
    // this only needs to seed the initial state. Search-driven expansion still
    // reacts to `autoExpand` changes below.
    const [showAll, setShowAll] = useState(
      autoExpand || subtreeHasSelected(item, selectedIds),
    );
    const [prevAutoExpand, setPrevAutoExpand] = useState(autoExpand);

    if (autoExpand !== prevAutoExpand) {
      setPrevAutoExpand(autoExpand);
      if (autoExpand) {
        setShowAll(true);
      }
    }

    return (
      <FormField
        key={item.id}
        control={form.control}
        name={parent.field?.toString() as string}
        render={({ field }) => {
          if (hasNestedList) {
            return (
              <div className={`flex flex-col gap-2 ${level > 0 ? 'ml-1' : ''}`}>
                <div
                  className="flex items-center justify-between cursor-pointer"
                  onClick={() => {
                    setShowAll(!showAll);
                  }}
                >
                  <FormLabel className="text-text-secondary text-sm">
                    {item.label}
                  </FormLabel>
                  {showAll ? (
                    <ChevronUp size={12} />
                  ) : (
                    <ChevronDown size={12} />
                  )}
                </div>
                {showAll &&
                  item.list?.map((child) => (
                    <FilterField
                      key={child.id}
                      item={child}
                      parent={{
                        ...item,
                        field: `${parent.field}.${item.field}`,
                      }}
                      level={level + 1}
                      autoExpand={autoExpand}
                      selectedIds={selectedIds}
                    />
                  ))}
              </div>
            );
          }

          return <FilterItem item={item} field={field} level={level} />;
        }}
      />
    );
  },
);

FilterField.displayName = 'FilterField';
