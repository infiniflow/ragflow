// Modified by FXMacroData to add the FXMacroData integration.
import { Operator } from '@/constants/agent';

export function getToolOperatorName(toolName?: string | null) {
  if (!toolName) {
    return '';
  }

  if (toolName.startsWith('fxmacrodata_') || toolName === Operator.FXMacroData) {
    return Operator.FXMacroData;
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

  return toolName
    .split('_')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
    .join('');
}
