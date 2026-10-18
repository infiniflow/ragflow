import {
  FilterCollection,
  FilterValue,
} from '@/components/list-filter-bar/interface';
import { pruneFilterValue } from '@/components/list-filter-bar/utils';
import storage from '@/utils/authorization-util';
import { ChangeEventHandler, useCallback, useEffect, useState } from 'react';
import { z } from 'zod';
import { useGetPaginationWithRouter } from './logic-hooks';

const DocumentListStateSchema = z.object({
  searchString: z.string(),
  filterValue: z.record(
    z.union([z.array(z.string()), z.record(z.array(z.string()))]),
  ),
});
type DocumentListState = z.infer<typeof DocumentListStateSchema>;
const EmptyState: DocumentListState = { searchString: '', filterValue: {} };

function readState(key: string): DocumentListState {
  try {
    return DocumentListStateSchema.parse(
      JSON.parse(sessionStorage.getItem(key) ?? 'null'),
    );
  } catch {
    return EmptyState;
  }
}

export function useDocumentListState(datasetId: string) {
  const user = storage.getUserInfoObject()?.email ?? '';
  const key = `RAGFlowDocumentList:${user}:${datasetId}`;
  const [saved, setSaved] = useState(() => {
    const value = readState(key);
    return { key, value, initialSearch: { value: value.searchString } };
  });
  // Reset during render so the next dataset's first request uses its own state.
  if (saved.key !== key) {
    const value = readState(key);
    setSaved({ key, value, initialSearch: { value: value.searchString } });
  }
  const state = saved.value;
  useEffect(() => {
    try {
      sessionStorage.setItem(saved.key, JSON.stringify(saved.value));
    } catch {
      // Storage can be disabled or full; the list still works in memory.
    }
  }, [saved]);
  const { setPagination } = useGetPaginationWithRouter();

  const handleInputChange: ChangeEventHandler<HTMLInputElement> = useCallback(
    (event) => {
      const searchString = event.target.value;
      setSaved((previous) => ({
        ...previous,
        value: { ...previous.value, searchString },
      }));
      setPagination({ page: 1 });
    },
    [setPagination],
  );
  const handleFilterSubmit = useCallback(
    (filterValue: FilterValue) => {
      setSaved((previous) => ({
        ...previous,
        value: { ...previous.value, filterValue },
      }));
      setPagination({ page: 1 });
    },
    [setPagination],
  );
  const checkValue = useCallback((filters: FilterCollection[]) => {
    setSaved((previous) => {
      const filterValue = pruneFilterValue(previous.value.filterValue, filters);
      return filterValue === previous.value.filterValue
        ? previous
        : { ...previous, value: { ...previous.value, filterValue } };
    });
  }, []);

  return {
    ...state,
    initialSearch: saved.initialSearch,
    handleInputChange,
    handleFilterSubmit,
    checkValue,
  };
}
