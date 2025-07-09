import { MoreButton } from '@/components/more-button';
import { Card, CardContent } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { IMcpServer } from '@/interfaces/database/mcp';
import { formatDate } from '@/utils/date';
import { McpDropdown } from './mcp-dropdown';
import { UseBulkOperateMCPReturnType } from './use-bulk-operate-mcp';

export type DatasetCardProps = {
  data: IMcpServer;
} & Pick<UseBulkOperateMCPReturnType, 'handleSelectChange' | 'selectedList'>;

export function McpCard({
  data,
  selectedList,
  handleSelectChange,
}: DatasetCardProps) {
  return (
    <Card key={data.id} className="w-64">
      <CardContent className="p-2.5 pt-2 group">
        <section className="flex justify-between pb-2">
          <h3 className="text-lg font-semibold line-clamp-1">{data.name}</h3>
          <div className="space-x-4">
            <McpDropdown>
              <MoreButton></MoreButton>
            </McpDropdown>
            <Checkbox
              checked={selectedList.includes(data.id)}
              onCheckedChange={(checked) => {
                if (typeof checked === 'boolean') {
                  handleSelectChange(data.id, checked);
                }
              }}
              onClick={(e) => {
                e.stopPropagation();
              }}
            />
          </div>
        </section>
        <div className="flex justify-between items-end">
          <div className="w-full">
            <div className="text-base font-semibold mb-3 line-clamp-1 text-text-sub-title">
              20 cached tools
            </div>
            <p className="text-sm text-text-sub-title">
              {formatDate(data.update_date)}
            </p>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
