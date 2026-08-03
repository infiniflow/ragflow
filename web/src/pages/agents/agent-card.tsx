import { HomeCard } from '@/components/home-card';
import { MoreButton } from '@/components/more-button';
import { SharedBadge } from '@/components/shared-badge';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '@/components/ui/hover-card';
import { AgentCategory } from '@/constants/agent';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { AgentListItemType, IFlow } from '@/interfaces/database/agent';
import { useLayoutEffect, useRef, useState } from 'react';
import { CanvasCategoryToFlowType, FlowType, FlowTypeConfig } from './constant';
import { AgentDropdown } from './agent-dropdown';
import { useRenameAgent } from './use-rename-agent';

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

  const icon = flowType ? FlowTypeConfig[flowType].icon : null;

  if (!icon) {
    return null;
  }

  const Icon = icon;

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

  const containerRef = useRef<HTMLDivElement>(null);
  const measureRef = useRef<HTMLDivElement>(null);
  const [visibleCount, setVisibleCount] = useState(list.length);

  useLayoutEffect(() => {
    const container = containerRef.current;
    const measure = measureRef.current;
    if (!container || !measure) return;

    const update = () => {
      const children = Array.from(measure.children) as HTMLElement[];
      const ellipsis = children.pop();
      if (!ellipsis) return;

      const gap = 4;
      const maxWidth = container.clientWidth;
      const widthOf = (els: HTMLElement[]) =>
        els.reduce((acc, el) => acc + el.offsetWidth, 0) +
        gap * Math.max(els.length - 1, 0);

      // Keep only the tags that fit in a single row.
      let count = children.length;
      while (count > 0 && widthOf(children.slice(0, count)) > maxWidth) {
        count -= 1;
      }
      // Reserve space for the ellipsis trigger when truncated.
      if (count < children.length) {
        while (
          count > 0 &&
          widthOf(children.slice(0, count)) + gap + ellipsis.offsetWidth >
            maxWidth
        ) {
          count -= 1;
        }
      }
      setVisibleCount(count);
    };

    update();
    const observer = new ResizeObserver(update);
    observer.observe(container);
    return () => observer.disconnect();
  }, [tags]);

  if (list.length === 0) return null;

  const visible = list.slice(0, visibleCount);
  const truncated = visibleCount < list.length;

  return (
    <div ref={containerRef} className="relative mt-1">
      {/* Hidden row used to measure the natural width of each tag */}
      <div
        ref={measureRef}
        aria-hidden
        className="pointer-events-none invisible absolute flex gap-1 whitespace-nowrap"
      >
        {list.map((tag) => (
          <Badge key={tag} variant="secondary" className="text-xs font-normal">
            {tag}
          </Badge>
        ))}
        <Badge variant="secondary" className="text-xs font-normal">
          ...
        </Badge>
      </div>
      <div className="flex gap-1 overflow-hidden">
        {visible.map((tag) => (
          <Badge key={tag} variant="secondary" className="text-xs font-normal">
            {tag}
          </Badge>
        ))}
        {truncated && (
          <HoverCard openDelay={100} closeDelay={100}>
            <HoverCardTrigger asChild>
              <Badge
                variant="secondary"
                className="text-xs font-normal cursor-pointer"
              >
                ...
              </Badge>
            </HoverCardTrigger>
            <HoverCardContent>
              <div className="flex flex-wrap gap-1">
                {list.map((tag) => (
                  <Badge
                    key={tag}
                    variant="secondary"
                    className="text-xs font-normal"
                  >
                    {tag}
                  </Badge>
                ))}
              </div>
            </HoverCardContent>
          </HoverCard>
        )}
      </div>
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
