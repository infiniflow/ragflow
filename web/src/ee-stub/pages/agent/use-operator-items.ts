import type { Operator } from '@/constants/agent';

export function useOperatorItems(operators: Operator[]): {
  operators: Operator[];
  initializeNode: (nodeId: string, operator: string) => void;
} {
  return {
    operators,
    initializeNode: () => {},
  };
}
