import { IAddedModel } from '@/interfaces/database/llm';
import { useCallback, useMemo } from 'react';

type AdditionalModelValidIdsOptions = {
  modelTypes: string[];
  models: IAddedModel[];
  modelsFetched: boolean;
  enabled: boolean;
  ownerTenantId?: string;
};

export function useAdditionalModelValidIds({
  modelsFetched,
}: AdditionalModelValidIdsOptions) {
  const validIds = useMemo(() => new Set<string>(), []);
  const getPendingMessage = useCallback((value?: string) => {
    void value;
    return undefined;
  }, []);
  const resolveValidIds = useCallback(async (freshModels: IAddedModel[]) => {
    void freshModels;
    return new Set<string>();
  }, []);

  return {
    validIds,
    isSettled: modelsFetched,
    getPendingMessage,
    resolveValidIds,
  };
}
