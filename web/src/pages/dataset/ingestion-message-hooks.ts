import {
  IngestionMessagesResponse,
  IngestionMessageParams,
} from '@/interfaces/database/ingestion';
import { listIngestionMessages } from '@/services/knowledge-service';
import { useIsGoBackend } from '@/utils/backend-variant';
import { useQuery } from '@tanstack/react-query';

const PollIntervalMs = 5000;

export const IngestionMessageKeys = {
  messages: (datasetId: string | undefined, logId: string | undefined) =>
    ['ingestionMessages', datasetId, logId] as const,
};

export const useIngestionMessages = (
  datasetId: string | undefined,
  logId: string | undefined,
  enabled: boolean,
  params: IngestionMessageParams = { limit: 200 },
) => {
  const isGoBackend = useIsGoBackend();

  return useQuery<IngestionMessagesResponse>({
    queryKey: IngestionMessageKeys.messages(datasetId, logId),
    enabled: enabled && isGoBackend && !!datasetId && !!logId,
    refetchInterval: (query) =>
      query.state.data?.terminal ? false : PollIntervalMs,
    queryFn: async () => {
      const { data: res = {} } = await listIngestionMessages(
        datasetId || '',
        logId || '',
        params,
      );
      return res.data as IngestionMessagesResponse;
    },
  });
};
