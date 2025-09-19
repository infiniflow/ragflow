import kbService from '@/services/knowledge-service';
import { useQuery } from '@tanstack/react-query';
import { useParams, useSearchParams } from 'umi';

export interface IOverviewTital {
  cancelled: number;
  failed: number;
  finished: number;
  processing: number;
}
const useFetchOverviewTital = () => {
  const [searchParams] = useSearchParams();
  const { id } = useParams();
  const knowledgeBaseId = searchParams.get('id') || id;
  const { data } = useQuery<IOverviewTital>({
    queryKey: ['overviewTital'],
    queryFn: async () => {
      const { data: res = {} } = await kbService.getKnowledgeBasicInfo({
        kb_id: knowledgeBaseId,
      });
      return res.data || [];
    },
  });
  return { data };
};

export { useFetchOverviewTital };
