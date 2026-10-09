import { AgentCategory } from '@/constants/agent';
import {
  AgentListItemType,
  IBuiltinPipeline,
  IBuiltinPipelineListItem,
} from '@/interfaces/database/agent';

// Whether the built-in pipeline section should be shown for the given
// canvas_category filter (the agents list's category multi-select).
// Built-in pipelines are parsing methods => dataflow canvases, so they appear
// in the "All" view and the "Pipeline" view, but not the pure "Agent" or
// "Compilation template group" views. This mirrors how PR #20603 surfaces
// built-in pipelines only alongside user pipelines.
export function shouldShowBuiltin(canvasCategoryIds?: string[]): boolean {
  if (!canvasCategoryIds || canvasCategoryIds.length === 0) {
    return true; // "All" view
  }
  return canvasCategoryIds.includes(AgentCategory.DataflowCanvas);
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
