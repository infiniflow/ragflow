import { HomeCard } from '@/components/home-card';
import { MoreButton } from '@/components/more-button';
import { SharedBadge } from '@/components/shared-badge';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { AgentCategory } from '@/constants/agent';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { AgentListItemType, IFlow } from '@/interfaces/database/agent';
import { LucideIcon, Network, Route, Shapes } from 'lucide-react';
import { AgentDropdown } from './agent-dropdown';
import { useRenameAgent } from './use-rename-agent';

export type DatasetCardProps = {
  data: IFlow & { type?: AgentListItemType };
} & Pick<ReturnType<typeof useRenameAgent>, 'showAgentRenameModal'>;

const CanvasCategoryIconMap: Record<string, LucideIcon> = {
  [AgentCategory.AgentCanvas]: Network,
  [AgentCategory.DataflowCanvas]: Route,
};

function AgentTypeIcon({
  data,
}: {
  data: IFlow & { type?: AgentListItemType };
}) {
  const Icon =
    data.type === AgentListItemType.CompilationTemplateGroup
      ? Shapes
      : CanvasCategoryIconMap[data.canvas_category];

  if (!Icon) {
    return null;
  }

  return (
    <Button variant={'ghost'} size={'sm'}>
      <Icon />
    </Button>
  );
}

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
      icon={<AgentTypeIcon data={data} />}
      extra={<AgentTags tags={data.tags} />}
      showReleaseTime
    />
  );
}
