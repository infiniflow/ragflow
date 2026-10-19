import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { initialScoredSwitchValues } from '../../constant';
import { useMemo } from 'react';
import { v4 as uuid } from 'uuid';

export function useValues(node?: RAGFlowNodeType) {
  return useMemo(() => {
    const formData = (node?.data?.form ?? {}) as Record<string, any>;
    const rawItems = Array.isArray(formData.items) ? formData.items : [];
    const options = Array.isArray(formData.options) ? formData.options : [];
    const items =
      rawItems.length > 0
        ? rawItems
        : options.map((option: Record<string, any>) => ({
            name: option?.name ?? '',
            description: option?.description ?? '',
            uuid: option?.uuid || uuid(),
          }));
    return {
      ...initialScoredSwitchValues,
      ...formData,
      items,
      escalate_enabled: formData.escalate_enabled !== false,
    };
  }, [node]);
}
