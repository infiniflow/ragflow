import { AgentCategory, AgentQuery } from '@/constants/agent';
import { useSearchParams } from 'umi';

export function useIsPipeline() {
  const [queryParameters] = useSearchParams();

  return (
    queryParameters.get(AgentQuery.Category) === AgentCategory.DataflowCanvas
  );
}
