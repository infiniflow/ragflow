import { KnowledgeApiAction } from '@/hooks/use-knowledge-request';
import { checkEmbedding } from '@/services/knowledge-service';
import { useMutation } from '@tanstack/react-query';
import { useParams, useSearchParams } from 'react-router';

export const useCheckKbEmbedding = () => {
  const { id } = useParams();
  const [searchParams] = useSearchParams();
  const knowledgeBaseId = searchParams.get('id') || id;

  const { mutateAsync, isPending } = useMutation({
    mutationKey: [KnowledgeApiAction.CheckKbEmbedding],
    mutationFn: async (embedId: string) => {
      const { data } = await checkEmbedding(knowledgeBaseId || '', {
        embd_id: embedId,
      });
      return data;
    },
  });

  return { checkKbEmbedding: mutateAsync, checking: isPending };
};
