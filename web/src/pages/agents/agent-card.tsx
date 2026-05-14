import { HomeCard } from '@/components/home-card';
import { MoreButton } from '@/components/more-button';
import { SharedBadge } from '@/components/shared-badge';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { AgentCategory } from '@/constants/agent';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { IFlow } from '@/interfaces/database/agent';
import { Route } from 'lucide-react';
import { AgentDropdown } from './agent-dropdown';
import { useRenameAgent } from './use-rename-agent';

export type DatasetCardProps = {
  data: IFlow;
} & Pick<ReturnType<typeof useRenameAgent>, 'showAgentRenameModal'>;

function AgentTags({ tags }: { tags?: string }) {
  const list = (tags || '')
    .split(',')
    .map((t) => t.trim())
    .filter(Boolean);
  if (list.length === 0) return null;
  return (
    <div className="flex flex-wrap gap-1 mt-1">
      {list.map((tag) => (
        <Badge key={tag} variant="secondary" className="text-xs font-normal">
          {tag}
        </Badge>
      ))}
    </div>
  );
}

export function AgentCard({ data, showAgentRenameModal }: DatasetCardProps) {
  const { navigateToAgent } = useNavigatePage();

  return (
    <HomeCard
      testId="agent-card"
      data={{
        ...data,
        name: data.title,
        description: data.description || '',
        release_time: data.release_time,
      }}
      moreDropdown={
        <AgentDropdown showAgentRenameModal={showAgentRenameModal} agent={data}>
          <MoreButton></MoreButton>
        </AgentDropdown>
      }
      sharedBadge={<SharedBadge>{data.nickname}</SharedBadge>}
      onClick={
        // data.canvas_category === AgentCategory.DataflowCanvas
        //   ? navigateToDataflow(data.id)
        //   :
        navigateToAgent(data?.id, data.canvas_category as AgentCategory)
      }
      icon={
        data.canvas_category === AgentCategory.DataflowCanvas && (
          <Button variant={'ghost'} size={'sm'}>
            <Route />
          </Button>
        )
      }
      extra={<AgentTags tags={data.tags} />}
      showReleaseTime
    />
  );
}
