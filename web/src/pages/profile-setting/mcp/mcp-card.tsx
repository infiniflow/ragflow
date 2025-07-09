import { MoreButton } from '@/components/more-button';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Card, CardContent } from '@/components/ui/card';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { IMcpServer } from '@/interfaces/database/mcp';
import { formatDate } from '@/utils/date';
import { McpDropdown } from './mcp-dropdown';

export type DatasetCardProps = {
  data: IMcpServer;
};

export function McpCard({ data }: DatasetCardProps) {
  const { navigateToAgent } = useNavigatePage();

  return (
    <Card key={data.id} className="w-64" onClick={navigateToAgent(data.id)}>
      <CardContent className="p-2.5 pt-2 group">
        <section className="flex justify-between mb-2">
          <div className="flex gap-2 items-center">
            <Avatar className="size-6 rounded-lg">
              <AvatarImage src={data?.avatar} />
              <AvatarFallback className="rounded-lg ">CN</AvatarFallback>
            </Avatar>
          </div>
          <McpDropdown>
            <MoreButton></MoreButton>
          </McpDropdown>
        </section>
        <div className="flex justify-between items-end">
          <div className="w-full">
            <h3 className="text-lg font-semibold mb-2 line-clamp-1">
              {data.name}
            </h3>
            <p className="text-xs text-text-sub-title">{data.description}</p>
            <p className="text-xs text-text-sub-title">
              {formatDate(data.update_date)}
            </p>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
