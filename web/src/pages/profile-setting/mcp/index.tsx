import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { useListMcpServer } from '@/hooks/use-mcp-request';
import { Import, Plus } from 'lucide-react';
import { EditMcpDialog } from './edit-mcp-dialog';
import { McpCard } from './mcp-card';
import { useEditMcp } from './use-edit-mcp';

const list = new Array(10).fill('1');
export default function McpServer() {
  const { data } = useListMcpServer();
  const { editVisible, showEditModal, hideEditModal, handleOk } = useEditMcp();

  return (
    <section className="p-4">
      <div className="text-text-title text-2xl">MCP Servers</div>
      <section className="flex items-center justify-between">
        <div className="text-text-sub-title">自定义 MCP Server 的列表</div>
        <div className="flex gap-5">
          <SearchInput className="w-40"></SearchInput>
          <Button variant={'secondary'}>
            <Import /> Import
          </Button>
          <Button onClick={showEditModal('')}>
            <Plus /> Add MCP
          </Button>
        </div>
      </section>
      <section className="flex gap-5 flex-wrap pt-5">
        {data.mcp_servers.map((item) => (
          <McpCard key={item.id} data={item}></McpCard>
        ))}
      </section>
      {editVisible && (
        <EditMcpDialog
          hideModal={hideEditModal}
          onOk={handleOk}
        ></EditMcpDialog>
      )}
    </section>
  );
}
