import { HomeCard } from '@/components/home-card';
import { MoreButton } from '@/components/more-button';
import { SharedBadge } from '@/components/shared-badge';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { AgentCategory } from '@/constants/agent';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { AgentListItemType, IFlow } from '@/interfaces/database/agent';
import { CanvasCategoryToFlowType, FlowType, FlowTypeConfig } from './constant';
import { AgentDropdown } from './agent-dropdown';
import { useRenameAgent } from './use-rename-agent';
import { useRef, useState } from 'react';
import { Tag } from 'lucide-react';

export type DatasetCardProps = {
  data: IFlow & { type?: AgentListItemType };
} & Pick<ReturnType<typeof useRenameAgent>, 'showAgentRenameModal'>;

function AgentTypeIcon({
  data,
}: {
  data: IFlow & { type?: AgentListItemType };
}) {
  const flowType =
    data.type === AgentListItemType.CompilationTemplateGroup
      ? FlowType.Compiler
      : CanvasCategoryToFlowType[data.canvas_category ?? ''];

  const config = flowType ? FlowTypeConfig[flowType] : null;

  if (!config) {
    return null;
  }

  const Icon = config.icon;

  return (
    <Button variant={'ghost'} size={'sm'}>
      <Icon style={{ color: config.color }} />
    </Button>
  );
}

function AgentTags({ tags }: { tags?: string }) {
  const list = (tags || '')
    .split(',')
    .map((t) => t.trim())
    .filter(Boolean);
  const containerRef = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);

  if (list.length === 0) return null;

  const handleOpenChange = (isOpen: boolean) => {
    if (isOpen) {
      const el = containerRef.current;
      setOpen(el ? el.scrollHeight > el.clientHeight : false);
    } else {
      setOpen(false);
    }
  };

  return (
    <Tooltip open={open} onOpenChange={handleOpenChange}>
      <TooltipTrigger asChild>
        <div ref={containerRef} className="line-clamp-2 leading-6 mt-1">
          {list.map((tag) => (
            <Badge
              key={tag}
              variant="secondary"
              className="text-xs font-normal mr-1 space-x-1"
            >
              <Tag className="size-3" />
              <span>{tag}</span>
            </Badge>
          ))}
        </div>
      </TooltipTrigger>
      <TooltipContent>
        <div className="flex flex-wrap gap-1 max-w-[280px]">
          {list.map((tag) => (
            <Badge
              key={tag}
              variant="secondary"
              className="text-xs font-normal space-x-1"
            >
              <Tag className="size-3" />
              <span>{tag}</span>
            </Badge>
          ))}
        </div>
      </TooltipContent>
    </Tooltip>
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
