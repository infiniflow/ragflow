import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchAgentListByPage } from '@/hooks/use-agent-request';
import { ApplicationCard } from './application-card';

export function Agents() {
  const { data } = useFetchAgentListByPage();
  const { navigateToAgent } = useNavigatePage();

  return data
    .slice(0, 10)
    .map((x) => (
      <ApplicationCard
        key={x.id}
        app={x}
        onClick={navigateToAgent(x.id)}
      ></ApplicationCard>
    ));
}
