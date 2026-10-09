import { AgentCategory } from '@/constants/agent';
import { useFetchBuiltinPipelines } from '@/hooks/use-agent-request';
import {
  AgentListItemType,
  IBuiltinPipeline,
  IBuiltinPipelineListItem,
} from '@/interfaces/database/agent';
import { useMemo } from 'react';

// Whether the built-in pipeline section should be shown for the given
// canvas_category filter as stored on the agents list's filter value. Built-in
// pipelines are parsing methods => dataflow canvases, so they appear in the
// "All" view (undefined) and the "Pipeline" view, but not the pure "Agent" or
// "Compilation template group" views. A structured (Record-typed) filter
// carries no plain ids, so the "All"/"Pipeline" intent cannot be assumed and
// built-ins stay hidden rather than guessing. This mirrors how PR #20603
// surfaces built-in pipelines only alongside user pipelines.
export function shouldShowBuiltinForRaw(
  rawCategory?: string | string[] | Record<string, string[]>,
): boolean {
  if (!rawCategory) {
    return true; // "All" view
  }
  if (typeof rawCategory === 'string') {
    return rawCategory === AgentCategory.DataflowCanvas;
  }
  if (Array.isArray(rawCategory)) {
    const ids = rawCategory.filter((x): x is string => typeof x === 'string');
    if (ids.length === 0) {
      return false; // structured filter with no plain ids => hide
    }
    return ids.includes(AgentCategory.DataflowCanvas);
  }
  // A single structured (Record) filter carries no plain ids; the "All"/
  // "Pipeline" intent cannot be assumed, so built-ins stay hidden.
  return false;
}

// Filters built-in pipelines by a case-insensitive keyword over title and
// description. An empty/whitespace/undefined keyword returns the list
// unchanged.
export function filterBuiltinByKeyword(
  items: IBuiltinPipeline[],
  keywords?: string,
): IBuiltinPipeline[] {
  if (!keywords || keywords.trim() === '') {
    return items;
  }
  const kw = keywords.trim().toLowerCase();
  return items.filter((item) => {
    const title = (item.title ?? '').toLowerCase();
    const description = (item.description ?? '').toLowerCase();
    return title.includes(kw) || description.includes(kw);
  });
}

// Normalises a built-in pipeline catalog entry into the list-item shape the
// agents list renders, tagging it as a read-only built-in dataflow canvas.
export function toBuiltinListItem(
  item: IBuiltinPipeline,
): IBuiltinPipelineListItem {
  return {
    ...item,
    canvas_category: AgentCategory.DataflowCanvas,
    type: AgentListItemType.BuiltinPipeline,
    builtin: true,
  };
}

// Shared hook that resolves the filtered, rendered built-in pipeline items for
// the given category view and search keyword. Both the section and the
// agents-list empty-state probe call this with the same arguments; React Query
// dedupes the underlying catalog request by key, so the request fires once.
export function useBuiltinItems(
  rawCategory?: string | string[] | Record<string, string[]>,
  searchString?: string,
): { items: IBuiltinPipelineListItem[]; loading: boolean } {
  const { data: builtinData, loading } = useFetchBuiltinPipelines();
  const items = useMemo(() => {
    if (!shouldShowBuiltinForRaw(rawCategory)) {
      return [];
    }
    return filterBuiltinByKeyword(builtinData?.canvas ?? [], searchString).map(
      toBuiltinListItem,
    );
  }, [rawCategory, builtinData, searchString]);
  return { items, loading };
}

export type AgentsEmptyState = 'content' | 'search-empty' | 'loading' | 'empty';

// Decides what the agents list should render when there are no user-owned
// canvases. The decision must account for *filtered* built-in pipeline matches,
// not merely whether the built-in section is *eligible* for the current
// category (a category-eligibility boolean would wrongly keep the empty
// CardContainer mounted on a zero-result search). `builtinVisible` and
// `builtinLoading` describe the eligible built-in catalog; `listLoading`
// describes the user-owned list.
export function resolveAgentsEmptyState(params: {
  dataLength: number;
  builtinItemsLength: number;
  searchString?: string;
  listLoading: boolean;
  builtinVisible: boolean;
  builtinLoading: boolean;
}): AgentsEmptyState {
  const hasContent = params.dataLength > 0 || params.builtinItemsLength > 0;

  // While the user-owned list is still loading, render nothing.
  if (params.listLoading) {
    return 'loading';
  }

  // Keep the container mounted while the eligible built-in catalog is still
  // loading, so we don't briefly flash an empty card before its items arrive
  // (matches the prior behaviour of showing the bare container during load).
  if (params.builtinVisible && params.builtinLoading && !hasContent) {
    return 'content';
  }

  if (hasContent) {
    return 'content';
  }

  if (params.searchString) {
    return 'search-empty';
  }

  return 'empty';
}
