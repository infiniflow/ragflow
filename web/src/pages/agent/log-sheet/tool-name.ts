import { Operator } from '@/constants/agent';

export function getToolOperatorName(toolName?: string | null) {
  if (!toolName) {
    return '';
  }

  const normalizedName = toolName.replaceAll('_', '').toLowerCase();
  if (normalizedName === Operator.QueritSearch.toLowerCase()) {
    return Operator.QueritSearch;
  }

  return toolName
    .split('_')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
    .join('');
}
