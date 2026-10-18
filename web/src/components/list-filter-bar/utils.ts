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

import { isArray, isEmpty, isEqual, isObject, some } from 'lodash';

import { FilterCollection, FilterType, FilterValue } from './interface';

// A filter value may hold empty structures (e.g. every field set to an empty
// array) once the user clears the popover; only values with at least one
// selected id count as an active filter.
export function hasActiveFilter(value?: FilterValue): boolean {
  return some(value, (fieldValue) =>
    isArray(fieldValue)
      ? !isEmpty(fieldValue)
      : isObject(fieldValue) && some(fieldValue, (ids) => !isEmpty(ids)),
  );
}

const getFilterIds = (filter: FilterType | FilterCollection): string[] => {
  if (Array.isArray(filter.list)) {
    return filter.list.flatMap(getFilterIds);
  }
  return 'id' in filter ? [filter.id] : [];
};

export function pruneFilterValue(
  filterValue: FilterValue,
  filters: FilterCollection[],
): FilterValue {
  const ids = filters.flatMap(getFilterIds);
  if (!ids.length) return filterValue;

  const value: FilterValue = {};
  for (const key in filterValue) {
    const fieldValue = filterValue[key];
    if (Array.isArray(fieldValue)) {
      value[key] = ids.filter((id) => fieldValue.includes(id));
    } else {
      const nestedValue: Record<string, string[]> = {};
      for (const nestedKey in fieldValue) {
        nestedValue[nestedKey] = ids.filter((id) =>
          fieldValue[nestedKey].includes(id),
        );
      }
      value[key] = nestedValue;
    }
  }
  return isEqual(filterValue, value) ? filterValue : value;
}
