import { IApplyDatasetParserConfigResult } from '@/interfaces/database/document';
import { ResponseType } from '@/interfaces/database/base';
import { applyDatasetParserConfig } from '@/services/knowledge-service';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { DocumentKeys } from './document-query-keys';

export function useApplyDatasetParserConfig(datasetId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      const { data }: { data: ResponseType<IApplyDatasetParserConfigResult> } =
        await applyDatasetParserConfig(datasetId);
      if (data.code !== 0) {
        throw new Error(data.message);
      }
      return data.data;
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: DocumentKeys.all() }),
        queryClient.invalidateQueries({ queryKey: DocumentKeys.allFilters() }),
      ]);
    },
  });
}
