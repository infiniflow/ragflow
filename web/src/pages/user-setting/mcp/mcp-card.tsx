import { Card, CardContent } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { IMcpServer } from '@/interfaces/database/mcp';
import { formatDate } from '@/utils/date';
import { isPlainObject } from 'lodash';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { McpOperation } from './mcp-operation';
import { UseBulkOperateMCPReturnType } from './use-bulk-operate-mcp';
import { UseEditMcpReturnType } from './use-edit-mcp';

export type DatasetCardProps = {
  data: IMcpServer;
  isSelectionMode: boolean;
} & Pick<UseBulkOperateMCPReturnType, 'handleSelectChange' | 'selectedList'> &
  Pick<UseEditMcpReturnType, 'showEditModal'>;

export function McpCard({
  data,
  selectedList,
  handleSelectChange,
  showEditModal,
  isSelectionMode,
}: DatasetCardProps) {
  const { t } = useTranslation();
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
    <Card key={data.id}>
      <CardContent className="p-2.5 pt-2 group">
        <section className="flex justify-between pb-2">
          <h3 className="text-base font-normal truncate flex-1 text-text-primary">
            {data.name}
          </h3>
          <div className="space-x-4">
            {isSelectionMode ? (
              <Checkbox
                checked={selectedList.includes(data.id)}
                onCheckedChange={onCheckedChange}
                onClick={(e) => {
                  e.stopPropagation();
                }}
              />
            ) : (
              <McpOperation
                mcpId={data.id}
                showEditModal={showEditModal}
              ></McpOperation>
            )}
          </div>
        </section>
        <div className="flex justify-between items-end text-xs text-text-secondary">
          <div className="w-full">
            <div className="line-clamp-1 pb-1">
              {toolLength} {t('mcp.cachedTools')}
            </div>
            <p>{formatDate(data.update_date)}</p>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
