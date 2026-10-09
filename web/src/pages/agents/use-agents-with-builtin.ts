import { useMemo } from 'react';
import {
  useFetchAgentListByPage,
  useFetchBuiltinPipelines,
} from '@/hooks/use-agent-request';
import {
  filterBuiltinByKeyword,
  shouldShowBuiltin,
  toBuiltinListItem,
} from './builtin-pipeline-list';

// useAgentsWithBuiltin composes the user canvas list (agents + user pipelines)
// with the built-in ingestion pipelines into a single render model, mirroring
// how PR #20603 already composes built-in pipelines into the parser dropdown.
// Built-in pipelines are static, non-DB resources served by GET /api/v1/pipelines;
// they are shown read-only after the user items, only for the "All" and
// "Pipeline" category views, and filtered by the same keyword as the user list.
export function useAgentsWithBuiltin() {
  const agentList = useFetchAgentListByPage();
  const { data: builtinData, loading: builtinLoading } =
    useFetchBuiltinPipelines();

  const rawCategory = agentList.filterValue?.canvasCategory;
  const canvasCategoryIds = useMemo<undefined | string[]>(() => {
    if (!rawCategory) return undefined;
    const list = Array.isArray(rawCategory) ? rawCategory : [rawCategory];
    return list.filter((x): x is string => typeof x === 'string');
  }, [rawCategory]);

  const builtinItems = useMemo(() => {
    // A structured (non-string) category filter carries no plain ids, so the
    // "All"/"Pipeline" intent cannot be assumed; hide built-ins rather than
    // guessing. shouldShowBuiltin treats an explicit empty array as "All",
    // which would wrongly surface built-ins here.
    const hasStructuredFilter =
      Array.isArray(rawCategory) &&
      rawCategory.length > 0 &&
      canvasCategoryIds.length === 0;
    if (hasStructuredFilter || !shouldShowBuiltin(canvasCategoryIds)) {
      return [];
    }
    return filterBuiltinByKeyword(
      builtinData?.canvas ?? [],
      agentList.debouncedSearchString,
    ).map(toBuiltinListItem);
  }, [rawCategory, canvasCategoryIds, builtinData, agentList.debouncedSearchString]);

  return {
    ...agentList,
    builtinItems,
    builtinLoading,
  };
}
