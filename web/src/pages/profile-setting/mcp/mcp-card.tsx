import { MoreButton } from '@/components/more-button';
import { Card, CardContent } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { IMcpServer } from '@/interfaces/database/mcp';
import { formatDate } from '@/utils/date';
import { isPlainObject } from 'lodash';
import { useMemo } from 'react';
import { McpDropdown } from './mcp-dropdown';
import { UseBulkOperateMCPReturnType } from './use-bulk-operate-mcp';
import { UseEditMcpReturnType } from './use-edit-mcp';

export type DatasetCardProps = {
  data: IMcpServer;
} & Pick<UseBulkOperateMCPReturnType, 'handleSelectChange' | 'selectedList'> &
  Pick<UseEditMcpReturnType, 'showEditModal'>;

export function McpCard({
  data,
  selectedList,
  handleSelectChange,
  showEditModal,
}: DatasetCardProps) {
  const toolLength = useMemo(() => {
    const tools = data.variables?.tools;
    if (isPlainObject(tools)) {
      return Object.keys(tools || {}).length;
    }
    return 0;
  }, [data.variables?.tools]);
  const onCheckedChange = (checked: boolean) => {
    if (typeof checked === 'boolean') {
      handleSelectChange(data.id, checked);
    }
  };
  return (
    <Card key={data.id} className="w-64">
      <CardContent className="p-2.5 pt-2 group">
        <section className="flex justify-between pb-2">
          <h3 className="text-lg font-semibold truncate flex-1">{data.name}</h3>
          <div className="space-x-4">
            <McpDropdown mcpId={data.id} showEditModal={showEditModal}>
              <MoreButton></MoreButton>
            </McpDropdown>
            <Checkbox
              checked={selectedList.includes(data.id)}
              onCheckedChange={onCheckedChange}
              onClick={(e) => {
                e.stopPropagation();
              }}
            />
          </div>
        </section>
        <div className="flex justify-between items-end">
          <div className="w-full">
            <div className="text-base font-semibold mb-3 line-clamp-1 text-text-secondary">
              {toolLength} cached tools
            </div>
            <p className="text-sm text-text-secondary">
              {formatDate(data.update_date)}
            </p>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
