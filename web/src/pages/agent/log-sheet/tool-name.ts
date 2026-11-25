import { Operator } from '@/constants/agent';

export function getToolOperatorName(toolName?: string | null) {
  if (!toolName) {
    return '';
  }

  const normalizedName = toolName.replaceAll('_', '').toLowerCase();
  if (normalizedName === Operator.QueritSearch.toLowerCase()) {
    return Operator.QueritSearch;
  }
  if (normalizedName === Operator.QueritContents.toLowerCase()) {
    return Operator.QueritContents;
  }
  // Search1API keeps its brand casing, which the generic split below loses.
  if (normalizedName === Operator.Search1APISearch.toLowerCase()) {
    return Operator.Search1APISearch;
  }
  if (normalizedName === Operator.Search1APICrawl.toLowerCase()) {
    return Operator.Search1APICrawl;
  }
  // Same for AnySearch, whose brand is one word.
  if (normalizedName === Operator.AnySearchSearch.toLowerCase()) {
    return Operator.AnySearchSearch;
  }
  if (normalizedName === Operator.AnySearchDiscover.toLowerCase()) {
    return Operator.AnySearchDiscover;
  }
  if (normalizedName === Operator.AnySearchExtract.toLowerCase()) {
    return Operator.AnySearchExtract;
  }

  return toolName
    .split('_')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
    .join('');
}
