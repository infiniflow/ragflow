import { MoreButton } from '@/components/more-button';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Card, CardContent } from '@/components/ui/card';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { IFlow } from '@/interfaces/database/flow';
import { formatDate } from '@/utils/date';
import { AgentDropdown } from './agent-dropdown';
import { useRenameAgent } from './use-rename-agent';

export type DatasetCardProps = {
  data: IFlow;
} & Pick<ReturnType<typeof useRenameAgent>, 'showAgentRenameModal'>;

export function AgentCard({ data, showAgentRenameModal }: DatasetCardProps) {
  const { navigateToAgent } = useNavigatePage();

  return (
    <Card key={data.id} className="w-40" onClick={navigateToAgent(data.id)}>
      <CardContent className="p-2.5 pt-2 group">
        <section className="flex justify-between mb-2">
          <div className="flex gap-2 items-center">
            <Avatar className="size-6 rounded-lg">
              <AvatarImage src={data.avatar} />
              <AvatarFallback className="rounded-lg ">CN</AvatarFallback>
            </Avatar>
          </div>
          <AgentDropdown
            showAgentRenameModal={showAgentRenameModal}
            agent={data}
          >
            <MoreButton></MoreButton>
          </AgentDropdown>
        </section>
        <div className="flex justify-between items-end">
          <div className="w-full">
            <h3 className="text-lg font-semibold mb-2 line-clamp-1">
              {data.title}
            </h3>
            <p className="text-xs text-text-sub-title">{data.description}</p>
            <p className="text-xs text-text-sub-title">
              {formatDate(data.update_time)}
            </p>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
