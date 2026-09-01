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

import { RowSelectionState } from '@tanstack/react-table';
import { isEmpty } from 'lodash';
import { useCallback, useMemo, useState } from 'react';

export function useRowSelection() {
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});

  const clearRowSelection = useCallback(() => {
    setRowSelection({});
  }, []);

  const selectedCount = useMemo(() => {
    return Object.keys(rowSelection).length;
  }, [rowSelection]);

  return {
    rowSelection,
    setRowSelection,
    rowSelectionIsEmpty: isEmpty(rowSelection),
    clearRowSelection,
    selectedCount,
  };
}

export type UseRowSelectionType = ReturnType<typeof useRowSelection>;

export function useSelectedIds<T extends Array<{ id: string }>>(
  rowSelection: RowSelectionState,
  list: T,
) {
  const selectedIds = useMemo(() => {
    // When using getRowId, rowSelection keys are IDs, not indices
    const selectionKeys = Object.keys(rowSelection);

    // Check if keys are numeric (index-based) or string IDs
    const isIndexBased = selectionKeys.every((key) => !isNaN(Number(key)));

    if (isIndexBased) {
      // Legacy index-based selection
      return list
        .filter((x, idx) => selectionKeys.some((y) => Number(y) === idx))
        .map((x) => x.id);
    } else {
      // ID-based selection (when getRowId is used)
      // return selectionKeys.filter((id) => list.some((item) => item.id === id));
      return selectionKeys;
    }
  }, [list, rowSelection]);

  return { selectedIds };
}
