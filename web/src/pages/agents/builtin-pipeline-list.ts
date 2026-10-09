import { AgentCategory } from '@/constants/agent';
import {
  AgentListItemType,
  IBuiltinPipeline,
  IBuiltinPipelineListItem,
} from '@/interfaces/database/agent';

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
  const kw = keywords.toLowerCase();
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
